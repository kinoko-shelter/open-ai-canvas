import type { ModelProtocol } from "@/lib/model-protocols";

export type ModelCapabilityConfig = {
    version: number;
    image?: ImageCapabilityConfig;
    video?: VideoCapabilityConfig;
};

export type ImageSizeParameter = "none" | "size" | "aspect_ratio";

export type ImageCapabilityConfig = {
    references: {
        promptMaxChars: number;
        maxImages: number;
        maxImageBytes: number;
        maskSupported: boolean;
    };
    size: {
        parameter: ImageSizeParameter;
        values: string[];
        default: string;
        allowCustom: boolean;
    };
    quality: {
        supported: boolean;
        values: string[];
        default: string;
    };
    transparentBackground: { supported: boolean; default: boolean };
    responseFormat: { supported: boolean };
    outputFormat: { supported: boolean };
    maxOutputs: number;
};

export type VideoCapabilityConfig = {
    references: {
        promptMaxChars: number;
        minImages: number;
        maxImages: number;
        maxImageBytes: number;
        maxVideos: number;
        maxVideoBytes: number;
        maxVideoDurationSeconds: number;
        maxAudios: number;
        maxAudioBytes: number;
        maxAudioDurationSeconds: number;
    };
    duration: {
        selection: "range" | "enum";
        min?: number;
        max?: number;
        step?: number;
        values?: number[];
        default: number;
    };
    ratios: string[];
    defaultRatio: string;
    resolutions: string[];
    defaultResolution: string;
    generateAudio: { supported: boolean; default: boolean };
    watermark: { supported: boolean; default: boolean };
    operations: string[];
    defaultOperation: string;
};

const defaultImageSizes = ["1:1", "3:2", "2:3", "4:3", "3:4", "5:4", "4:5", "16:9", "21:9", "9:16", "2048x2048", "2048x1152", "1152x2048", "3840x2160", "2160x3840"];
const gptImage2Ratios = ["1:1", "3:2", "2:3", "4:3", "3:4", "5:4", "4:5", "16:9", "9:16", "21:9"];
const geminiImageRatios = ["1:1", "16:9", "9:16", "4:3", "3:4", "21:9", "3:2", "2:3", "5:4", "4:5"];
const geminiNano2ExtraRatios = ["1:4", "1:8", "4:1", "8:1"];

export function isGptImage2Model(model: string) {
    return model.trim().toLowerCase().startsWith("gpt-image-2");
}

function isGeminiImageModel(model: string) {
    const value = model.trim().toLowerCase();
    return (value.startsWith("gemini-") && value.includes("image")) || value.includes("nano-banana");
}

function usesGeminiImageProfile(protocol: ModelProtocol | undefined, model: string, apiFormat: "openai" | "gemini" | undefined) {
    return protocol === "gemini-image" || (apiFormat === "gemini" && isGeminiImageModel(model));
}

export function hasModelSpecificImageCapability(protocol: ModelProtocol | undefined, model: string, apiFormat?: "openai" | "gemini") {
    return isGptImage2Model(model) || usesGeminiImageProfile(protocol, model, apiFormat);
}

function geminiImageRatioValues(model: string) {
    const value = model.trim().toLowerCase();
    return value.startsWith("gemini-3.1-flash-image-preview") ? [...geminiImageRatios, ...geminiNano2ExtraRatios] : geminiImageRatios;
}

export function defaultImageCapabilityConfig(protocol?: ModelProtocol, model = "", apiFormat?: "openai" | "gemini"): ImageCapabilityConfig {
    const image: ImageCapabilityConfig = {
        references: { promptMaxChars: 32000, maxImages: 16, maxImageBytes: 30 * 1024 * 1024, maskSupported: true },
        size: { parameter: "size", values: [...defaultImageSizes], default: "1:1", allowCustom: true },
        quality: { supported: true, values: ["auto", "low", "medium", "high"], default: "auto" },
        transparentBackground: { supported: true, default: false },
        responseFormat: { supported: true },
        outputFormat: { supported: true },
        maxOutputs: 15,
    };
    if (protocol === "grok-image") {
        image.references.maxImages = 1;
        image.references.maskSupported = false;
        // grok2api / xAI Imagine：size→aspect_ratio，quality→resolution(1k/2k)。
        image.size = {
            parameter: "aspect_ratio",
            values: ["1:1", "3:4", "4:3", "9:16", "16:9", "2:3", "3:2"],
            default: "1:1",
            allowCustom: false,
        };
        image.quality = { supported: true, values: ["1k", "2k"], default: "2k" };
        image.transparentBackground = { supported: false, default: false };
        image.responseFormat = { supported: true };
        image.outputFormat = { supported: false };
        image.maxOutputs = 1;
    } else if (protocol === "volcengine-ark-image") {
        image.references.maskSupported = false;
        image.quality.supported = false;
        image.transparentBackground.supported = false;
        image.responseFormat.supported = false;
        image.outputFormat.supported = false;
    }
    if (protocol === "volcengine-jimeng-image") {
        image.references.maxImages = 14;
        image.references.maskSupported = false;
        image.quality.supported = false;
        image.transparentBackground.supported = false;
        image.responseFormat.supported = false;
        image.outputFormat.supported = false;
    }
    if (protocol !== "grok-image" && model.trim().toLowerCase().startsWith("grok-imagine-image")) {
        image.references.maxImages = 0;
        image.references.maskSupported = false;
        image.size = {
            parameter: "aspect_ratio",
            values: ["1:1", "3:4", "4:3", "9:16", "16:9", "2:3", "3:2"],
            default: "1:1",
            allowCustom: false,
        };
        image.quality = { supported: true, values: ["1k", "2k"], default: "2k" };
        image.transparentBackground = { supported: false, default: false };
        image.responseFormat = { supported: true };
        image.outputFormat = { supported: false };
        image.maxOutputs = 1;
    }
    if (isGptImage2Model(model)) {
        image.size = { parameter: "size", values: ["auto", ...gptImage2Ratios], default: "auto", allowCustom: true };
        image.quality = { supported: true, values: ["1k", "2k", "4k"], default: "1k" };
    }
    if (usesGeminiImageProfile(protocol, model, apiFormat)) {
        image.size = { parameter: "aspect_ratio", values: geminiImageRatioValues(model), default: "1:1", allowCustom: false };
        image.quality = { supported: false, values: [], default: "auto" };
        image.transparentBackground = { supported: false, default: false };
        image.responseFormat = { supported: false };
        image.outputFormat = { supported: false };
    }
    return image;
}

function applyModelSpecificImageCapability(image: ImageCapabilityConfig, protocol: ModelProtocol | undefined, model: string, apiFormat: "openai" | "gemini" | undefined) {
    const canonical = defaultImageCapabilityConfig(protocol, model, apiFormat);
    if (isGptImage2Model(model)) {
        return {
            ...image,
            size: withCanonicalDefault(image.size, canonical.size),
            quality: withCanonicalDefault(image.quality, canonical.quality),
        };
    }
    if (usesGeminiImageProfile(protocol, model, apiFormat)) {
        return {
            ...image,
            references: { ...image.references, maskSupported: false },
            size: withCanonicalDefault(image.size, canonical.size),
            quality: withCanonicalDefault(image.quality, canonical.quality),
            transparentBackground: canonical.transparentBackground,
            responseFormat: canonical.responseFormat,
            outputFormat: canonical.outputFormat,
        };
    }
    return image;
}

function withCanonicalDefault<T extends { values: string[]; default: string }>(current: T, canonical: T) {
    return { ...canonical, default: canonical.values.includes(current.default) ? current.default : canonical.default };
}

export function defaultModelCapabilityConfig(protocol?: ModelProtocol, model = "", apiFormat?: "openai" | "gemini"): ModelCapabilityConfig {
    const video: VideoCapabilityConfig = {
        references: {
            promptMaxChars: 1000,
            minImages: 0,
            maxImages: 9,
            maxImageBytes: 30 * 1024 * 1024,
            maxVideos: 0,
            maxVideoBytes: 0,
            maxVideoDurationSeconds: 0,
            maxAudios: 0,
            maxAudioBytes: 0,
            maxAudioDurationSeconds: 0,
        },
        duration: { selection: "range", min: 1, max: 15, step: 1, default: 6 },
        ratios: ["16:9", "9:16", "1:1", "4:3", "3:4", "21:9"],
        defaultRatio: "16:9",
        resolutions: ["480p", "720p", "1080p", "1440p", "2160p"],
        defaultResolution: "720p",
        generateAudio: { supported: false, default: false },
        watermark: { supported: false, default: false },
        operations: ["text_to_video", "image_to_video"],
        defaultOperation: "text_to_video",
    };
    if (protocol === "volcengine-jimeng-video") video.duration = { selection: "enum", values: [5, 10], default: 5 };
    if (protocol === "gemini-veo") {
        video.duration = { selection: "enum", values: [4, 6, 8], default: 6 };
        video.resolutions = ["720p", "1080p"];
    }
    if (protocol === "volcengine-ark-video" || protocol === "newapi-channel-1" || protocol === "newapi-channel-2") {
        video.references.maxVideos = 3;
        video.references.maxAudios = 3;
        video.references.maxVideoBytes = 200 * 1024 * 1024;
        video.references.maxAudioBytes = 15 * 1024 * 1024;
        video.references.maxVideoDurationSeconds = 15;
        video.references.maxAudioDurationSeconds = 15;
        video.generateAudio = { supported: true, default: true };
    }
    if (protocol === "volcengine-ark-video" || protocol === "newapi-channel-1") video.resolutions = ["480p", "720p", "1080p"];
    if (protocol === "volcengine-ark-video") video.watermark = { supported: true, default: false };
    if (protocol === "novita-video") {
        video.references.maxImages = 1;
        video.references.maxImageBytes = 10 * 1024 * 1024;
        video.duration = { selection: "enum", values: [5, 10], default: 5 };
        video.ratios = ["16:9", "9:16", "1:1"];
        video.resolutions = ["1080p"];
        video.defaultResolution = "1080p";
    }
    const fixedGrokVideoResolution = grokVideoResolutionFromModel(model);
    if (fixedGrokVideoResolution) {
        video.resolutions = [fixedGrokVideoResolution];
        video.defaultResolution = fixedGrokVideoResolution;
    }
    return { version: 1, image: defaultImageCapabilityConfig(protocol, model, apiFormat), video };
}

function grokVideoResolutionFromModel(model: string) {
    const match = model.trim().match(/^grok-imagine-video-[\w.-]+-(480p|720p|1080p)$/i);
    return match ? match[1].toLowerCase() : "";
}

export function normalizeModelCapabilityConfig(input: ModelCapabilityConfig | undefined, protocol?: ModelProtocol, model = "", apiFormat?: "openai" | "gemini") {
    const fallback = defaultModelCapabilityConfig(protocol, model, apiFormat);
    const resolved = input
        ? {
              ...fallback,
              ...input,
              image: input.image || fallback.image,
              video: input.video
                  ? { ...fallback.video!, ...input.video, references: { ...fallback.video!.references, ...input.video.references } }
                  : fallback.video,
          }
        : fallback;
    if (resolved.image) resolved.image = applyModelSpecificImageCapability(resolved.image, protocol, model, apiFormat);
    const fixedGrokVideoResolution = grokVideoResolutionFromModel(model);
    if (fixedGrokVideoResolution && resolved.video) {
        resolved.video = { ...resolved.video, resolutions: [fixedGrokVideoResolution], defaultResolution: fixedGrokVideoResolution };
    }
    return resolved;
}

export function modelCapabilityConfigFor(
    config: { channels: Array<{ id: string; models: string[]; apiFormat?: "openai" | "gemini"; interfaceType?: ModelProtocol; modelCosts?: Array<{ model: string; capabilityConfig?: ModelCapabilityConfig; protocol?: ModelProtocol }> }> },
    model: string,
) {
    const separator = model.indexOf("::");
    const channelId = separator >= 0 ? model.slice(0, separator) : "";
    const modelName = separator >= 0 ? model.slice(separator + 2) : model;
    const channel = config.channels.find((item) => item.id === channelId) || config.channels.find((item) => item.models.includes(modelName));
    const cost = channel?.modelCosts?.find((item) => item.model === modelName);
    return normalizeModelCapabilityConfig(cost?.capabilityConfig, cost?.protocol || channel?.interfaceType, modelName, channel?.apiFormat);
}

export function normalizeImageValue(profile: ImageCapabilityConfig, value: { size?: string; quality?: string; count?: string; transparentBackground?: string }) {
    const size = normalizeImageSizeSetting(profile, value.size);
    const quality = profile.quality.supported
        ? (value.quality && profile.quality.values.includes(value.quality) ? value.quality : profile.quality.default || "auto")
        : profile.quality.default || "auto";
    const count = String(Math.max(1, Math.min(profile.maxOutputs, Math.floor(Math.abs(Number(value.count)) || 1))));
    const transparentBackground = profile.transparentBackground.supported && value.transparentBackground === "true" ? "true" : "false";
    return { size, quality, count, transparentBackground };
}

export function normalizeImageSizeSetting(profile: ImageCapabilityConfig, value?: string) {
    if (profile.size.parameter === "none") return "auto";
    const candidate = value?.trim() || profile.size.default;
    if (profile.size.allowCustom || profile.size.values.includes(candidate)) return candidate;
    return profile.size.default || profile.size.values[0] || "auto";
}

export function imageSizeRequest(profile: ImageCapabilityConfig, value?: string) {
    const parameter = profile.size.parameter;
    if (parameter === "none") return undefined;
    const normalized = normalizeImageSizeSetting(profile, value);
    if (!normalized || normalized === "auto") return undefined;
    return { parameter, value: normalized };
}

export function normalizeVideoValue(profile: VideoCapabilityConfig, value: { seconds?: string; ratio?: string; resolution?: string }) {
    const duration = profile.duration.selection === "enum"
        ? (profile.duration.values || []).includes(Number(value.seconds)) ? Number(value.seconds) : profile.duration.default
        : normalizeRangeDuration(profile, Number(value.seconds));
    const ratio = profile.ratios.includes(value.ratio || "") ? value.ratio! : profile.defaultRatio;
    const resolution = profile.resolutions.includes(value.resolution || "") ? value.resolution! : profile.defaultResolution;
    return { seconds: String(duration), ratio, resolution };
}

export function videoResolutionRequest(profile: VideoCapabilityConfig, value: string | undefined) {
    const requested = String(value || "").trim().toLowerCase();
    if (!requested || requested === "auto" || requested === "default" || requested === "medium" || requested === "high") return undefined;
    const candidates = [requested];
    if (/^\d+$/.test(requested)) candidates.push(`${requested}p`);
    if (requested === "low") candidates.push("480p");
    if (requested === "2k") candidates.push("1440p");
    if (requested === "1440" || requested === "1440p") candidates.push("2k");
    if (requested === "4k") candidates.push("2160p");
    if (requested === "2160" || requested === "2160p") candidates.push("4k");
    const supported = new Map(profile.resolutions.map((resolution) => [resolution.trim().toLowerCase(), resolution.trim()]));
    for (const candidate of candidates) {
        const match = supported.get(candidate);
        if (match) return match;
    }
    return undefined;
}

function normalizeRangeDuration(profile: VideoCapabilityConfig, value: number) {
    const min = profile.duration.min || 1;
    const max = profile.duration.max || min;
    const step = profile.duration.step || 1;
    const candidate = Number.isFinite(value) ? Math.floor(value) : profile.duration.default;
    const clamped = Math.min(max, Math.max(min, candidate));
    const maxStep = Math.max(0, Math.floor((max - min) / step));
    return min + Math.min(maxStep, Math.max(0, Math.round((clamped - min) / step))) * step;
}

export function videoDurationOptions(profile: VideoCapabilityConfig) {
    if (profile.duration.selection === "enum") return profile.duration.values || [];
    const min = profile.duration.min || 1;
    const max = profile.duration.max || min;
    const step = profile.duration.step || 1;
    return Array.from({ length: Math.floor((max - min) / step) + 1 }, (_, index) => min + index * step);
}

export function videoDurationAllowed(profile: VideoCapabilityConfig, value: number) {
    if (profile.duration.selection === "enum") return (profile.duration.values || []).includes(value);
    const min = profile.duration.min || 1;
    const max = profile.duration.max || min;
    const step = profile.duration.step || 1;
    return value >= min && value <= max && (value - min) % step === 0;
}
