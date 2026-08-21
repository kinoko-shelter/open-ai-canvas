import { parseBackendGenerationResult } from "@/services/api/generation-task";
import type { GenerationTask, TaskTextReplay } from "@/services/api/task-center";

export type RecoverableCreationTextMessage = {
    mode?: string;
    status?: string;
    content: string;
    error?: string;
    taskIds?: string[];
};

export type CreationTextTaskRecovery = {
    status: "streaming" | "pending" | "done" | "error" | "cancelled";
    content: string;
    error?: string;
    taskIds: string[];
};

type TextReplayGenerationTask = GenerationTask & { textReplay?: TaskTextReplay };

// 页面重载后从后端任务恢复消息；text_replay 可在生成中恢复已持久化的正文。
export function recoverCreationTextTask(message: RecoverableCreationTextMessage, tasks: TextReplayGenerationTask[]): CreationTextTaskRecovery | null {
    if (message.mode !== "text" || (message.status !== "streaming" && message.status !== "pending")) return null;

    if (!tasks.length) return null;

    const nextTaskIds = Array.from(new Set([...(message.taskIds || []), ...tasks.map((task) => task.id)]));
    const replayTask = tasks.find((task) => task.status === "text_replay");
    if (replayTask) {
        const replay = replayTask.textReplay;
        const content = replay?.finalText || replay?.textDraft || replay?.deltas.map((delta) => delta.content).join("") || message.content;
        if (replay?.complete && replay.finalText?.trim()) return { status: "done", content: replay.finalText, error: undefined, taskIds: nextTaskIds };
        return { status: "streaming", content, error: undefined, taskIds: nextTaskIds };
    }
    if (tasks.some((task) => task.status === "queued" || task.status === "running")) {
        return { status: message.status === "streaming" ? "streaming" : "pending", content: message.content, error: undefined, taskIds: nextTaskIds };
    }

    const succeeded = tasks.find((task) => task.status === "succeeded");
    if (succeeded) {
        try {
            const text = parseBackendGenerationResult(succeeded).text;
            if (!text?.trim()) throw new Error("后端任务没有返回文本");
            return { status: "done", content: text, error: undefined, taskIds: nextTaskIds };
        } catch (error) {
            return { status: "error", content: "生成失败", error: error instanceof Error ? error.message : "文本任务结果格式错误", taskIds: nextTaskIds };
        }
    }
    if (tasks.every((task) => task.status === "cancelled")) {
        return { status: "cancelled", content: "已停止", error: undefined, taskIds: nextTaskIds };
    }
    const failed = tasks.find((task) => task.status === "failed");
    return { status: "error", content: "生成失败", error: failed?.error || "文本任务已失败", taskIds: nextTaskIds };
}
