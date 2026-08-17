import { describe, expect, test } from "bun:test";

import { failedImageBatchChildren, markImageBatchRetrying, reconcileImageBatchRoot, restoreUnsubmittedImageBatchChild } from "../src/lib/canvas/canvas-image-batch-retry";
import { CanvasNodeType, type CanvasNodeData, type CanvasNodeStatus } from "../src/types/canvas";

function imageNode(id: string, status: CanvasNodeStatus, metadata: Partial<NonNullable<CanvasNodeData["metadata"]>> = {}): CanvasNodeData {
    return {
        id,
        type: CanvasNodeType.Image,
        title: id,
        position: { x: 0, y: 0 },
        width: 320,
        height: 240,
        metadata: { status, ...metadata },
    };
}

describe("canvas image batch retry", () => {
    test("only returns failed children for the current batch in batch order", () => {
        const root = imageNode("root", "error", { isBatchRoot: true, batchChildIds: ["failed-2", "success", "failed-1", "loading", "foreign"] });
        const nodes = [
            root,
            imageNode("failed-1", "error", { batchRootId: root.id }),
            imageNode("failed-2", "error", { batchRootId: root.id }),
            imageNode("success", "success", { batchRootId: root.id, content: "image:success" }),
            imageNode("loading", "loading", { batchRootId: root.id }),
            imageNode("foreign", "error", { batchRootId: "another-root" }),
        ];

        expect(failedImageBatchChildren(root, nodes).map((node) => node.id)).toEqual(["failed-2", "failed-1"]);
    });

    test("restores the root primary image while retaining other failures", () => {
        const root = imageNode("root", "loading", { isBatchRoot: true, batchChildIds: ["failed", "success"], errorDetails: "旧错误" });
        const failed = imageNode("failed", "error", { batchRootId: root.id, errorDetails: "上游失败" });
        const success = imageNode("success", "success", { batchRootId: root.id, content: "image:success", storageKey: "resource:success", mimeType: "image/png", naturalWidth: 1024, naturalHeight: 1024 });

        const next = reconcileImageBatchRoot(root, [root, failed, success]);

        expect(next.metadata).toMatchObject({ status: "success", content: "image:success", storageKey: "resource:success", primaryImageId: "success", batchFailedCount: 1 });
        expect(next.metadata.errorDetails).toBeUndefined();
    });

    test("syncs a representative failure to a fully failed batch root", () => {
        const root = imageNode("root", "loading", { isBatchRoot: true, batchChildIds: ["failed-1", "failed-2"] });
        const failed = imageNode("failed-1", "error", { batchRootId: root.id, errorDetails: "No available compatible accounts", generationErrorCode: "upstream_unavailable" });

        const next = reconcileImageBatchRoot(root, [root, failed, imageNode("failed-2", "error", { batchRootId: root.id, errorDetails: "另一错误" })]);

        expect(next.metadata).toMatchObject({ status: "error", errorDetails: "No available compatible accounts", generationErrorCode: "upstream_unavailable", batchFailedCount: 2 });
        expect(next.metadata.content).toBeUndefined();
    });

    test("moves the root and failed children into loading before retry", () => {
        const root = imageNode("root", "error", { isBatchRoot: true, batchChildIds: ["failed-1", "success", "failed-2"], batchFailedCount: 2, errorDetails: "全部失败" });
        const failed1 = imageNode("failed-1", "error", { batchRootId: root.id, errorDetails: "失败 1" });
        const success = imageNode("success", "success", { batchRootId: root.id, content: "image:success" });
        const failed2 = imageNode("failed-2", "error", { batchRootId: root.id, errorDetails: "失败 2" });

        const next = markImageBatchRetrying(root.id, [failed1.id, failed2.id], [root, failed1, success, failed2]);

        expect(next.find((node) => node.id === root.id)?.metadata).toMatchObject({ status: "loading", batchFailedCount: 2 });
        expect(next.find((node) => node.id === failed1.id)?.metadata?.status).toBe("loading");
        expect(next.find((node) => node.id === failed2.id)?.metadata?.status).toBe("loading");
        expect(next.find((node) => node.id === success.id)?.metadata).toMatchObject({ status: "success", content: "image:success" });
    });

    test("restores a child that never submitted a retry request", () => {
        const original = imageNode("failed", "error", { batchRootId: "root", errorDetails: "原始失败" });
        const loading = imageNode("failed", "loading", { batchRootId: "root" });

        expect(restoreUnsubmittedImageBatchChild(loading, original).metadata).toMatchObject({ status: "error", errorDetails: "原始失败" });
        expect(restoreUnsubmittedImageBatchChild(imageNode("failed", "success", { content: "image:success" }), original).metadata?.status).toBe("success");
    });
});
