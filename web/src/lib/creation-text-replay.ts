import { appendTaskTextDelta, completeTextReplayTask, createGenerationTask, type GenerationTask } from "@/services/api/task-center";
import type { AiConfig } from "@/stores/use-config-store";

type TextReplayOptions = {
    aigcProjectId?: number;
    metadata?: Record<string, unknown>;
    onTaskCreated?: (task: GenerationTask) => void;
};

/**
 * text-replay 前端发布器：为前端直连模型生成的文本，在后端维护一个
 * text_replay 任务作持久化存档 + 回放。
 *
 * - start()：创建后端 text_replay 任务（不排队、不计费），拿到 taskId；
 * - publish(fullText)：按阈值节流地把新增片段通过 appendTaskTextDelta 上报
 *   （生成中途刷新页面也能读到已生成部分）；
 * - finish(finalText)：把最终正文写入任务并置为 succeeded，queryTaskTextReplay
 *   即可读到 finalText。
 *
 * 全部失败静默降级：文本直连生成是主流程，后端持久化只是增强，失败不阻塞生成。
 */
export function createTextReplayPublisher(config: AiConfig, prompt: string, options: TextReplayOptions = {}) {
    let taskId: string | null = null;
    let lastLen = 0;
    let buffered = "";
    let flushTimer: ReturnType<typeof setTimeout> | null = null;
    let closed = false;
    let startPromise: Promise<void> | null = null;
    let writeChain = Promise.resolve();

    // 增量累积到该阈值才落一次库，避免逐 token 高频写。
    const FLUSH_THRESHOLD = 2048;
    const enqueueDelta = (chunk: string) => {
        if (!taskId || !chunk.trim()) return;
        writeChain = writeChain.then(() => appendTaskTextDelta(taskId!, chunk)).catch(() => undefined);
    };

    const flush = (force = false) => {
        if (flushTimer) {
            clearTimeout(flushTimer);
            flushTimer = null;
        }
        if (closed || !taskId) return;
        if (buffered.length >= FLUSH_THRESHOLD || force) {
            const chunk = buffered;
            buffered = "";
            enqueueDelta(chunk);
        } else if (buffered) {
            flushTimer = setTimeout(() => flush(true), 1000);
        }
    };

    const start = () => {
        if (startPromise) return startPromise;
        startPromise = createGenerationTask({
            aigcProjectId: options.aigcProjectId,
            type: "text",
            prompt: prompt.trim() || "文本创作",
            model: config.model,
            input: {
                mode: "text",
                replay: true,
                prompt: prompt.trim() || "文本创作",
                metadata: options.metadata,
                config: {
                    model: config.model,
                    apiFormat: config.apiFormat,
                },
            },
        })
            .then((task) => {
                taskId = task.id || null;
                if (taskId) options.onTaskCreated?.(task);
                if (!closed) flush();
            })
            .catch(() => {
                taskId = null;
            });
        return startPromise;
    };

    const publish = (fullText: string) => {
        if (closed || fullText.length <= lastLen) return;
        buffered += fullText.slice(lastLen);
        lastLen = fullText.length;
        flush();
    };

    const finish = async (finalText: string) => {
        if (closed) return;
        closed = true;
        if (flushTimer) {
            clearTimeout(flushTimer);
            flushTimer = null;
        }
        await startPromise;
        if (!taskId) return;
        const chunk = buffered;
        buffered = "";
        enqueueDelta(chunk);
        await writeChain;
        if (finalText.trim()) await completeTextReplayTask(taskId, finalText).catch(() => undefined);
    };

    return { start, publish, finish };
}
