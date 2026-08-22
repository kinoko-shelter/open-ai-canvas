import { getFeatureAvailability, type AuthSessionPayload } from "@/services/api/auth";
import { getModelCatalog, type CapabilitySpec, type ModelCatalogResponse, type OptionConstraint, type PublicChannelCatalog, type PublicLogicalModel } from "@/services/api/logical-models";
import { localForageStorage } from "@/lib/localforage-storage";
import { appQueryClient } from "@/lib/query-client";
import { scopedLocalStorage, setActiveUserScope } from "@/lib/user-scope";
import { CANVAS_STORE_KEY, flushCanvasStorePersistence, useCanvasStore } from "@/stores/canvas/use-canvas-store";
import { ASSET_STORE_KEY, useAssetStore } from "@/stores/use-asset-store";
import { CONFIG_STORE_KEY, PUBLIC_MODEL_CATALOG_ID, defaultConfig, normalizeConfigSnapshot, useConfigStore, type ModelCapability, type ModelChannel } from "@/stores/use-config-store";
import { defaultModelCapabilityConfig, type ModelCapabilityConfig } from "@/lib/model-capabilities";
import { useUserStore } from "@/stores/use-user-store";
import { installRemoteUserDataAutoSync, resetRemoteUserDataSync, syncRemoteUserData } from "@/services/user-data-sync";

export async function applyUserSession(payload: AuthSessionPayload) {
    const previousUserId = useUserStore.getState().user?.id || "";
    const nextUserId = payload.user?.id || "";
    useUserStore.getState().setHydrated(false);
    try {
        // Query key 不携带用户 ID；身份变化时必须取消并清空旧账号请求，避免跨账号复用内存数据。
        if (previousUserId !== nextUserId) appQueryClient.clear();
        resetRemoteUserDataSync();
        await flushCanvasStorePersistence();
        setActiveUserScope(payload.user?.id);
        const [persistedCanvas, persistedAssets] = await Promise.all([
            localForageStorage.getItem(CANVAS_STORE_KEY),
            localForageStorage.getItem(ASSET_STORE_KEY),
        ]);
        const persistedConfig = scopedLocalStorage.getItem(CONFIG_STORE_KEY);
        useUserStore.getState().setUser(payload.user);
        useUserStore.getState().setCanImpersonateUsers(payload.canImpersonateUsers);
        useUserStore.getState().setImpersonation(payload.impersonation);
        useUserStore.getState().setRuntimeLimits(payload.runtimeLimits);
        useUserStore.getState().setDrawingEngine(payload.drawingEngine);
        useUserStore.getState().setFeatures(payload.features);
        await Promise.all([useCanvasStore.persist.rehydrate(), useAssetStore.persist.rehydrate(), useConfigStore.persist.rehydrate()]);
        // Zustand 在目标 scope 没有快照时会保留旧内存，必须显式恢复该 scope 的空状态。
        if (!persistedCanvas) useCanvasStore.setState({ projects: [] });
        if (!persistedAssets) useAssetStore.setState({ assets: [] });
        const catalog = await getModelCatalog();
        const systemChannels = modelChannelsFromCatalog(catalog);
        if (!persistedConfig) {
            // 只有首次配置缺失时才生成能力推荐；已有配置中的空数组代表用户明确清空。
            const initialSystemConfig = {
                ...defaultConfig,
                channels: systemChannels,
                imageModels: undefined,
                videoModels: undefined,
                textModels: undefined,
                audioModels: undefined,
            };
            useConfigStore.getState().replaceConfig(normalizeConfigSnapshot({ config: initialSystemConfig }).config);
        } else {
            useConfigStore.getState().mergeSystemChannels(systemChannels);
        }
        installRemoteUserDataAutoSync();
        if (payload.user?.id) await syncRemoteUserData(payload.user.id);
        else resetRemoteUserDataSync();
    } finally {
        useUserStore.getState().setHydrated(true);
    }
}

export async function refreshSystemChannels() {
    const catalog = await getModelCatalog();
    useConfigStore.getState().mergeSystemChannels(modelChannelsFromCatalog(catalog));
}

function modelChannelsFromCatalog(catalog: ModelCatalogResponse) {
    if (catalog.source === "frontend") return managedModelChannels(catalog.models || []);
    if (catalog.source === "system") return systemChannelModelChannels(catalog.channels || []);
    return [];
}

function managedModelChannels(models: PublicLogicalModel[]) {
    const availableModels = models.filter((item) => item.available);
    if (!availableModels.length) return [];
    const managed: ModelChannel = {
        id: PUBLIC_MODEL_CATALOG_ID,
        name: "平台模型",
        baseUrl: "/api",
        apiKey: "system",
        apiFormat: "openai",
        scope: "system",
        enabled: true,
        models: availableModels.map((item) => item.id),
        modelAliases: Object.fromEntries(
            availableModels.flatMap((item) => (item.legacyModelIds || []).map((legacyID) => [legacyID, item.id])),
        ),
        modelCosts: availableModels.map((item) => ({
            model: item.id,
            displayName: item.name,
            description: item.description,
            icon: item.icon,
            capability: item.capability,
            pricePolicy: item.pricePolicy,
            billingMode: item.billingMode,
            unitPriceMicrocredits: item.unitPriceMicrocredits,
            inputTokenPriceMicrocredits: item.inputPriceMicrocredits,
            outputTokenPriceMicrocredits: item.outputPriceMicrocredits,
            cachedTokenPriceMicrocredits: item.cachedPriceMicrocredits,
            capabilityConfig: projectLogicalCapability(item.capabilitySpec, item.defaultOptions),
            logicalModelId: item.id,
            logicalCapabilitySpec: item.capabilitySpec,
            logicalCapabilityProfiles: item.capabilityProfiles,
			logicalPriceTiers: item.priceTiers,
            defaultOptions: item.defaultOptions,
        })),
    };
    return [managed];
}

function systemChannelModelChannels(channels: PublicChannelCatalog[]): ModelChannel[] {
    return channels
        .map((channel) => {
            const models = channel.models.filter((item) => item.available);
            if (!models.length) return null;
            return {
                id: channel.id,
                name: channel.displayName || channel.name,
                baseUrl: "/api",
                apiKey: "system",
                apiFormat: "openai" as const,
                scope: "system" as const,
                enabled: true,
                models: models.map((item) => item.modelKey),
                modelAliases: {},
                modelCosts: models.map((item) => {
                    const firstTier = item.priceTiers[0];
                    return {
                        model: item.modelKey,
                        displayName: item.displayName,
                        capability: item.capability as ModelCapability,
                        pricePolicy: "channel" as const,
                        billingMode: firstTier?.billingMode || "fixed_request",
                        unitPriceMicrocredits: firstTier?.unitPriceMicrocredits || 0,
                        inputTokenPriceMicrocredits: firstTier?.inputTokenPriceMicrocredits || 0,
                        outputTokenPriceMicrocredits: firstTier?.outputTokenPriceMicrocredits || 0,
                        cachedTokenPriceMicrocredits: firstTier?.cachedTokenPriceMicrocredits || 0,
                        capabilityConfig: item.capabilityConfig as ModelCapabilityConfig | undefined,
                        logicalPriceTiers: item.priceTiers,
                    };
                }),
            } satisfies ModelChannel;
        })
        .filter((channel): channel is ModelChannel => Boolean(channel));
}

function projectLogicalCapability(spec: CapabilitySpec, defaults: Record<string, unknown>): ModelCapabilityConfig {
    const projected = defaultModelCapabilityConfig();
    if (spec.capability === "image" && projected.image) {
        projected.image.references.maxImages = spec.inputs?.image?.max ?? 0;
        projected.image.references.maskSupported = (spec.inputs?.mask?.max ?? 0) > 0;
        projected.image.size = { parameter: "none", values: [], default: "auto", allowCustom: false };
        projected.image.quality = { supported: false, values: [], default: "auto" };
        projected.image.transparentBackground = { supported: false, default: false };
        applyStringOption(spec.options?.size || spec.options?.aspectRatio, defaults.size, (values, initial) => {
            projected.image!.size = { parameter: "size", values, default: initial, allowCustom: false };
        });
        applyStringOption(spec.options?.quality, defaults.quality, (values, initial) => {
            projected.image!.quality = { supported: true, values, default: initial };
        });
        projected.image.maxOutputs = maxNumericOption(spec.options?.count, 1);
        projected.image.transparentBackground = booleanOption(spec.options?.transparentBackground, defaults.transparentBackground);
    }
    if (spec.capability === "video" && projected.video) {
        projected.video.references.minImages = spec.inputs?.image?.min ?? 0;
        projected.video.references.maxImages = spec.inputs?.image?.max ?? 0;
        projected.video.references.maxVideos = spec.inputs?.video?.max ?? 0;
        projected.video.references.maxAudios = spec.inputs?.audio?.max ?? 0;
        projected.video.operations = spec.operations || [];
        projected.video.defaultOperation = spec.operations?.[0] || "";
        const duration = spec.options?.videoSeconds || spec.options?.duration;
        if (duration?.values?.length) projected.video.duration = { selection: "enum", values: duration.values.map(Number).filter(Number.isFinite), default: Number(defaults.videoSeconds ?? duration.values[0]) };
        else if (duration?.min !== undefined && duration.max !== undefined) projected.video.duration = { selection: "range", min: duration.min, max: duration.max, step: duration.step || 1, default: Number(defaults.videoSeconds ?? duration.min) };
        projected.video.ratios = stringValues(spec.options?.size || spec.options?.aspectRatio);
        projected.video.defaultRatio = String(defaults.size ?? projected.video.ratios[0] ?? "");
        projected.video.resolutions = stringValues(spec.options?.vquality || spec.options?.resolution);
        projected.video.defaultResolution = String(defaults.vquality ?? projected.video.resolutions[0] ?? "");
        projected.video.generateAudio = booleanOption(spec.options?.videoGenerateAudio, defaults.videoGenerateAudio);
        projected.video.watermark = booleanOption(spec.options?.videoWatermark, defaults.videoWatermark);
    }
    return projected;
}

function applyStringOption(option: OptionConstraint | undefined, fallback: unknown, apply: (values: string[], initial: string) => void) {
    const values = stringValues(option);
    if (values.length) apply(values, String(fallback ?? values[0]));
}

function stringValues(option?: OptionConstraint) {
    return (option?.values || []).map(String);
}

function maxNumericOption(option: OptionConstraint | undefined, fallback: number) {
    if (option?.max !== undefined) return option.max;
    const values = (option?.values || []).map(Number).filter(Number.isFinite);
    return values.length ? Math.max(...values) : fallback;
}

function booleanOption(option: OptionConstraint | undefined, fallback: unknown) {
    const supported = (option?.values || []).some((value) => value === true || value === "true");
    return { supported, default: supported && String(fallback) === "true" };
}

export async function refreshFeatureAvailability() {
    const payload = await getFeatureAvailability();
    useUserStore.getState().setFeatures(payload.features);
    return payload.features;
}
