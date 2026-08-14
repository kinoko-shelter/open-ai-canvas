import { creationAssetKey, creationImageAsset, creationVideoAsset, isSameCreationAsset, type CreationAssetIdentity } from "@/lib/generation-media-assets";
import { parseBackendGenerationResult, type BackendGenerationResult } from "@/services/api/generation-task";
import { resourceFileUrl, resourceIdFromStorageKey } from "@/services/api/resources";
import type { GenerationTask } from "@/services/api/task-center";
import { storeGeneratedVideo } from "@/services/api/video";
import { resolveMediaUrl } from "@/services/file-storage";
import { resolveImageUrl, uploadImage, type UploadedImage } from "@/services/image-storage";
import { useAssetStore, type Asset, type NewAsset } from "@/stores/use-asset-store";

type GenerationImageResult = NonNullable<BackendGenerationResult["images"]>[number];
type GenerationVideoResult = NonNullable<BackendGenerationResult["video"]>;
type GenerationTaskMedia =
    | { kind: "image"; resultIndex: number; result: GenerationImageResult }
    | { kind: "video"; resultIndex: number; result: GenerationVideoResult };

export type GenerationMediaCollectionResult = {
    total: number;
    added: number;
    alreadyCollected: number;
};

export function generationTaskMedia(task: GenerationTask): GenerationTaskMedia[] {
    const result = taskResult(task);
    const images = Array.isArray(result?.images) ? result.images.filter(hasImageResult) : [];
    if (images.length) return images.map((image, resultIndex) => ({ kind: "image", resultIndex, result: image }));
    const video = result?.video;
    if (hasVideoResult(video)) return [{ kind: "video", resultIndex: 0, result: video }];
    if (!task.previewUrl) return [];
    if (task.previewKind === "video" || task.type.includes("video")) return [{ kind: "video", resultIndex: 0, result: { dataUrl: task.previewUrl, mimeType: "video/mp4" } }];
    return [{ kind: "image", resultIndex: 0, result: { dataUrl: task.previewUrl, mimeType: "image/png" } }];
}

export function canCollectGenerationTask(task: GenerationTask): boolean {
    return task.status === "succeeded" && generationTaskMedia(task).length > 0;
}

export function isGenerationTaskMediaCollected(task: GenerationTask, assets = useAssetStore.getState().assets): boolean {
    const media = generationTaskMedia(task);
    return media.length > 0 && media.every((item) => assets.some((asset) => isSameCreationAsset(asset, { taskId: task.id, resultIndex: item.resultIndex })));
}

export async function collectGenerationTaskMedia(task: GenerationTask): Promise<GenerationMediaCollectionResult> {
    if (task.status !== "succeeded") throw new Error("仅可收藏已完成任务的生成结果");
    const media = generationTaskMedia(task);
    if (!media.length) throw new Error("任务没有可收藏的图片或视频结果");

    let added = 0;
    let alreadyCollected = 0;
    for (const item of media) {
        const identity = { taskId: task.id, resultIndex: item.resultIndex };
        if (useAssetStore.getState().assets.some((existing) => isSameCreationAsset(existing, identity))) {
            alreadyCollected += 1;
            continue;
        }
        const asset = item.kind === "image"
            ? creationImageAsset({ title: task.prompt.slice(0, 24), uploaded: await persistGenerationImageResult(item.result), metadata: collectionMetadata(task, item.resultIndex) })
            : creationVideoAsset({ title: task.prompt.slice(0, 24), uploaded: await persistGenerationVideoResult(item.result), metadata: collectionMetadata(task, item.resultIndex) });
        if (addGenerationAssetOnce(asset, identity)) added += 1;
        else alreadyCollected += 1;
    }
    return { total: media.length, added, alreadyCollected };
}

export async function persistGenerationImageResult(image: GenerationImageResult): Promise<UploadedImage> {
    const dataUrl = mediaDataUrl(image);
    if (!image.storageKey) {
        if (!dataUrl) throw new Error("图片结果资源不可用");
        return uploadImage(dataUrl);
    }
    const resourceID = resourceIdFromStorageKey(image.storageKey);
    const url = resourceID ? resourceFileUrl(resourceID) : await resolveImageUrl(image.storageKey, dataUrl);
    if (!url) throw new Error("图片结果资源不可用");
    return {
        url,
        storageKey: image.storageKey,
        width: image.width || 1024,
        height: image.height || 1024,
        bytes: image.bytes || 0,
        mimeType: image.mimeType || "image/png",
    };
}

export async function persistGenerationVideoResult(video: GenerationVideoResult) {
    const dataUrl = mediaDataUrl(video);
    if (!video.storageKey) {
        if (!dataUrl) throw new Error("视频结果资源不可用");
        return storeGeneratedVideo({ url: dataUrl, mimeType: video.mimeType || "video/mp4" });
    }
    const resourceID = resourceIdFromStorageKey(video.storageKey);
    const url = resourceID ? resourceFileUrl(resourceID) : await resolveMediaUrl(video.storageKey, dataUrl);
    if (!url) throw new Error("视频结果资源不可用");
    return {
        url,
        storageKey: video.storageKey,
        width: video.width,
        height: video.height,
        durationMs: video.durationMs,
        bytes: video.bytes || 0,
        mimeType: video.mimeType || "video/mp4",
    };
}

function addGenerationAssetOnce(asset: NewAsset, identity: CreationAssetIdentity): boolean {
    const store = useAssetStore.getState();
    const key = creationAssetKey(identity);
    if (key && store.assets.some((existing) => isSameCreationAsset(existing, identity))) return false;
    store.addAsset(key ? { ...asset, metadata: { ...asset.metadata, creationAssetKey: key } } : asset);
    return true;
}

function taskResult(task: GenerationTask): BackendGenerationResult | undefined {
    if (!task.resultJson) return undefined;
    try {
        return parseBackendGenerationResult(task);
    } catch {
        return undefined;
    }
}

function hasImageResult(value: unknown): value is GenerationImageResult {
    return hasStoredMedia(value);
}

function hasVideoResult(value: unknown): value is GenerationVideoResult {
    return hasStoredMedia(value);
}

function hasStoredMedia(value: unknown): value is { dataUrl?: unknown; storageKey?: unknown } {
    if (!value || typeof value !== "object") return false;
    const media = value as { dataUrl?: unknown; storageKey?: unknown };
    return (typeof media.dataUrl === "string" && Boolean(media.dataUrl.trim())) || (typeof media.storageKey === "string" && Boolean(media.storageKey.trim()));
}

function mediaDataUrl(value: { dataUrl?: unknown }) {
    return typeof value.dataUrl === "string" ? value.dataUrl : "";
}

function collectionMetadata(task: GenerationTask, resultIndex: number): Record<string, unknown> {
    return {
        source: "create-generation",
        taskId: task.id,
        conversationId: task.clientContext?.conversationId,
        messageId: task.clientContext?.messageId,
        batchIndex: task.clientContext?.batchIndex,
        resultIndex,
        prompt: task.prompt,
    };
}
