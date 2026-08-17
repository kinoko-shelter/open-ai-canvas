import { describe, expect, test } from "bun:test";

import { buildTimelineRenderPlan, formatSrtTimestamp, type TimelineRenderSource } from "../src/lib/timeline/timeline-to-ffmpeg";
import type { TimelineClip, TimelineProject } from "../src/types/timeline";

function videoClip(id: string, nodeId: string, startMs: number, durationMs: number): TimelineClip {
    return { id, kind: "video", nodeId, trackId: "video", startMs, durationMs, title: id, sourceStartMs: 0, sourceDurationMs: durationMs };
}

function timeline(clips: TimelineClip[]): TimelineProject {
    return { version: 2, tracks: [], clips, durationMs: clips.reduce((max, clip) => Math.max(max, clip.startMs + clip.durationMs), 0) };
}

function source(nodeId: string): TimelineRenderSource {
    return { nodeId, fileName: `input-${nodeId}.mp4`, durationMs: 15_000, url: `file:///${nodeId}.mp4` };
}

function concatTotalSeconds(plan: ReturnType<typeof buildTimelineRenderPlan>): number {
    let total = 0;
    for (const entry of plan.concatEntries) {
        const step = plan.steps.find((item) => item.output === entry);
        if (step?.kind === "trim") total += Number(step.args[step.args.indexOf("-t") + 1]);
        if (step?.kind === "gap") {
            const lavfi = step.args.find((arg) => arg.startsWith("color=c=black")) || "";
            total += Number(lavfi.split("d=")[1]);
        }
    }
    return total;
}

function concatStartOffsetsMs(plan: ReturnType<typeof buildTimelineRenderPlan>): number[] {
    const offsets: number[] = [];
    let cursor = 0;
    for (const entry of plan.concatEntries) {
        const step = plan.steps.find((item) => item.output === entry);
        if (!step) continue;
        offsets.push(cursor);
        if (step.kind === "trim") cursor += Number(step.args[step.args.indexOf("-t") + 1]) * 1000;
        if (step.kind === "gap") {
            const lavfi = step.args.find((arg) => arg.startsWith("color=c=black")) || "";
            cursor += Number(lavfi.split("d=")[1]) * 1000;
        }
    }
    return offsets;
}

describe("buildTimelineRenderPlan 片段与黑场对齐", () => {
    test("连续片段保持原有顺序且不插入黑场", () => {
        const project = timeline([videoClip("a", "node-a", 0, 15_000), videoClip("b", "node-b", 15_000, 4_000), videoClip("c", "node-c", 19_000, 15_000)]);
        const plan = buildTimelineRenderPlan(project, [source("node-a"), source("node-b"), source("node-c")]);

        expect(plan.steps.filter((step) => step.kind === "gap")).toHaveLength(0);
        expect(plan.concatEntries).toEqual(["trim-0.mp4", "trim-1.mp4", "trim-2.mp4"]);
        expect(concatTotalSeconds(plan)).toBe(34);
    });

    test("黑场插在时间线空隙处，而不是追加到片尾", () => {
        const project = timeline([videoClip("a", "node-a", 0, 15_000), videoClip("b", "node-b", 25_000, 15_000)]);
        const plan = buildTimelineRenderPlan(project, [source("node-a"), source("node-b")]);

        expect(plan.concatEntries).toEqual(["trim-0.mp4", "gap-1.mp4", "trim-1.mp4"]);
        expect(concatTotalSeconds(plan)).toBe(40);
        expect(concatStartOffsetsMs(plan)).toEqual([0, 15_000, 25_000]);
    });

    test("中间无源片段由单个黑场代替，后续片段时间不漂移", () => {
        const project = timeline([videoClip("a", "node-a", 0, 15_000), videoClip("b", "node-b", 15_000, 4_000), videoClip("c", "node-c", 19_000, 15_000)]);
        const plan = buildTimelineRenderPlan(project, [source("node-a"), source("node-c")]);

        expect(plan.concatEntries).toEqual(["trim-0.mp4", "gap-2.mp4", "trim-2.mp4"]);
        expect(concatTotalSeconds(plan)).toBe(34);
        expect(concatStartOffsetsMs(plan)).toEqual([0, 15_000, 19_000]);
    });

    test("无源片段前已有空隙时不重复计算黑场", () => {
        const project = timeline([videoClip("a", "node-a", 0, 15_000), videoClip("b", "node-b", 20_000, 4_000), videoClip("c", "node-c", 24_000, 15_000)]);
        const plan = buildTimelineRenderPlan(project, [source("node-a"), source("node-c")]);
        const gaps = plan.steps.filter((step) => step.kind === "gap");

        expect(gaps).toHaveLength(1);
        expect(gaps[0]?.args.join(" ")).toContain("d=9");
        expect(plan.concatEntries).toEqual(["trim-0.mp4", "gap-2.mp4", "trim-2.mp4"]);
        expect(concatTotalSeconds(plan)).toBe(39);
    });

    test("连续无源片段合并为一个黑场", () => {
        const project = timeline([videoClip("a", "node-a", 0, 15_000), videoClip("b", "node-b", 15_000, 4_000), videoClip("c", "node-c", 19_000, 8_000), videoClip("d", "node-d", 27_000, 15_000)]);
        const plan = buildTimelineRenderPlan(project, [source("node-a"), source("node-d")]);
        const gaps = plan.steps.filter((step) => step.kind === "gap");

        expect(gaps).toHaveLength(1);
        expect(gaps[0]?.args.join(" ")).toContain("d=12");
        expect(plan.concatEntries).toEqual(["trim-0.mp4", "gap-3.mp4", "trim-3.mp4"]);
        expect(concatTotalSeconds(plan)).toBe(42);
    });

    test("首个无源片段从零点补黑场", () => {
        const project = timeline([videoClip("b", "node-b", 0, 4_000), videoClip("c", "node-c", 4_000, 15_000)]);
        const plan = buildTimelineRenderPlan(project, [source("node-c")]);

        expect(plan.concatEntries).toEqual(["gap-1.mp4", "trim-1.mp4"]);
        expect(concatTotalSeconds(plan)).toBe(19);
    });

    test("没有可用源素材时不尝试输出空成片", () => {
        const plan = buildTimelineRenderPlan(timeline([videoClip("a", "node-a", 0, 15_000)]), []);

        expect(plan.concatEntries).toEqual([]);
        expect(plan.steps.some((step) => step.output === "export.mp4")).toBe(false);
    });
});

describe("trim 步骤输出 seek", () => {
    test("-ss 在 -i 之后，避免按关键帧定位导致裁切偏移", () => {
        const plan = buildTimelineRenderPlan(timeline([videoClip("a", "node-a", 0, 15_000)]), [source("node-a")]);
        const args = plan.steps.find((step) => step.kind === "trim")?.args || [];

        expect(args.indexOf("-ss")).toBeGreaterThan(args.indexOf("-i"));
    });
});

describe("formatSrtTimestamp", () => {
    test("SRT 时间码毫秒对齐三位", () => {
        expect(formatSrtTimestamp(3_600_000 + 60_000 + 1_234)).toBe("01:01:01,234");
        expect(formatSrtTimestamp(0)).toBe("00:00:00,000");
    });
});
