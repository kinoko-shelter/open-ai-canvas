import { apiClient, request } from "@/services/api/request";

export type InputConstraint = { min: number; max: number };
export type OptionConstraint = { values?: unknown[]; min?: number; max?: number; step?: number };
export type CapabilitySpec = {
    version: 1;
    capability: "text" | "image" | "video" | "audio";
    operations?: string[];
    inputs?: Record<string, InputConstraint>;
    options?: Record<string, OptionConstraint>;
};

export type ModelRequestIntent = {
    capability: CapabilitySpec["capability"];
    operation?: string;
    inputs?: Record<string, number>;
    options?: Record<string, unknown>;
};

export type PublicLogicalModel = {
    id: string;
    code: string;
    name: string;
    icon?: string;
    description: string;
    capability: CapabilitySpec["capability"];
    sortOrder: number;
    pricePolicy: "channel" | "unified";
    billingMode: "fixed_request" | "per_second" | "token";
    unitPriceMicrocredits: number;
    inputPriceMicrocredits: number;
    outputPriceMicrocredits: number;
    cachedPriceMicrocredits: number;
	priceTiers: PublicLogicalModelPriceTier[];
    legacyModelIds: string[];
    capabilitySpec: CapabilitySpec;
    capabilityProfiles: CapabilitySpec[];
    defaultOptions: Record<string, unknown>;
    available: boolean;
};

export type PublicLogicalModelPriceTier = {
	selector: Record<string, string>;
	resolution: string;
	videoSeconds: number;
	billingMode: "fixed_request" | "per_second" | "token";
	unitPriceMicrocredits: number;
	inputTokenPriceMicrocredits: number;
	outputTokenPriceMicrocredits: number;
	cachedTokenPriceMicrocredits: number;
};

export type AdminLogicalRoute = {
    id: string;
    channelModelId: string;
    channelId: string;
    channelModelKey: string;
    channelModelName: string;
    enabled: boolean;
    priority: number;
    weight: number;
    available: boolean;
    capabilitySpec: CapabilitySpec;
};

export type AdminLogicalModel = PublicLogicalModel & {
    enabled: boolean;
    activeRevisionId: string;
    revisionVersion: number;
    configurationError?: string;
    availabilityError?: string;
    routes: AdminLogicalRoute[];
};

// 管理端可能短暂读到升级前的响应；集合字段统一归一，避免页面因缺失数组中断渲染。
export function normalizeAdminLogicalModel(model: AdminLogicalModel): AdminLogicalModel {
    return {
        ...model,
        priceTiers: Array.isArray(model.priceTiers) ? model.priceTiers : [],
        legacyModelIds: Array.isArray(model.legacyModelIds) ? model.legacyModelIds : [],
        capabilityProfiles: Array.isArray(model.capabilityProfiles) ? model.capabilityProfiles : [],
        defaultOptions: model.defaultOptions && typeof model.defaultOptions === "object" ? model.defaultOptions : {},
        routes: Array.isArray(model.routes) ? model.routes : [],
    };
}

export type LogicalModelMutation = {
    code: string;
    name: string;
    icon: string;
    description: string;
    capability: CapabilitySpec["capability"];
    enabled: boolean;
    sortOrder: number;
    pricePolicy: PublicLogicalModel["pricePolicy"];
    billingMode: PublicLogicalModel["billingMode"];
    unitPriceMicrocredits: number;
    inputPriceMicrocredits: number;
    outputPriceMicrocredits: number;
    cachedPriceMicrocredits: number;
    legacyModelIds?: string[];
    capabilitySpec: CapabilitySpec;
    defaultOptions: Record<string, unknown>;
    routes: Array<{ channelModelId: string; enabled: boolean; priority: number; weight: number }>;
};

export type RouteSimulationResult = {
    productMatch: { matched: boolean; reasons?: string[] };
    candidates: Array<{ routeId: string; channelModelId: string; channelModelKey: string; channelModelName: string; priority: number; weight: number; enabled: boolean; matched: boolean; blocked: boolean; inPool: boolean; reasons?: string[] }>;
};

export type LogicalModelQuote = {
    logicalModelId: string;
    billingMode: PublicLogicalModel["billingMode"];
    quantity: number;
    amountMicrocredits: number;
    estimated: boolean;
};

export type ModelCatalogSource = "frontend" | "system";

export type PublicChannelCatalog = {
    id: string;
    name: string;
    displayName: string;
    models: PublicChannelModel[];
};

export type PublicChannelModel = {
    id: string;
    modelKey: string;
    displayName: string;
    capability: CapabilitySpec["capability"];
    capabilityConfig?: Record<string, unknown>;
    priceTiers: PublicChannelModelPriceTier[];
    pricingMode: string;
    displayPrice?: number;
    priceLabel: string;
    available: boolean;
};

export type PublicChannelModelPriceTier = PublicLogicalModelPriceTier & {
    id: string;
};

export type ModelCatalogResponse = {
    source: ModelCatalogSource;
    models?: PublicLogicalModel[];
    channels?: PublicChannelCatalog[];
};

export function getModelCatalog() {
    return request<ModelCatalogResponse>(apiClient.get("/model-catalog"));
}

export function getAvailableModelCatalog(intent: ModelRequestIntent) {
    return request<ModelCatalogResponse>(apiClient.post("/model-catalog/available", intent));
}

export function listLogicalModels() {
    return request<{ models: PublicLogicalModel[] }>(apiClient.get("/models"));
}

export function listAvailableLogicalModels(intent: ModelRequestIntent) {
    return request<{ models: PublicLogicalModel[] }>(apiClient.post("/models/available", intent));
}

export function quoteLogicalModel(id: string, intent: ModelRequestIntent, signal?: AbortSignal) {
    return request<{ quote: LogicalModelQuote }>(apiClient.post(`/models/${encodeURIComponent(id)}/quote`, intent, { signal }));
}

export function listAdminLogicalModels() {
    return request<{ models?: AdminLogicalModel[] }>(apiClient.get("/admin/logical-models")).then((result) => ({
        models: Array.isArray(result?.models) ? result.models.filter(Boolean).map(normalizeAdminLogicalModel) : [],
    }));
}

export function createAdminLogicalModel(input: LogicalModelMutation) {
    return request<{ model: AdminLogicalModel }>(apiClient.post("/admin/logical-models", input));
}

export function updateAdminLogicalModel(id: string, input: LogicalModelMutation) {
    return request<{ model: AdminLogicalModel }>(apiClient.patch(`/admin/logical-models/${encodeURIComponent(id)}`, input));
}

export function deleteAdminLogicalModel(id: string) {
    return request<void>(apiClient.delete(`/admin/logical-models/${encodeURIComponent(id)}`));
}

export function simulateAdminLogicalModel(id: string, intent: ModelRequestIntent) {
    return request<RouteSimulationResult>(apiClient.post(`/admin/logical-models/${encodeURIComponent(id)}/simulate`, intent));
}
