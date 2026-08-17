import { useCallback, useEffect, useLayoutEffect, useRef, useState, type Dispatch, type MouseEvent as ReactMouseEvent, type PointerEvent as ReactPointerEvent, type SetStateAction } from "react";
import { App } from "antd";
import { nanoid } from "nanoid";

import type { PendingConnectionCreate } from "@/components/canvas/canvas-workspace-overlays";
import { getNodeSpec } from "@/constant/canvas";
import { batchSourceRestriction, buildBatchConnectionCreateRequest, hasBatchConnectionCandidate, planBatchConnections, type CanvasBatchConnectionPreview } from "@/lib/canvas/canvas-batch-connection";
import { canvasConnectionError } from "@/lib/canvas/canvas-connection-policy";
import { attachNodeToStoryboardRow, createCanvasNode, getConnectionTargetAnchor, isHiddenBatchChild, normalizeConnection, storyboardHandleAtY, storyboardPromptTemplateMetadata, storyboardRowFromHandle } from "@/lib/canvas/canvas-project-domain";
import { createCanvasDrawingFromImage } from "@/lib/canvas/canvas-drawing-storage";
import { isDrawingEngineAvailable, type CanvasDrawingEngine } from "@/lib/canvas/canvas-drawing-engine";
import { isFrameNode, isNodeHiddenByCollapsedFrame } from "@/lib/canvas/canvas-frame";
import type { AiConfig } from "@/stores/use-config-store";
import { useUserStore } from "@/stores/use-user-store";
import { CanvasNodeType, type CanvasConnection, type CanvasNodeData, type ConnectionHandle, type ContextMenuState, type Position, type ViewportTransform } from "@/types/canvas";

type UseCanvasConnectionControllerOptions = {
    projectId: string;
    config: AiConfig;
    defaultDrawingEngine: CanvasDrawingEngine;
    nodesRef: { current: CanvasNodeData[] };
    connectionsRef: { current: CanvasConnection[] };
    viewportRef: { current: ViewportTransform };
    scriptScrollTopById: Record<string, number>;
    screenToCanvas: (clientX: number, clientY: number) => Position;
    setNodes: Dispatch<SetStateAction<CanvasNodeData[]>>;
    setConnections: Dispatch<SetStateAction<CanvasConnection[]>>;
    setSelectedNodeIds: Dispatch<SetStateAction<Set<string>>>;
    setSelectedConnectionId: Dispatch<SetStateAction<string | null>>;
    setContextMenu: Dispatch<SetStateAction<ContextMenuState | null>>;
    setDialogNodeId: Dispatch<SetStateAction<string | null>>;
    setDrawingNodeId: Dispatch<SetStateAction<string | null>>;
};

type ConnectionDropTarget = {
    nodeId: string | null;
    handleId?: string;
    anchorRatio?: number;
    isNearNode: boolean;
};

const CONNECTION_HANDLE_HIT_RADIUS = 40;
const CONNECTION_NODE_HIT_PADDING = 32;
const NODE_STATUS_IDLE = "idle" as const;

export function useCanvasConnectionController({
    projectId,
    config,
    defaultDrawingEngine,
    nodesRef,
    connectionsRef,
    viewportRef,
    scriptScrollTopById,
    screenToCanvas,
    setNodes,
    setConnections,
    setSelectedNodeIds,
    setSelectedConnectionId,
    setContextMenu,
    setDialogNodeId,
    setDrawingNodeId,
}: UseCanvasConnectionControllerOptions) {
    const { message } = App.useApp();
    const tldrawLicenseKey = useUserStore((state) => state.drawingEngine.tldrawLicenseKey);
    const [connectingParams, setConnectingParams] = useState<ConnectionHandle | null>(null);
    const [connectionTargetNodeId, setConnectionTargetNodeId] = useState<string | null>(null);
    const [connectionTargetAnchorRatio, setConnectionTargetAnchorRatio] = useState<number | undefined>();
    const [pendingConnectionCreate, setPendingConnectionCreate] = useState<PendingConnectionCreate | null>(null);
    const [batchConnectionPreview, setBatchConnectionPreview] = useState<CanvasBatchConnectionPreview | null>(null);
    const [mouseWorld, setMouseWorld] = useState<Position>({ x: 0, y: 0 });
    const connectingParamsRef = useRef(connectingParams);
    const connectingPointerIdRef = useRef<number | null>(null);
    const connectingPointerStartRef = useRef<Position | null>(null);
    const pendingConnectionCreateRef = useRef(pendingConnectionCreate);
    const batchConnectionPreviewRef = useRef<CanvasBatchConnectionPreview | null>(null);

    useLayoutEffect(() => {
        connectingParamsRef.current = connectingParams;
        pendingConnectionCreateRef.current = pendingConnectionCreate;
    }, [connectingParams, pendingConnectionCreate]);

    const updateBatchConnectionPreview = useCallback((next: CanvasBatchConnectionPreview | null) => {
        batchConnectionPreviewRef.current = next;
        setBatchConnectionPreview(next);
    }, []);

    const clearBatchConnection = useCallback(() => {
        updateBatchConnectionPreview(null);
    }, [updateBatchConnectionPreview]);

    const setConnecting = useCallback((next: ConnectionHandle | null) => {
        connectingParamsRef.current = next;
        setConnectingParams(next);
        if (!next) {
            connectingPointerIdRef.current = null;
            connectingPointerStartRef.current = null;
            setConnectionTargetNodeId(null);
            setConnectionTargetAnchorRatio(undefined);
        }
    }, []);

    const closeConnectionCreateMenu = useCallback(() => {
        pendingConnectionCreateRef.current = null;
        setPendingConnectionCreate(null);
    }, []);

    const cancelPendingConnectionCreate = useCallback(() => {
        closeConnectionCreateMenu();
        setConnecting(null);
        clearBatchConnection();
    }, [clearBatchConnection, closeConnectionCreateMenu, setConnecting]);

    const previewBatchConnection = useCallback((sourceNodeIds: string[], targetNodeId: string | null, targetHandleId: string | undefined, targetAnchorRatio: number | undefined, nextMouseWorld: Position) => {
        const plan = targetNodeId
            ? planBatchConnections({ sourceNodeIds, targetNodeId, targetHandleId, targetAnchorRatio, nodes: nodesRef.current, connections: connectionsRef.current, config })
            : null;
        const eligibleSourceCount = sourceNodeIds.filter((id) => {
            const node = nodesRef.current.find((item) => item.id === id);
            return Boolean(node && !batchSourceRestriction(node));
        }).length;
        const status = !targetNodeId || !plan ? "idle" : plan.connections.length === eligibleSourceCount ? "valid" : plan.connections.length ? "partial" : "invalid";
        updateBatchConnectionPreview({ sourceNodeIds, targetNodeId, targetHandleId, targetAnchorRatio, mouseWorld: nextMouseWorld, status });
        return plan;
    }, [config, connectionsRef, nodesRef, updateBatchConnectionPreview]);

    const commitBatchConnection = useCallback((sourceNodeIds: string[], targetNodeId: string, targetHandleId?: string, targetAnchorRatio?: number) => {
        const plan = planBatchConnections({ sourceNodeIds, targetNodeId, targetHandleId, targetAnchorRatio, nodes: nodesRef.current, connections: connectionsRef.current, config });
        if (!plan.connections.length) {
            message.warning(plan.skipped[0]?.reason || "没有可建立的连接");
            return plan;
        }
        setNodes((currentNodes) => plan.connections.reduce((current, connection) => attachNodeToStoryboardRow(current, connection), currentNodes));
        setConnections((currentConnections) => [...currentConnections, ...plan.connections]);
        setContextMenu(null);
        const skippedCount = plan.skipped.length;
        const duplicateCount = plan.duplicates.length;
        const suffix = skippedCount || duplicateCount ? `，跳过 ${skippedCount + duplicateCount} 个` : "";
        if (skippedCount) message.warning(`已连接 ${plan.connected.length} 个节点${suffix}：${plan.skipped[0].reason}`);
        else message.success(`已连接 ${plan.connected.length} 个节点${suffix}`);
        return plan;
    }, [config, connectionsRef, message, nodesRef, setConnections, setContextMenu, setNodes]);

    const connectNodes = useCallback((current: ConnectionHandle, targetNodeId: string, targetHandleId?: string, targetAnchorRatio?: number) => {
        if (current.nodeId === targetNodeId) return;
        const connection = normalizeConnection(current.nodeId, targetNodeId, nodesRef.current, current.handleType);
        if (!connection) {
            message.warning("配置节点之间不能连接");
            return;
        }
        const { fromNodeId, toNodeId } = connection;
        const fromHandleId = fromNodeId === current.nodeId ? current.handleId : targetHandleId;
        const toHandleId = toNodeId === current.nodeId ? current.handleId : targetHandleId;
        const fromAnchorRatio = fromNodeId === current.nodeId ? current.anchorRatio : targetAnchorRatio;
        const toAnchorRatio = toNodeId === current.nodeId ? current.anchorRatio : targetAnchorRatio;
        const policyError = canvasConnectionError(config, nodesRef.current, connectionsRef.current, { fromNodeId, toNodeId });
        if (policyError) {
            message.warning(policyError);
            return;
        }
        const exists = connectionsRef.current.find((item) => item.fromNodeId === fromNodeId && item.toNodeId === toNodeId && item.fromHandleId === fromHandleId && item.toHandleId === toHandleId);
        if (exists) {
            setConnections((currentConnections) => currentConnections.map((item) => item.id === exists.id ? { ...item, fromAnchorRatio, toAnchorRatio } : item));
        } else {
            setConnections((currentConnections) => [...currentConnections, { id: `conn-${Date.now()}`, fromNodeId, toNodeId, fromHandleId, toHandleId, fromAnchorRatio, toAnchorRatio }]);
            setNodes((currentNodes) => attachNodeToStoryboardRow(currentNodes, { fromNodeId, toNodeId, fromHandleId, toHandleId }));
        }
        setContextMenu(null);
    }, [config, connectionsRef, message, nodesRef, setConnections, setContextMenu, setNodes]);

    const createConnectedNode = useCallback(async (type: CanvasNodeType.Image | CanvasNodeType.Text | CanvasNodeType.Script | CanvasNodeType.Video | CanvasNodeType.Audio | CanvasNodeType.Drawing, pending: PendingConnectionCreate) => {
        if (type === CanvasNodeType.Drawing && !isDrawingEngineAvailable(defaultDrawingEngine, tldrawLicenseKey)) {
            message.error("当前生产构建未配置 tldraw License Key，不能创建 tldraw 绘图");
            return;
        }
        const batchSourceNodeIds = pending.batchSourceNodeIds?.length ? Array.from(new Set(pending.batchSourceNodeIds)) : [];
        const batchSourceNodes = batchSourceNodeIds
            .map((nodeId) => nodesRef.current.find((node) => node.id === nodeId))
            .filter((node): node is CanvasNodeData => Boolean(node));
        const storyboardRow = batchSourceNodeIds.length ? undefined : type === CanvasNodeType.Video ? storyboardRowFromHandle(nodesRef.current, pending.connection.nodeId, pending.connection.handleId) : undefined;
        const videoPrompt = storyboardRow ? (storyboardRow.videoMotionPrompt || storyboardRow.plotDescription).trim() : "";
        const sourceNode = pending.connection.handleType === "source" ? nodesRef.current.find((node) => node.id === pending.connection.nodeId) : undefined;
        const batchScriptPrompt = batchSourceNodes
            .filter((node) => node.type === CanvasNodeType.Text)
            .map((node) => (node.metadata?.content || node.metadata?.prompt || "").trim())
            .filter(Boolean)
            .join("\n\n");
        const scriptPrompt = type === CanvasNodeType.Script
            ? batchSourceNodeIds.length ? batchScriptPrompt : sourceNode?.type === CanvasNodeType.Text ? (sourceNode.metadata?.content || sourceNode.metadata?.prompt || "").trim() : ""
            : "";
        const metadata = type === CanvasNodeType.Drawing
            ? { drawingEngine: defaultDrawingEngine }
            : type === CanvasNodeType.Script && scriptPrompt
              ? { prompt: scriptPrompt, composerContent: scriptPrompt }
            : type === CanvasNodeType.Video && storyboardRow
              ? { prompt: videoPrompt, composerContent: videoPrompt, ...storyboardPromptTemplateMetadata(storyboardRow, "video"), generationMode: "video" as const, videoEditOperation: "text_to_video" as const, workflowKind: "shot" as const, workflowTitle: `镜头 ${storyboardRow.shotNumber} 视频`, shotIndex: storyboardRow.shotNumber, seconds: String(storyboardRow.durationSeconds), status: NODE_STATUS_IDLE }
              : undefined;
        const sourceNodeForQuickCreate = pending.quick ? nodesRef.current.find((node) => node.id === pending.connection.nodeId) : undefined;
        const spec = getNodeSpec(type);
        const anchorY = sourceNodeForQuickCreate ? sourceNodeForQuickCreate.position.y + sourceNodeForQuickCreate.height * (pending.connection.anchorRatio ?? 0.5) : pending.position.y;
        const position = sourceNodeForQuickCreate
            ? {
                  x: pending.connection.handleType === "source"
                      ? sourceNodeForQuickCreate.position.x + sourceNodeForQuickCreate.width + 96 + spec.width / 2
                      : sourceNodeForQuickCreate.position.x - 96 - spec.width / 2,
                  y: anchorY,
              }
            : pending.position;
        const newNode = createCanvasNode(type, position, metadata);
        if (storyboardRow) newNode.title = `镜头 ${storyboardRow.shotNumber} · 视频`;
        if (batchSourceNodeIds.length && type === CanvasNodeType.Drawing) {
            message.error("批量连接暂不支持创建绘图，请先连接到普通节点");
            closeConnectionCreateMenu();
            return;
        }
        const batchPlan = batchSourceNodeIds.length
            ? planBatchConnections({ sourceNodeIds: batchSourceNodeIds, targetNodeId: newNode.id, nodes: [...nodesRef.current, newNode], connections: connectionsRef.current, config, allowCapacityOverflow: true })
            : null;
        if (batchPlan) {
            if (!batchPlan.connections.length) {
                const detail = batchPlan.skipped.slice(0, 3).map((item) => item.reason).join("；");
                message.warning(detail ? `没有可建立的连接：${detail}` : "没有可建立的连接");
                closeConnectionCreateMenu();
                setConnecting(null);
                return;
            }
            const nextConnections = [...connectionsRef.current, ...batchPlan.connections];
            const nextNodes = batchPlan.connections.reduce((currentNodes, connection) => attachNodeToStoryboardRow(currentNodes, connection), [...nodesRef.current, newNode]);
            nodesRef.current = nextNodes;
            connectionsRef.current = nextConnections;
            setNodes(nextNodes);
            setConnections(nextConnections);
            setSelectedNodeIds(new Set([newNode.id]));
            setSelectedConnectionId(null);
            if (type !== CanvasNodeType.Text && type !== CanvasNodeType.Script && type !== CanvasNodeType.Audio) setDialogNodeId(newNode.id);
            const skippedCount = batchPlan.skipped.length;
            const duplicateCount = batchPlan.duplicates.length;
            const suffix = skippedCount || duplicateCount ? `，跳过 ${skippedCount + duplicateCount} 个` : "";
            if (skippedCount) message.warning(`已创建并连接 ${batchPlan.connections.length} 个源节点${suffix}：${batchPlan.skipped[0].reason}`);
            else message.success(`已创建节点并连接 ${batchPlan.connections.length} 个源节点${suffix}`);
            closeConnectionCreateMenu();
            setConnecting(null);
            return;
        }
        const connection = normalizeConnection(pending.connection.nodeId, newNode.id, [...nodesRef.current, newNode], pending.connection.handleType);
        if (!connection) {
            message.warning("配置节点之间不能连接");
            return;
        }
        const policyError = canvasConnectionError(config, [...nodesRef.current, newNode], connectionsRef.current, connection);
        if (policyError) {
            message.warning(policyError);
            return;
        }
        if (type === CanvasNodeType.Drawing) {
            const drawingSourceNode = nodesRef.current.find((node) => node.id === pending.connection.nodeId);
            const sourceUrl = drawingSourceNode?.type === CanvasNodeType.Image ? drawingSourceNode.metadata?.content : "";
            if (pending.connection.handleType !== "source" || !drawingSourceNode || !sourceUrl || !newNode.metadata?.drawingId) {
                message.error("只有已有图片内容的输出连线可以创建绘图");
                return;
            }
            closeConnectionCreateMenu();
            setConnecting(null);
            try {
                const saved = await createCanvasDrawingFromImage(projectId, newNode.metadata.drawingId, defaultDrawingEngine, {
                    url: sourceUrl,
                    storageKey: drawingSourceNode.metadata?.storageKey,
                    name: drawingSourceNode.title || "来源图片",
                    mimeType: drawingSourceNode.metadata?.mimeType,
                });
                newNode.title = `${drawingSourceNode.title || "图片"} · 绘图`;
                newNode.metadata = {
                    ...newNode.metadata,
                    drawingEngine: saved.engine,
                    drawingRevision: saved.revision,
                    drawingUpdatedAt: saved.updatedAt,
                    drawingShapeCount: saved.shapeCount,
                    drawingPageCount: saved.pageCount,
                };
            } catch (error) {
                message.error(error instanceof Error ? `创建绘图失败：${error.message}` : "创建绘图失败");
                return;
            }
        }
        const fromHandleId = connection.fromNodeId === pending.connection.nodeId ? pending.connection.handleId : undefined;
        const toHandleId = connection.toNodeId === pending.connection.nodeId ? pending.connection.handleId : undefined;
        const fromAnchorRatio = connection.fromNodeId === pending.connection.nodeId ? pending.connection.anchorRatio : 0.5;
        const toAnchorRatio = connection.toNodeId === pending.connection.nodeId ? pending.connection.anchorRatio : 0.5;
        const connected = { ...connection, fromHandleId, toHandleId, fromAnchorRatio, toAnchorRatio };
        setNodes((currentNodes) => attachNodeToStoryboardRow([...currentNodes, newNode], connected));
        setConnections((currentConnections) => [...currentConnections, { id: nanoid(), ...connected }]);
        setSelectedNodeIds(new Set([newNode.id]));
        setSelectedConnectionId(null);
        if (type === CanvasNodeType.Drawing) setDrawingNodeId(newNode.id);
        else if (type !== CanvasNodeType.Text && type !== CanvasNodeType.Script && type !== CanvasNodeType.Audio) setDialogNodeId(newNode.id);
        closeConnectionCreateMenu();
        setConnecting(null);
    }, [closeConnectionCreateMenu, config, connectionsRef, defaultDrawingEngine, message, nodesRef, projectId, setConnecting, setConnections, setDialogNodeId, setDrawingNodeId, setNodes, setSelectedConnectionId, setSelectedNodeIds, tldrawLicenseKey]);

    const getConnectionCreateDisabledReason = useCallback((type: CanvasNodeType.Image | CanvasNodeType.Text | CanvasNodeType.Script | CanvasNodeType.Video | CanvasNodeType.Audio | CanvasNodeType.Drawing, pending: PendingConnectionCreate) => {
        if (pending.batchSourceNodeIds?.length) {
            if (type === CanvasNodeType.Drawing) return "批量连接暂不支持绘图";
            const spec = getNodeSpec(type);
            const pendingNode: CanvasNodeData = { id: "__pending-connection-node__", type, title: "", position: pending.position, width: spec.width, height: spec.height };
            const plan = planBatchConnections({ sourceNodeIds: pending.batchSourceNodeIds, targetNodeId: pendingNode.id, nodes: [...nodesRef.current, pendingNode], connections: connectionsRef.current, config, allowCapacityOverflow: true });
            return plan.connections.length ? "" : plan.skipped[0]?.reason || "当前选中的节点不能连接到此类型";
        }
        const spec = getNodeSpec(type);
        const pendingNode: CanvasNodeData = { id: "__pending-connection-node__", type, title: "", position: pending.position, width: spec.width, height: spec.height };
        const pendingNodes = [...nodesRef.current, pendingNode];
        const connection = normalizeConnection(pending.connection.nodeId, pendingNode.id, pendingNodes, pending.connection.handleType);
        if (!connection) return "当前节点类型不能这样连接";
        return canvasConnectionError(config, pendingNodes, connectionsRef.current, connection);
    }, [config, connectionsRef, nodesRef]);

    const getConnectionDropTarget = useCallback((clientX: number, clientY: number, current: ConnectionHandle): ConnectionDropTarget => {
        const world = screenToCanvas(clientX, clientY);
        const scale = Math.max(viewportRef.current.k, 0.05);
        const padding = CONNECTION_NODE_HIT_PADDING / scale;
        const handleRadius = CONNECTION_HANDLE_HIT_RADIUS / scale;
        let isNearNode = false;
        let bestNodeId: string | null = null;
        let bestHandleId: string | undefined;
        let bestAnchorRatio: number | undefined;
        let bestPriority = Number.POSITIVE_INFINITY;

        [...nodesRef.current]
            .filter((node) => !isHiddenBatchChild(node, nodesRef.current) && !isNodeHiddenByCollapsedFrame(node, nodesRef.current) && !isFrameNode(node))
            .reverse()
            .forEach((node) => {
                const scrollTop = scriptScrollTopById[node.id] || 0;
                const targetHandleId = node.type === CanvasNodeType.Script ? storyboardHandleAtY(node, world.y, scrollTop) : undefined;
                if (node.type === CanvasNodeType.Script && !targetHandleId) return;
                const targetAnchorRatio = node.type === CanvasNodeType.Script ? undefined : Math.min(0.94, Math.max(0.06, (world.y - node.position.y) / Math.max(node.height, 1)));
                const anchor = getConnectionTargetAnchor(node, current, targetHandleId, scrollTop, targetAnchorRatio);
                const dx = world.x - anchor.x;
                const dy = world.y - anchor.y;
                const hitsHandle = dx * dx + dy * dy <= handleRadius * handleRadius;
                const hitsInside = world.x >= node.position.x && world.x <= node.position.x + node.width && world.y >= node.position.y && world.y <= node.position.y + node.height;
                const hitsExpanded = world.x >= node.position.x - padding && world.x <= node.position.x + node.width + padding && world.y >= node.position.y - padding && world.y <= node.position.y + node.height + padding;
                if (!hitsHandle && !hitsInside && !hitsExpanded) return;
                isNearNode = true;
                const normalized = node.id === current.nodeId ? null : normalizeConnection(current.nodeId, node.id, nodesRef.current, current.handleType);
                if (!normalized || canvasConnectionError(config, nodesRef.current, connectionsRef.current, normalized)) return;
                const priority = hitsInside ? 0 : hitsHandle ? 1 : 2;
                if (priority < bestPriority) {
                    bestNodeId = node.id;
                    bestHandleId = targetHandleId;
                    bestAnchorRatio = targetAnchorRatio;
                    bestPriority = priority;
                }
            });
        return { nodeId: bestNodeId, handleId: bestHandleId, anchorRatio: bestAnchorRatio, isNearNode };
    }, [config, connectionsRef, nodesRef, screenToCanvas, scriptScrollTopById, viewportRef]);

    const getBatchConnectionDropTarget = useCallback((clientX: number, clientY: number, sourceNodeIds: string[]): ConnectionDropTarget => {
        const world = screenToCanvas(clientX, clientY);
        const scale = Math.max(viewportRef.current.k, 0.05);
        const padding = CONNECTION_NODE_HIT_PADDING / scale;
        const handleRadius = CONNECTION_HANDLE_HIT_RADIUS / scale;
        let isNearNode = false;
        let bestNodeId: string | null = null;
        let bestHandleId: string | undefined;
        let bestAnchorRatio: number | undefined;
        let bestPriority = Number.POSITIVE_INFINITY;

        [...nodesRef.current]
            .filter((node) => !sourceNodeIds.includes(node.id) && !isHiddenBatchChild(node, nodesRef.current) && !isNodeHiddenByCollapsedFrame(node, nodesRef.current) && !isFrameNode(node))
            .reverse()
            .forEach((node) => {
                const scrollTop = scriptScrollTopById[node.id] || 0;
                const targetHandleId = node.type === CanvasNodeType.Script ? storyboardHandleAtY(node, world.y, scrollTop) : undefined;
                if (node.type === CanvasNodeType.Script && !targetHandleId) return;
                const targetAnchorRatio = node.type === CanvasNodeType.Script ? undefined : Math.min(0.94, Math.max(0.06, (world.y - node.position.y) / Math.max(node.height, 1)));
                const source = nodesRef.current.find((item) => sourceNodeIds.includes(item.id) && !batchSourceRestriction(item));
                if (!source) return;
                const anchor = getConnectionTargetAnchor(node, { nodeId: source.id, handleType: "source" }, targetHandleId, scrollTop, targetAnchorRatio);
                const dx = world.x - anchor.x;
                const dy = world.y - anchor.y;
                const hitsHandle = dx * dx + dy * dy <= handleRadius * handleRadius;
                const hitsInside = world.x >= node.position.x && world.x <= node.position.x + node.width && world.y >= node.position.y && world.y <= node.position.y + node.height;
                const hitsExpanded = world.x >= node.position.x - padding && world.x <= node.position.x + node.width + padding && world.y >= node.position.y - padding && world.y <= node.position.y + node.height + padding;
                if (!hitsHandle && !hitsInside && !hitsExpanded) return;
                isNearNode = true;
                if (!hasBatchConnectionCandidate(sourceNodeIds, node.id, nodesRef.current)) return;
                const plan = planBatchConnections({ sourceNodeIds, targetNodeId: node.id, targetHandleId, targetAnchorRatio, nodes: nodesRef.current, connections: connectionsRef.current, config });
                if (!plan.connections.length) return;
                const priority = hitsInside ? 0 : hitsHandle ? 1 : 2;
                if (priority < bestPriority) {
                    bestNodeId = node.id;
                    bestHandleId = targetHandleId;
                    bestAnchorRatio = targetAnchorRatio;
                    bestPriority = priority;
                }
            });
        return { nodeId: bestNodeId, handleId: bestHandleId, anchorRatio: bestAnchorRatio, isNearNode };
    }, [config, connectionsRef, nodesRef, screenToCanvas, scriptScrollTopById, viewportRef]);

    const beginBatchConnectionMode = useCallback((sourceNodeIds: string[]) => {
        const uniqueSourceNodeIds = Array.from(new Set(sourceNodeIds)).filter((nodeId) => {
            const node = nodesRef.current.find((item) => item.id === nodeId);
            return Boolean(node && !batchSourceRestriction(node));
        });
        if (uniqueSourceNodeIds.length < 2) {
            message.warning("请至少选择两个可作为连接源的节点");
            return;
        }
        const sourceNodes = uniqueSourceNodeIds.map((nodeId) => nodesRef.current.find((node) => node.id === nodeId)).filter((node): node is CanvasNodeData => Boolean(node));
        const sourceCenter = sourceNodes.reduce((center, node) => ({ x: center.x + node.position.x + node.width / 2, y: center.y + node.position.y + node.height / 2 }), { x: 0, y: 0 });
        const mouse = { x: sourceCenter.x / sourceNodes.length, y: sourceCenter.y / sourceNodes.length };
        closeConnectionCreateMenu();
        setConnecting(null);
        setContextMenu(null);
        previewBatchConnection(uniqueSourceNodeIds, null, undefined, undefined, mouse);
        message.info("选择目标节点，或点击空白处创建下一步");
    }, [closeConnectionCreateMenu, message, nodesRef, previewBatchConnection, setConnecting, setContextMenu]);

    const handleBatchConnectionTargetClick = useCallback((event: ReactMouseEvent, nodeId: string) => {
        const batch = batchConnectionPreviewRef.current;
        if (!batch || event.button !== 0) return false;
        const target = getBatchConnectionDropTarget(event.clientX, event.clientY, batch.sourceNodeIds);
        if (!target.nodeId || target.nodeId !== nodeId) {
            message.warning("当前节点不能作为这些节点的批量连接目标");
            clearBatchConnection();
            return true;
        }
        commitBatchConnection(batch.sourceNodeIds, target.nodeId, target.handleId, target.anchorRatio);
        clearBatchConnection();
        return true;
    }, [clearBatchConnection, commitBatchConnection, getBatchConnectionDropTarget, message]);

    const handleBatchConnectionCanvasClick = useCallback((event: ReactPointerEvent<HTMLDivElement>) => {
        const batch = batchConnectionPreviewRef.current;
        if (!batch || event.button !== 0) return false;
        const position = screenToCanvas(event.clientX, event.clientY);
        const request = buildBatchConnectionCreateRequest(batch.sourceNodeIds, nodesRef.current, position);
        if (!request) {
            message.warning("找不到可用于批量连接的源节点");
            clearBatchConnection();
            return true;
        }
        const pending: PendingConnectionCreate = { ...request };
        pendingConnectionCreateRef.current = pending;
        setPendingConnectionCreate(pending);
        setContextMenu(null);
        clearBatchConnection();
        return true;
    }, [clearBatchConnection, message, nodesRef, screenToCanvas, setContextMenu]);

    useEffect(() => {
        const handlePointerMove = (event: PointerEvent) => {
            const batch = batchConnectionPreviewRef.current;
            if (!batch || pendingConnectionCreateRef.current) return;
            const target = getBatchConnectionDropTarget(event.clientX, event.clientY, batch.sourceNodeIds);
            previewBatchConnection(batch.sourceNodeIds, target.nodeId, target.handleId, target.anchorRatio, screenToCanvas(event.clientX, event.clientY));
        };
        window.addEventListener("pointermove", handlePointerMove);
        return () => window.removeEventListener("pointermove", handlePointerMove);
    }, [getBatchConnectionDropTarget, previewBatchConnection, screenToCanvas]);

    const finishConnection = useCallback((clientX: number, clientY: number) => {
        if (pendingConnectionCreateRef.current) return;
        const currentConnection = connectingParamsRef.current;
        if (!currentConnection) return;
        const dropTarget = getConnectionDropTarget(clientX, clientY, currentConnection);
        if (dropTarget.nodeId) {
            connectNodes(currentConnection, dropTarget.nodeId, dropTarget.handleId, dropTarget.anchorRatio);
            setConnecting(null);
        } else if (dropTarget.isNearNode) {
            setConnecting(null);
        } else {
            const position = screenToCanvas(clientX, clientY);
            setMouseWorld(position);
            const pending = { connection: currentConnection, position };
            pendingConnectionCreateRef.current = pending;
            setPendingConnectionCreate(pending);
        }
    }, [connectNodes, getConnectionDropTarget, screenToCanvas, setConnecting]);

    const handleConnectStart = useCallback((event: ReactPointerEvent, nodeId: string, handleType: "source" | "target", handleId?: string, anchorRatio?: number) => {
        event.preventDefault();
        event.stopPropagation();
        clearBatchConnection();
        connectingPointerIdRef.current = event.pointerId;
        connectingPointerStartRef.current = { x: event.clientX, y: event.clientY };
        setMouseWorld(screenToCanvas(event.clientX, event.clientY));
        setConnecting({ nodeId, handleType, handleId, anchorRatio });
        setConnectionTargetNodeId(null);
        setConnectionTargetAnchorRatio(undefined);
        setSelectedConnectionId(null);
    }, [clearBatchConnection, screenToCanvas, setConnecting, setSelectedConnectionId]);

    useEffect(() => {
        const handlePointerMove = (event: PointerEvent) => {
            const current = connectingParamsRef.current;
            if (!current || connectingPointerIdRef.current !== event.pointerId || pendingConnectionCreateRef.current) return;
            const dropTarget = getConnectionDropTarget(event.clientX, event.clientY, current);
            setConnectionTargetNodeId(dropTarget.nodeId);
            setConnectionTargetAnchorRatio(dropTarget.anchorRatio);
            setMouseWorld(screenToCanvas(event.clientX, event.clientY));
        };
        const handlePointerUp = (event: PointerEvent) => {
            if (connectingPointerIdRef.current !== event.pointerId) return;
            const start = connectingPointerStartRef.current;
            if (start && Math.hypot(event.clientX - start.x, event.clientY - start.y) <= 5) {
                const current = connectingParamsRef.current;
                if (current) {
                    const pending = { connection: current, position: screenToCanvas(event.clientX, event.clientY), quick: true };
                    pendingConnectionCreateRef.current = pending;
                    setPendingConnectionCreate(pending);
                    setConnecting(null);
                    return;
                }
            }
            finishConnection(event.clientX, event.clientY);
        };
        const handlePointerCancel = (event: PointerEvent) => {
            if (connectingPointerIdRef.current === event.pointerId) setConnecting(null);
        };
        const cancel = () => {
            if (connectingParamsRef.current) setConnecting(null);
        };
        window.addEventListener("pointermove", handlePointerMove);
        window.addEventListener("pointerup", handlePointerUp);
        window.addEventListener("pointercancel", handlePointerCancel);
        window.addEventListener("blur", cancel);
        return () => {
            window.removeEventListener("pointermove", handlePointerMove);
            window.removeEventListener("pointerup", handlePointerUp);
            window.removeEventListener("pointercancel", handlePointerCancel);
            window.removeEventListener("blur", cancel);
        };
    }, [finishConnection, getConnectionDropTarget, screenToCanvas, setConnecting]);

    return {
        batchConnectionPreview,
        beginBatchConnectionMode,
        cancelPendingConnectionCreate,
        closeConnectionCreateMenu,
        connectionTargetNodeId,
        connectionTargetAnchorRatio,
        connectingParams,
        createConnectedNode,
        getConnectionCreateDisabledReason,
        handleBatchConnectionCanvasClick,
        handleBatchConnectionTargetClick,
        handleConnectStart,
        mouseWorld,
        pendingConnectionCreate,
        setConnecting,
    };
}
