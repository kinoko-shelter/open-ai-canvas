import { modelCapabilityConfigFor, videoDurationAllowed } from "@/lib/model-capabilities";
import { modelOptionName, resolveModelChannel, selectableModelsByCapability, type AiConfig, type ModelCapability } from "@/stores/use-config-store";

export type ModelInputSummary = {
    textCount: number;
    imageCount: number;
    videoCount: number;
    audioCount: number;
    characterCount: number;
};

export type ModelRequirements = {
    capability?: ModelCapability;
    input?: ModelInputSummary;
    videoOperation?: string;
    videoSeconds?: string;
    imageSize?: string;
    options?: Record<string, unknown>;
};

export type DisplayModelGroup = {
    key: string;
    label: string;
    models: string[];
};

export type ModelReferenceLimits = {
    maxImages: number;
    maxVideos: number;
    maxAudios: number;
};

export function groupModelsByDisplayName(config: AiConfig, models: string[]): DisplayModelGroup[] {
    const groups = new Map<string, DisplayModelGroup>();
    models.forEach((model) => {
        const channel = resolveModelChannel(config, model);
        const label = configuredModelDisplayName(config, model);
        const key = `${channel.id}\u0000${label.toLocaleLowerCase()}`;
        const current = groups.get(key);
        if (current) current.models.push(model);
        else groups.set(key, { key, label, models: [model] });
    });
    return Array.from(groups.values());
}

export function configuredModelDisplayName(config: AiConfig, value: string) {
    const model = modelOptionName(value);
    const channel = resolveModelChannel(config, value);
    return channel.modelCosts?.find((item) => item.model === model)?.displayName?.trim() || model;
}

export function modelCompatibilityError(config: AiConfig, model: string, requirements?: ModelRequirements) {
    const capability = requirements?.capability;
    const input = requirements?.input;
    if (!capability || !input) return "";
    const visualInputCount = input.imageCount + input.characterCount;
    const channel = resolveModelChannel(config, model);
    const logicalCost = channel.modelCosts?.find((item) => item.model === modelOptionName(model));
    const logicalSpecs = logicalCost?.logicalCapabilityProfiles?.length ? logicalCost.logicalCapabilityProfiles : logicalCost?.logicalCapabilitySpec ? [logicalCost.logicalCapabilitySpec] : [];
    if (logicalSpecs.length) {
        const publicOptionNames = logicalCost?.logicalCapabilitySpec?.options || {};
        const logicalRequirements = {
            ...requirements,
            options: Object.fromEntries(Object.entries(requirements.options || {}).filter(([name]) => Boolean(publicOptionNames[name]))),
        };
        const errors = logicalSpecs.map((spec) => logicalModelCompatibilityError(spec, logicalRequirements, visualInputCount));
        return errors.some((error) => !error) ? "" : errors[0] || "当前输入不受支持";
    }

    if (capability === "image") {
        if (input.videoCount > 0) return "图片模型不支持参考视频";
        if (input.audioCount > 0) return "图片模型不支持参考音频";
        const maxImages = modelCapabilityConfigFor(config, model).image!.references.maxImages;
        if (visualInputCount > maxImages) return `最多支持 ${maxImages} 张参考图`;
        return "";
    }

    if (capability === "video") {
        const profile = modelCapabilityConfigFor(config, model).video!;
        if (visualInputCount > profile.references.maxImages) return `最多支持 ${profile.references.maxImages} 张参考图`;
        if (visualInputCount < profile.references.minImages) return `至少需要 ${profile.references.minImages} 张参考图`;
        if (input.videoCount > profile.references.maxVideos) return `最多支持 ${profile.references.maxVideos} 个参考视频`;
        if (input.audioCount > profile.references.maxAudios) return `最多支持 ${profile.references.maxAudios} 个参考音频`;
        if (requirements.videoSeconds && !videoDurationAllowed(profile, Number(requirements.videoSeconds))) return "不支持当前视频时长";
        const operation = resolveVideoOperation(input, requirements.videoOperation);
        if (operation !== "concat" && !profile.operations.includes(operation)) return `不支持${videoOperationLabel(operation)}`;
        return "";
    }

    if (capability === "text") {
        return input.audioCount > 0 ? "文本模型不支持参考音频" : "";
    }

    if (input.characterCount > 1) return "角色配音一次只能引用一个角色卡";
    return input.imageCount > 0 || input.videoCount > 0 || input.audioCount > 0 ? "音频模型只接受文本或单个角色卡输入" : "";
}

export function modelRequestOptions(config: AiConfig, capability: ModelCapability) {
    switch (capability) {
        case "image":
            return { size: config.size, quality: config.quality, transparentBackground: config.transparentBackground === "true", count: Number(config.count) };
        case "video":
            return { size: config.size, videoSeconds: Number(config.videoSeconds), vquality: config.vquality, videoGenerateAudio: config.videoGenerateAudio === "true", videoWatermark: config.videoWatermark === "true" };
        case "audio":
            return { audioVoice: config.audioVoice, audioFormat: config.audioFormat, audioSpeed: Number(config.audioSpeed) };
        default:
            return {};
    }
}

function logicalModelCompatibilityError(spec: NonNullable<NonNullable<AiConfig["channels"][number]["modelCosts"]>[number]["logicalCapabilitySpec"]>, requirements: ModelRequirements, visualInputCount: number) {
    if (requirements.capability && spec.capability !== requirements.capability) return "不支持当前生成类型";
    const input = requirements.input;
    const counts: Record<string, number> = {
        text: 0,
        image: visualInputCount,
        video: input?.videoCount || 0,
        audio: input?.audioCount || 0,
    };
    for (const [kind, count] of Object.entries(counts)) {
        const constraint = spec.inputs?.[kind];
        if (!constraint && count > 0) return `不支持${kind}输入`;
        if (constraint && (count < constraint.min || count > constraint.max)) return `${kind}输入需为 ${constraint.min}-${constraint.max} 个`;
    }
    const operation = requirements.capability === "video" && input ? resolveVideoOperation(input, requirements.videoOperation) : requirements.videoOperation;
    if (operation && spec.operations?.length && !spec.operations.includes(operation)) return "不支持当前生成模式";
    const options = { ...requirements.options, ...(requirements.videoSeconds ? { videoSeconds: requirements.videoSeconds } : {}), ...(requirements.imageSize ? { size: requirements.imageSize } : {}) };
    for (const [name, value] of Object.entries(options)) {
        if (value === undefined || value === null || value === "") continue;
        const constraint = spec.options?.[name];
        if (!constraint || !logicalOptionMatches(name, constraint, value)) return logicalOptionError(name);
    }
    return "";
}

function logicalOptionMatches(name: string, constraint: { values?: unknown[]; min?: number; max?: number; step?: number }, value: unknown) {
    if (constraint.values?.length) {
        const requested = normalizeLogicalOptionValue(name, value);
        // 与后端路由一致：`*` 表示渠道允许该参数的任意合法取值。
        return constraint.values.some((candidate) => normalizeLogicalOptionValue(name, candidate) === "*" || normalizeLogicalOptionValue(name, candidate) === requested);
    }
    const numeric = Number(value);
    if (!Number.isFinite(numeric)) return false;
    if (constraint.min !== undefined && numeric < constraint.min) return false;
    if (constraint.max !== undefined && numeric > constraint.max) return false;
    if (constraint.step !== undefined && constraint.min !== undefined) return Math.abs((numeric - constraint.min) / constraint.step - Math.round((numeric - constraint.min) / constraint.step)) < 1e-9;
    return true;
}

function normalizeLogicalOptionValue(name: string, value: unknown) {
    const normalized = String(value).trim().toLowerCase();
    if (name !== "vquality") return normalized;
    if (normalized === "low") return "480p";
    if (["auto", "medium", "high"].includes(normalized)) return "720p";
    if (normalized === "2k") return "1440p";
    if (normalized === "4k") return "2160p";
    const resolution = normalized.replace(/p$/i, "");
    return resolution ? `${resolution}p` : "";
}

function logicalOptionError(name: string) {
    const label: Record<string, string> = {
        size: "尺寸",
        quality: "质量",
        transparentBackground: "透明背景",
        count: "输出数量",
        videoSeconds: "时长",
        vquality: "分辨率",
        videoGenerateAudio: "同步音频",
        videoWatermark: "水印设置",
        audioVoice: "音色",
        audioFormat: "音频格式",
        audioSpeed: "语速",
    };
    return `不支持当前${label[name] || name}`;
}

export function compatibleModelInGroup(config: AiConfig, models: string[], requirements?: ModelRequirements, preferred?: string) {
    const ordered = preferred && models.includes(preferred) ? [preferred, ...models.filter((model) => model !== preferred)] : models;
    return ordered.find((model) => !modelCompatibilityError(config, model, requirements)) || "";
}

export function resolveCompatibleModel(config: AiConfig, selected: string, requirements?: ModelRequirements) {
    if (!requirements?.capability) return selected;
    const options = selectableModelsByCapability(config, requirements.capability);
    if (!options.length) return selected;
    const selectedGroup = groupModelsByDisplayName(config, options).find((group) => group.models.includes(selected));
    if (!selectedGroup) return selected;
    return compatibleModelInGroup(config, selectedGroup.models, requirements, selected);
}

export function maxModelInputCapacity(config: AiConfig, capability: "image" | "video", kind: "image" | "video" | "audio") {
    const options = selectableModelsByCapability(config, capability);
    if (!options.length) return null;
    return options.reduce((maximum, model) => {
        const profile = modelCapabilityConfigFor(config, model);
        const value =
            capability === "image" ? (kind === "image" ? profile.image!.references.maxImages : 0) : kind === "image" ? profile.video!.references.maxImages : kind === "video" ? profile.video!.references.maxVideos : profile.video!.references.maxAudios;
        return Math.max(maximum, value);
    }, 0);
}

export function modelGroupReferenceLimits(config: AiConfig, selected: string, capability: ModelCapability, requirements?: ModelRequirements): ModelReferenceLimits | undefined {
    if (capability !== "image" && capability !== "video") return undefined;
    const options = selectableModelsByCapability(config, capability);
    const selectedGroup = groupModelsByDisplayName(config, options).find((group) => group.models.includes(selected));
    const groupModels = selectedGroup?.models || (selected ? [selected] : []);
    if (!groupModels.length) return undefined;
    const compatibleModels = requirements ? groupModels.filter((model) => !modelCompatibilityError(config, model, requirements)) : groupModels;
    const models = compatibleModels.length ? compatibleModels : groupModels;
    return models.reduce<ModelReferenceLimits>(
        (limits, model) => {
            const profile = modelCapabilityConfigFor(config, model);
            if (capability === "image") {
                return { ...limits, maxImages: Math.max(limits.maxImages, profile.image!.references.maxImages) };
            }
            const references = profile.video!.references;
            return {
                maxImages: Math.max(limits.maxImages, references.maxImages),
                maxVideos: Math.max(limits.maxVideos, references.maxVideos),
                maxAudios: Math.max(limits.maxAudios, references.maxAudios),
            };
        },
        { maxImages: 0, maxVideos: 0, maxAudios: 0 },
    );
}

export function inferVideoOperation(input: ModelInputSummary) {
    const visualInputCount = input.imageCount + input.characterCount;
    if (input.audioCount > 0 && visualInputCount === 0 && input.videoCount === 0) return "audio_to_video";
    if (input.videoCount > 0) return "extend";
    if (visualInputCount > 0) return "image_to_video";
    return "text_to_video";
}

export function resolveVideoOperation(input: ModelInputSummary, storedOperation?: string) {
    if (storedOperation && !["text_to_video", "image_to_video", "audio_to_video", "extend", "reference_to_video"].includes(storedOperation)) return storedOperation;
    return inferVideoOperation(input);
}

function videoOperationLabel(operation: string) {
    if (operation === "text_to_video") return "文生视频";
    if (operation === "image_to_video") return "图生视频";
    if (operation === "audio_to_video") return "音频生视频";
    if (operation === "reference_to_video") return "参考素材生视频";
    if (operation === "extend") return "视频续写";
    return "当前生成模式";
}
