import { describe, expect, test } from "bun:test";

import { reconcileCreationAttachmentLimit, removeCreationReferenceTokens } from "../src/pages/create/creation-references";

const attachments = [
    { id: "first", name: "第一张", type: "image/png", dataUrl: "https://example.test/first.png", url: "https://example.test/first.png", previewUrl: "https://example.test/first.png" },
    { id: "second", name: "第二张", type: "image/png", dataUrl: "https://example.test/second.png", url: "https://example.test/second.png", previewUrl: "https://example.test/second.png" },
];

const references = [
    { id: "upload:first", nodeId: "upload:first", kind: "image" as const, label: "图片1", title: "当前参考内容", attachmentId: "first", active: true },
    { id: "upload:second", nodeId: "upload:second", kind: "image" as const, label: "图片2", title: "当前参考内容", attachmentId: "second", active: true },
];

describe("创作参考素材上限", () => {
    test("切换模型时只裁掉超过上限的参考素材", () => {
        const reconciled = reconcileCreationAttachmentLimit(attachments, references, 1);

        expect(reconciled.attachments).toEqual([attachments[0]]);
        expect(reconciled.removedReferences).toEqual([references[1]]);
    });

    test("裁掉参考素材时同步移除提示词引用标记", () => {
        const prompt = "保留 @[node:upload:first]，移除 @[node:upload:second]";

        expect(removeCreationReferenceTokens(prompt, [references[1]])).toBe("保留 @[node:upload:first]，移除 ");
    });
});
