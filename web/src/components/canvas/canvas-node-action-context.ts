import { createContext, useContext } from "react";

import type { CanvasNodeData, CanvasNodeMetadata } from "@/types/canvas";

// 批次子图操作条（下载/创建副本/删除）与主图位下载需要调用画布级动作，
// 但画布节点经 CanvasProjectWorldLayers 渲染、不便逐个透传 handler，
// 通过 Context 注入，避免改动 world-layers。无 Provider 时静默降级为 no-op。
export type CanvasNodeActionContextValue = {
    download?: (node: CanvasNodeData) => void;
    duplicate?: (node: CanvasNodeData) => void;
    deleteNode?: (node: CanvasNodeData) => void;
    updateMetadata?: (nodeId: string, patch: Partial<CanvasNodeMetadata>) => void;
    resizeNode?: (nodeId: string, size: { width: number; height: number }) => void;
};

export const CanvasNodeActionContext = createContext<CanvasNodeActionContextValue>({});

export function useCanvasNodeActions() {
    return useContext(CanvasNodeActionContext);
}
