import type { UploadedFile } from "@/services/file-storage";
import type { UploadedImage } from "@/services/image-storage";
import type { Asset, ImageAsset } from "@/stores/use-asset-store";
import type { ReferenceImage } from "@/types/image";
import type { ReferenceVideo } from "@/types/media";

export { creationAssetKey, creationImageAsset, creationVideoAsset, isSameCreationAsset, type CreationAssetIdentity } from "@/lib/generation-media-assets";

export type CreationAttachment = (ReferenceImage | ReferenceVideo) & { previewUrl: string };

export function creationAttachmentFromImage(file: File, uploaded: UploadedImage): CreationAttachment {
    return {
        id: `upload:${file.name}:${uploaded.storageKey}`,
        name: file.name,
        type: uploaded.mimeType || file.type || "image/png",
        dataUrl: uploaded.url,
        url: uploaded.url,
        storageKey: uploaded.storageKey,
        bytes: uploaded.bytes,
        width: uploaded.width,
        height: uploaded.height,
        previewUrl: uploaded.url,
    };
}

export function creationAttachmentFromVideo(file: File, uploaded: UploadedFile): CreationAttachment {
    return {
        id: `upload:${file.name}:${uploaded.storageKey}`,
        name: file.name,
        type: uploaded.mimeType || file.type || "video/mp4",
        url: uploaded.url,
        storageKey: uploaded.storageKey,
        bytes: uploaded.bytes,
        width: uploaded.width,
        height: uploaded.height,
        durationMs: uploaded.durationMs,
        previewUrl: uploaded.url,
    };
}

export function creationAttachmentFromAsset(asset: ImageAsset): CreationAttachment {
    const url = asset.data.dataUrl || asset.coverUrl;
    return {
        id: `asset:${asset.id}`,
        name: asset.title || "素材图片",
        type: asset.data.mimeType || "image/png",
        dataUrl: url,
        url,
        storageKey: asset.data.storageKey,
        bytes: asset.data.bytes,
        width: asset.data.width,
        height: asset.data.height,
        previewUrl: url,
    };
}

export function creationAttachmentFromVideoAsset(asset: Extract<Asset, { kind: "video" }>): CreationAttachment {
    return {
        id: `asset:${asset.id}`,
        name: asset.title || "素材视频",
        type: asset.data.mimeType || "video/mp4",
        url: asset.data.url,
        storageKey: asset.data.storageKey,
        bytes: asset.data.bytes,
        width: asset.data.width,
        height: asset.data.height,
        durationMs: asset.data.durationMs,
        previewUrl: asset.coverUrl || asset.data.url,
    };
}
