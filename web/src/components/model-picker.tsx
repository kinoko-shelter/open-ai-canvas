import { useEffect, useId, useMemo, useRef, useState, type KeyboardEvent } from "react";
import { Check, ChevronDown, Coins } from "lucide-react";
import { Popover } from "antd";

import { canvasThemes, type CanvasTheme } from "@/lib/canvas-theme";
import { modelCapabilityConfigFor, videoDurationOptions } from "@/lib/model-capabilities";
import { normalizeVideoResolution } from "@/lib/video-generation-options";
import { compatibleModelInGroup, groupModelsByDisplayName, modelCompatibilityError, modelRequestOptions, resolveCompatibleModel, type ModelRequirements } from "@/lib/model-selection";
import { cn } from "@/lib/utils";
import { modelDisplayName, modelIcon, modelOptionLabel, modelOptionName, PUBLIC_MODEL_CATALOG_ID, resolveModelChannel, selectableModelsByCapability, type AiConfig, type ModelCapability } from "@/stores/use-config-store";
import { useThemeStore } from "@/stores/use-theme-store";
import { useUserStore } from "@/stores/use-user-store";
import { ModelLogo } from "@/components/model-logo";
import { quoteLogicalModel, type LogicalModelQuote, type ModelRequestIntent } from "@/services/api/logical-models";

type ModelPickerProps = {
    config: AiConfig;
    value?: string;
    onChange: (model: string) => void;
    capability?: ModelCapability;
    className?: string;
    fullWidth?: boolean;
    placeholder?: string;
    onMissingConfig?: () => void;
    showSelectedPrice?: boolean;
    variant?: "default" | "creation";
    requirements?: ModelRequirements;
};

export function ModelPicker({ config, value, onChange, capability, className, fullWidth = false, placeholder = "选择模型", onMissingConfig, showSelectedPrice = true, variant = "default", requirements }: ModelPickerProps) {
    const creditsEnabled = useUserStore((state) => state.features.creditsEnabled);
    const pickerId = useId();
    // 双保险：即使 store merge 写出非法 theme，这里也兜底到 dark，避免 "reading 'node'" 崩溃
    const rawTheme = useThemeStore((state) => state.theme);
    const theme = (canvasThemes[rawTheme as keyof typeof canvasThemes] ?? canvasThemes.dark) as CanvasTheme;
    const [open, setOpen] = useState(false);
    const menuRef = useRef<HTMLDivElement>(null);
    const triggerRef = useRef<HTMLButtonElement>(null);
    const options = useMemo(() => {
        const filtered = selectableModelsByCapability(config, capability);
        const current = value?.trim();
        const currentIncluded = current ? filtered.includes(current) : true;
        return Array.from(new Set([...filtered, ...(!currentIncluded && current ? [current] : [])].filter((model): model is string => Boolean(model))));
    }, [capability, config, value]);
    const optionGroups = useMemo(() => {
        const channelGroups = config.channels
            .map((channel) => ({
                key: channel.id,
                label: channel.name || "未命名渠道",
                scope: channel.id === PUBLIC_MODEL_CATALOG_ID ? "" : channel.scope === "system" ? "平台服务" : "我的模型",
                models: groupModelsByDisplayName(
                    config,
                    options.filter((model) => resolveModelChannel(config, model).id === channel.id),
                ),
            }))
            .filter((group) => group.models.length);
        const groupedModels = new Set(channelGroups.flatMap((group) => group.models.flatMap((modelGroup) => modelGroup.models)));
        const ungroupedModels = options.filter((model) => !groupedModels.has(model));
        return ungroupedModels.length ? [...channelGroups, { key: "ungrouped", label: "其他模型", scope: "未指定渠道", models: groupModelsByDisplayName(config, ungroupedModels) }] : channelGroups;
    }, [config, options]);
    const current = value || "";
    // 参数档位会在选中模型后由调用方归一到其能力配置，不能因为旧模型留下的参数而禁止切换。
    const selectionRequirements = useMemo(
        () => requirements ? { ...requirements, videoSeconds: undefined, imageSize: undefined, options: undefined } : undefined,
        [requirements],
    );
    const resolvedCurrent = resolveCompatibleModel(config, current, selectionRequirements) || current;
    const currentPrice = modelMenuPrice(config, resolvedCurrent, capability, requirements);
    const quoteRequest = useMemo(() => modelQuoteRequest(config, resolvedCurrent, capability, requirements), [capability, config, requirements, resolvedCurrent]);
    const menuQuoteRequests = useMemo(() => {
        const requests = new Map<string, { logicalModelID: string; intent: ModelRequestIntent }>();
        optionGroups.forEach((group) => {
            group.models.forEach((modelGroup) => {
                const selected = modelGroup.models.includes(current);
                const model = compatibleModelInGroup(config, modelGroup.models, selectionRequirements, selected ? current : undefined);
                const request = model ? modelQuoteRequest(config, model, capability, requirements) : undefined;
                if (model && request) requests.set(model, request);
            });
        });
        return Array.from(requests.entries());
    }, [capability, config, current, optionGroups, requirements, selectionRequirements]);
    const [routeQuote, setRouteQuote] = useState<LogicalModelQuote | undefined>();
    const [routeQuoteLoading, setRouteQuoteLoading] = useState(false);
    const [menuQuotes, setMenuQuotes] = useState<Record<string, LogicalModelQuote>>({});
    const [menuQuotesLoading, setMenuQuotesLoading] = useState(false);
    const creationVariant = variant === "creation";

    useEffect(() => {
        if (!showSelectedPrice || !creditsEnabled || !quoteRequest) {
            setRouteQuote(undefined);
            setRouteQuoteLoading(false);
            return;
        }
        const controller = new AbortController();
        setRouteQuote(undefined);
        setRouteQuoteLoading(true);
        quoteLogicalModel(quoteRequest.logicalModelID, quoteRequest.intent, controller.signal)
            .then((payload) => {
                if (!controller.signal.aborted) setRouteQuote(payload.quote);
            })
            .catch(() => {
                if (!controller.signal.aborted) setRouteQuote(undefined);
            })
            .finally(() => {
                if (!controller.signal.aborted) setRouteQuoteLoading(false);
            });
        return () => controller.abort();
    }, [creditsEnabled, quoteRequest, showSelectedPrice]);

    useEffect(() => {
        if (!open || !creditsEnabled || !menuQuoteRequests.length) {
            setMenuQuotes({});
            setMenuQuotesLoading(false);
            return;
        }
        const controller = new AbortController();
        setMenuQuotes({});
        setMenuQuotesLoading(true);
        Promise.all(menuQuoteRequests.map(async ([model, request]) => {
            try {
                const payload = await quoteLogicalModel(request.logicalModelID, request.intent, controller.signal);
                return [model, payload.quote] as const;
            } catch {
                return undefined;
            }
        })).then((entries) => {
            if (controller.signal.aborted) return;
            const quotes: Record<string, LogicalModelQuote> = {};
            entries.forEach((entry) => {
                if (entry) quotes[entry[0]] = entry[1];
            });
            setMenuQuotes(quotes);
            setMenuQuotesLoading(false);
        });
        return () => controller.abort();
    }, [creditsEnabled, menuQuoteRequests, open]);

    useEffect(() => {
        const closeOtherPicker = (event: Event) => {
            if ((event as CustomEvent<string>).detail !== pickerId) setOpen(false);
        };
        window.addEventListener("model-picker-open", closeOtherPicker);
        return () => window.removeEventListener("model-picker-open", closeOtherPicker);
    }, [pickerId]);

    useEffect(() => {
        if (!open) return;
        // 画布拖拽从 pointerdown 开始，须在捕获阶段关闭 Portal 菜单，避免菜单与触发器分离。
        const closeOnOutsidePointer = (event: PointerEvent) => {
            const target = event.target;
            if (!(target instanceof Node)) return;
            if (triggerRef.current?.contains(target) || menuRef.current?.contains(target)) return;
            setOpen(false);
        };
        window.addEventListener("pointerdown", closeOnOutsidePointer, true);
        return () => window.removeEventListener("pointerdown", closeOnOutsidePointer, true);
    }, [open]);

    const setPickerOpen = (nextOpen: boolean) => {
        if (nextOpen && !options.length && config.channelMode === "local") onMissingConfig?.();
        if (nextOpen) window.dispatchEvent(new CustomEvent("model-picker-open", { detail: pickerId }));
        setOpen(nextOpen);
    };
    const focusMenuOption = (last = false) => {
        window.requestAnimationFrame(() => {
            const buttons = menuRef.current?.querySelectorAll<HTMLButtonElement>('[role="option"]');
            const target = last ? buttons?.item((buttons?.length || 1) - 1) : buttons?.item(0);
            target?.focus();
        });
    };
    const handleTriggerKeyDown = (event: KeyboardEvent<HTMLButtonElement>) => {
        if (event.key !== "ArrowDown" && event.key !== "ArrowUp") return;
        event.preventDefault();
        setPickerOpen(true);
        focusMenuOption(event.key === "ArrowUp");
    };
    const handleMenuKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
        if (event.key === "Escape") {
            event.preventDefault();
            setOpen(false);
            triggerRef.current?.focus();
            return;
        }
        if (!["ArrowDown", "ArrowUp", "Home", "End"].includes(event.key)) return;
        const buttons = Array.from(event.currentTarget.querySelectorAll<HTMLButtonElement>('[role="option"]'));
        if (!buttons.length) return;
        event.preventDefault();
        const activeIndex = buttons.indexOf(document.activeElement as HTMLButtonElement);
        const nextIndex = event.key === "Home" ? 0 : event.key === "End" ? buttons.length - 1 : event.key === "ArrowUp" ? Math.max(0, activeIndex - 1) : Math.min(buttons.length - 1, activeIndex + 1);
        buttons[nextIndex]?.focus();
    };
    const content = (
        <div
            ref={menuRef}
            data-canvas-no-zoom
            className={cn("canvas-model-picker-menu max-w-[calc(100vw-24px)]", creationVariant ? "creation-model-picker-menu w-[360px]" : "w-[var(--panel-width-compact)]")}
            style={{ background: theme.node.panel, color: theme.node.text }}
            role="listbox"
            aria-label={placeholder}
            onKeyDown={handleMenuKeyDown}
            onMouseDown={(event) => event.stopPropagation()}
            onPointerDown={(event) => event.stopPropagation()}
        >
            {creationVariant ? (
                <div className="creation-model-picker-heading">
                    <span>选择模型</span>
                    {current ? <strong>{modelDisplayName(config, current)}</strong> : null}
                </div>
            ) : null}
            {optionGroups.length ? (
                optionGroups.map((group) => (
                    <section key={group.key} className="canvas-model-picker-group min-w-0 overflow-hidden">
                        <div className="canvas-model-picker-group-label" style={{ color: theme.node.muted }}>
                            <span className="truncate">{group.label}</span>
                            {group.scope ? <span className="shrink-0" style={{ color: theme.node.muted }}>{group.scope}</span> : null}
                        </div>
                        <div className="grid min-w-0 gap-1">
                            {group.models.map((modelGroup) => {
                                const selected = modelGroup.models.includes(current);
                                const model = compatibleModelInGroup(config, modelGroup.models, selectionRequirements, selected ? current : undefined);
                                const displayModel = model || (selected ? current : modelGroup.models[0]);
                                const disabledReason = model ? "" : modelCompatibilityError(config, modelGroup.models[0], selectionRequirements) || "当前输入不符合该模型能力";
                                return (
                                    <button
                                        key={modelGroup.key}
                                        type="button"
                                        role="option"
                                        aria-selected={selected}
                                        aria-disabled={Boolean(disabledReason)}
                                        disabled={Boolean(disabledReason)}
                                        title={disabledReason || modelOptionLabel(config, displayModel)}
                                        className="canvas-model-picker-option disabled:cursor-not-allowed disabled:opacity-45"
                                        style={{ background: selected ? theme.toolbar.activeBg : "transparent", color: theme.node.text }}
                                        onClick={() => {
                                            if (!model) return;
                                            onChange(model);
                                            setOpen(false);
                                            window.requestAnimationFrame(() => triggerRef.current?.focus());
                                        }}
                                    >
                                        <ModelLabel config={config} model={displayModel} capability={capability} theme={theme} creationVariant={creationVariant} showPrice={creditsEnabled} quote={menuQuotes[displayModel]} quoteLoading={menuQuotesLoading && menuQuoteRequests.some(([quotedModel]) => quotedModel === displayModel)} disabledReason={disabledReason} />
                                        {selected ? <Check className="canvas-model-picker-option-check" style={{ color: theme.node.activeStroke }} /> : null}
                                    </button>
                                );
                            })}
                        </div>
                    </section>
                ))
            ) : (
                <div className="canvas-model-picker-empty" style={{ color: theme.node.muted }}>
                    {emptyModelLabel(config, capability)}
                </div>
            )}
        </div>
    );

    return (
        <div className={cn(fullWidth ? "w-full min-w-0" : "w-fit max-w-full")} onMouseDown={(event) => event.stopPropagation()} onPointerDown={(event) => event.stopPropagation()}>
            <Popover
                open={open}
                onOpenChange={setPickerOpen}
                trigger="click"
                placement="bottomLeft"
                arrow={false}
                content={content}
                classNames={{
                    root: cn("canvas-model-picker-popover", creationVariant && "creation-model-picker-popover"),
                    container: cn("canvas-composer-popover-surface", creationVariant && "creation-model-picker-surface"),
                    content: "canvas-composer-popover-content",
                }}
            >
                <button
                    ref={triggerRef}
                    type="button"
                    className={cn("canvas-composer-model-picker", fullWidth ? "w-full" : "min-w-36 max-w-full", className)}
                    aria-haspopup="listbox"
                    aria-expanded={open}
                    aria-label={placeholder}
                    title={current ? modelOptionLabel(config, current) : placeholder}
                    onKeyDown={handleTriggerKeyDown}
                >
                    <span className="canvas-model-picker-label flex min-w-0 items-center gap-1.5">
                        <span className="canvas-model-picker-trigger-icon" style={{ background: theme.toolbar.itemHover }}>
                            <ModelIcon config={config} model={current} />
                        </span>
                        <span className="min-w-0 flex-1 truncate">{current ? (creationVariant ? modelDisplayName(config, current) : modelOptionLabel(config, current)) : placeholder}</span>
                        {showSelectedPrice && creditsEnabled ? <ModelPrice price={currentPrice} quote={routeQuote} loading={routeQuoteLoading} compact /> : null}
                    </span>
                    <ChevronDown className={cn("canvas-model-picker-chevron", open && "is-open")} aria-hidden="true" />
                </button>
            </Popover>
        </div>
    );
}

function emptyModelLabel(config: AiConfig, capability?: ModelCapability) {
    const label = capability === "image" ? "生图" : capability === "video" ? "视频" : capability === "text" ? "文本" : capability === "audio" ? "音频" : "";
    if (capability && config.models.length) return `暂无支持当前输入的${label}模型`;
    return config.models.length ? `暂无匹配的${label}模型` : "当前没有可用模型，请联系管理员或检查模型配置";
}

function ModelLabel({
    config,
    model,
    capability,
    theme,
    creationVariant,
    showPrice,
    quote,
    quoteLoading,
    disabledReason,
}: {
    config: AiConfig;
    model: string;
    capability?: ModelCapability;
    theme: (typeof canvasThemes)[keyof typeof canvasThemes];
    creationVariant: boolean;
    showPrice: boolean;
    quote?: LogicalModelQuote;
    quoteLoading: boolean;
    disabledReason?: string;
}) {
    const meta = modelMenuMeta(model, capability);
    const channel = resolveModelChannel(config, model);
    const logicalCost = channel.modelCosts?.find((item) => item.model === modelOptionName(model));
    const logicalSpec = logicalCost?.logicalCapabilitySpec;
    const videoProfile = capability === "video" ? modelCapabilityConfigFor(config, model).video : undefined;
    const capabilitySummary = disabledReason || logicalCost?.description?.trim() || (logicalSpec ? logicalCapabilitySummary(logicalSpec) : videoProfile ? `${formatDurationSummary(videoProfile)} · ${videoProfile.resolutions.map((item) => item.toUpperCase()).join("/")}` : meta.description);
    return (
        <span className="flex w-full min-w-0 items-center gap-1.5 overflow-hidden py-0">
            <span className="grid size-6 shrink-0 place-items-center rounded-md" style={{ background: theme.toolbar.itemHover }}>
                <ModelIcon config={config} model={model} />
            </span>
            <span className="min-w-0 flex-1 overflow-hidden">
                <span className="block min-w-0 truncate text-[var(--fs-label)] font-medium leading-none">{modelDisplayName(config, model)}</span>
                <span className="mt-1 block truncate text-[var(--fs-tiny)]" style={{ color: theme.node.muted }} title={capabilitySummary}>
                    {capabilitySummary}
                </span>
            </span>
            {showPrice ? <ModelPrice price={modelMenuPrice(config, model, capability)} quote={quote} loading={quoteLoading} /> : null}
            {!creationVariant && meta.time ? (
                <span className="shrink-0 rounded-full px-1.5 py-0.5 text-[var(--fs-tiny)] tabular-nums" style={{ background: theme.toolbar.itemHover, color: theme.node.muted }}>
                    {meta.time}
                </span>
            ) : null}
        </span>
    );
}

function logicalCapabilitySummary(spec: NonNullable<NonNullable<AiConfig["channels"][number]["modelCosts"]>[number]["logicalCapabilitySpec"]>) {
    const operationLabels: Record<string, string> = {
        text_to_video: "文生视频",
        image_to_video: "图生视频",
        audio_to_video: "音频生视频",
        extend: "视频续写",
        inpaint: "局部修改",
        replace_element: "元素替换",
        camera_motion: "运镜调整",
        style_transfer: "风格迁移",
    };
    const inputLabels: Record<string, { label: string; unit: string }> = {
        image: { label: spec.capability === "text" ? "图片理解" : "参考图片", unit: "张" },
        video: { label: spec.capability === "text" ? "视频理解" : "参考视频", unit: "个" },
        audio: { label: "参考音频", unit: "个" },
        mask: { label: "蒙版", unit: "张" },
    };
    const optionLabels: Record<string, string> = {
        size: "画面比例",
        aspectRatio: "画面比例",
        quality: "生成质量",
        count: "输出数量",
        videoSeconds: "视频时长",
        duration: "视频时长",
        vquality: "输出分辨率",
        resolution: "输出分辨率",
        audioVoice: "音色",
        audioFormat: "音频格式",
        audioSpeed: "语速",
    };
    const values: string[] = [];
    values.push(...(spec.operations || []).map((operation) => operationLabels[operation] || operation));
    for (const [name, constraint] of Object.entries(spec.inputs || {})) {
        if (constraint.max <= 0) continue;
        const definition = inputLabels[name];
        if (!definition) continue;
        values.push(spec.capability === "text" ? `支持${definition.label}` : `${definition.label}最多 ${constraint.max}${definition.unit}`);
    }
    for (const [name, constraint] of Object.entries(spec.options || {})) {
        const label = optionLabels[name];
        if (!label) continue;
        if (constraint.values?.length) values.push(`${label} ${constraint.values.map(publicScalarLabel).join("/")}`);
        else if (constraint.min !== undefined && constraint.max !== undefined) values.push(`${label} ${constraint.min}-${constraint.max}`);
    }
    return values.slice(0, 2).join(" · ") || "智能匹配当前输入";
}

function publicScalarLabel(value: unknown) {
    if (value === true) return "支持";
    if (value === false) return "关闭";
    return String(value);
}

function formatDurationSummary(profile: NonNullable<ReturnType<typeof modelCapabilityConfigFor>["video"]>) {
    const values = videoDurationOptions(profile);
    if (profile.duration.selection === "enum") return values.map((item) => `${item}s`).join("/");
    return `${profile.duration.min || values[0]}-${profile.duration.max || values[values.length - 1]}s`;
}

type ModelMenuPrice =
    | { kind: "tiers"; label: string; compactLabel: string; title: string }
    | { kind: "fixed"; value: number; unit: "次" | "秒" | "百万 Token" };

type TokenPrice = {
    inputTokenPriceMicrocredits?: number;
    outputTokenPriceMicrocredits?: number;
    cachedTokenPriceMicrocredits?: number;
};

function modelMenuPrice(config: AiConfig, model: string, capability?: ModelCapability, requirements?: ModelRequirements): ModelMenuPrice | null | undefined {
    if (!model) return undefined;
    const channel = resolveModelChannel(config, model);
    const cost = channel.modelCosts?.find((item) => item.model === modelOptionName(model));
    if (!cost) return channel.scope === "system" ? null : undefined;
    if (cost.pricePolicy === "channel") {
        const tiers = cost.logicalPriceTiers || [];
        if (!tiers.length) return null;
        return channelTierPriceSummary(priceTiersForCurrentSelection(tiers, capability, config, requirements), tiers);
    }
    if (cost.billingMode === "token") return tokenPriceSummary([cost]);
    return { kind: "fixed", value: cost.unitPriceMicrocredits / 1_000_000, unit: cost.billingMode === "per_second" ? "秒" : "次" };
}

function priceTiersForCurrentSelection(
    tiers: NonNullable<NonNullable<AiConfig["channels"][number]["modelCosts"]>[number]["logicalPriceTiers"]>,
    capability: ModelCapability | undefined,
    config: AiConfig,
    requirements?: ModelRequirements,
) {
    const requested: Record<string, string> = {};
    if (capability === "video") {
        const imageCount = (requirements?.input?.imageCount || 0) + (requirements?.input?.characterCount || 0);
        if (imageCount > 0) requested.imageCount = String(imageCount);
        const resolution = normalizeTierResolution(config.vquality);
        if (resolution !== "*") requested.vquality = resolution;
        const seconds = Math.max(0, Math.floor(Number(config.videoSeconds) || 0));
        if (seconds > 0) requested.videoSeconds = String(seconds);
    }
    if (capability === "image") {
        if (config.quality && config.quality !== "auto") requested.quality = config.quality.toLowerCase();
        if (config.size && config.size !== "auto") requested.size = config.size.toLowerCase();
    }
    let bestScore = -1;
    let matched: typeof tiers = [];
    for (const tier of tiers) {
		const selector = tier.selector || {};
		const conditions = Object.entries(selector).filter(([, value]) => value && value !== "*");
		// 未选择具体规格（auto）时汇总可用档位，不能误报为未配置。
		const requestedConditions = conditions.filter(([key]) => requested[key] !== undefined);
		if (requestedConditions.some(([key, value]) => requested[key] !== value)) continue;
		const score = requestedConditions.length;
        if (score > bestScore) {
            bestScore = score;
            matched = [tier];
        } else if (score === bestScore) {
            matched.push(tier);
        }
    }
    return matched;
}

function normalizeTierResolution(value: string) {
    const raw = String(value || "").trim();
    if (!raw || raw === "*") return "*";
    return `${normalizeVideoResolution(raw)}p`;
}

function channelTierPriceSummary(
    visibleTiers: NonNullable<NonNullable<AiConfig["channels"][number]["modelCosts"]>[number]["logicalPriceTiers"]>,
    allTiers: NonNullable<NonNullable<AiConfig["channels"][number]["modelCosts"]>[number]["logicalPriceTiers"]>,
): Extract<ModelMenuPrice, { kind: "tiers" }> {
    const fixedRequestValues = visibleTiers
        .filter((tier) => tier.billingMode === "fixed_request")
        .map((tier) => tier.unitPriceMicrocredits / 1_000_000)
        .filter((value) => value > 0);
    const perSecondValues = visibleTiers
        .filter((tier) => tier.billingMode === "per_second")
        .map((tier) => tier.unitPriceMicrocredits / 1_000_000)
        .filter((value) => value > 0);
    const tokenTiers = visibleTiers.filter((tier) => tier.billingMode === "token");
    const tokenPrice = tokenTiers.length ? tokenPriceSummary(tokenTiers) : undefined;
    const label = fixedRequestValues.length
        ? formatPriceRange(fixedRequestValues, "积分")
        : perSecondValues.length
            ? formatPriceRange(perSecondValues, "积分/秒")
            : tokenPrice
                ? tokenPrice.label
                : "未配置";
    return {
        kind: "tiers",
        label,
        compactLabel: tokenPrice?.compactLabel || label,
        title: `系统规格价格：${allTiers.map((tier) => `${tierSpecificationLabel(tier)} ${tierPriceLabel(tier)}`).join("；")}`,
    };
}

function formatPriceRange(values: number[], suffix: string) {
    return `${formatPriceValues(values)} ${suffix}`;
}

function formatPriceValues(values: number[]) {
    const unique = Array.from(new Set(values)).sort((left, right) => left - right);
    const format = (value: number) => value.toLocaleString("zh-CN", { maximumFractionDigits: 3 });
    return unique.length === 1 ? format(unique[0]) : `${format(unique[0])}-${format(unique[unique.length - 1])}`;
}

function tokenPriceSummary(prices: TokenPrice[]): Extract<ModelMenuPrice, { kind: "tiers" }> {
    const rates = {
        input: tokenPriceValues(prices, "inputTokenPriceMicrocredits"),
        output: tokenPriceValues(prices, "outputTokenPriceMicrocredits"),
        cached: tokenPriceValues(prices, "cachedTokenPriceMicrocredits"),
    };
    const hasConfiguredRate = Object.values(rates).some((values) => values.some((value) => value > 0));
    const visibleRates = hasConfiguredRate
        ? [rates.input, rates.output].filter((values) => values.length)
        : [];
    const label = visibleRates.length
        ? `${visibleRates.map(formatPriceValues).join("/")} 积分/M Token`
        : "Token 计费";
    const titleParts = hasConfiguredRate
        ? [
            rates.input.length ? `输入 ${formatPriceRange(rates.input, "积分/百万 Token")}` : "",
            rates.output.length ? `输出 ${formatPriceRange(rates.output, "积分/百万 Token")}` : "",
            rates.cached.length ? `缓存 ${formatPriceRange(rates.cached, "积分/百万 Token")}` : "",
        ].filter(Boolean)
        : [];
    return {
        kind: "tiers",
        label,
        compactLabel: "Token 计费",
        title: titleParts.length ? `Token 单价：${titleParts.join("；")}` : "按 Token 计费，实际消耗以任务结算为准",
    };
}

function tokenPriceValues(prices: TokenPrice[], key: keyof TokenPrice) {
    return prices
        .map((price) => price[key])
        .filter((value): value is number => typeof value === "number" && Number.isFinite(value) && value >= 0)
        .map((value) => value / 1_000_000);
}

function tierResolutionLabel(value: string) {
    const normalized = normalizeTierResolution(value);
    return normalized === "*" ? "全部分辨率" : normalized.toUpperCase();
}

function tierDurationLabel(seconds: number) {
    return seconds > 0 ? `${seconds} 秒` : "全部时长";
}

function tierSpecificationLabel(tier: NonNullable<NonNullable<AiConfig["channels"][number]["modelCosts"]>[number]["logicalPriceTiers"]>[number]) {
    const selector = tier.selector || {};
	const operationLabels: Record<string, string> = { text_to_image: "文生图", image_to_image: "图生图", text_to_video: "文生视频", image_to_video: "图生视频", video_to_video: "视频生视频" };
	const operation = selector.operation && selector.operation !== "*" ? (operationLabels[selector.operation] || selector.operation) : "";
	const details = [
		operation,
		selector.quality && selector.quality !== "*" ? selector.quality.toUpperCase() : "",
		selector.size && selector.size !== "*" ? selector.size : "",
		tier.resolution !== "*" ? tierResolutionLabel(tier.resolution) : "",
		tier.videoSeconds ? tierDurationLabel(tier.videoSeconds) : "",
		selector.imageCount && selector.imageCount !== "*" ? `${selector.imageCount} 张参考图` : "",
	].filter(Boolean);
	return details.length ? details.join(" / ") : "默认规格";
}

function tierPriceLabel(tier: NonNullable<NonNullable<AiConfig["channels"][number]["modelCosts"]>[number]["logicalPriceTiers"]>[number]) {
    if (tier.billingMode === "token") return tokenPriceSummary([tier]).label;
    return `${formatPriceRange([tier.unitPriceMicrocredits / 1_000_000], tier.billingMode === "per_second" ? "积分/秒" : "积分")}`;
}

function ModelPrice({ price, quote, loading = false, compact = false }: { price: ModelMenuPrice | null | undefined; quote?: LogicalModelQuote; loading?: boolean; compact?: boolean }) {
    if (quote) {
        const amount = (quote.amountMicrocredits / 1_000_000).toLocaleString("zh-CN", { maximumFractionDigits: 3 });
        const label = quote.estimated ? `预计 ${amount}` : `${amount}`;
        return (
            <span className="inline-flex shrink-0 items-center gap-0.5 text-[var(--fs-tiny)] font-bold tabular-nums text-amber-600 dark:text-amber-300" title={`${quote.estimated ? "预计" : "本次"}消耗 ${amount} 积分`}>
                <Coins className="size-3" />
                {compact ? label : `${label} 积分`}
            </span>
        );
    }
    if (loading) return <span className="shrink-0 text-[var(--fs-tiny)] font-medium text-foreground/45">计算中</span>;
    if (price === undefined) return null;
    if (price === null) return compact ? null : <span className="shrink-0 text-[var(--fs-tiny)] text-foreground/40">未配置</span>;
    if (price.kind === "tiers") {
        return (
            <span className="inline-flex shrink-0 items-center gap-0.5 text-[var(--fs-tiny)] font-bold tabular-nums text-amber-600 dark:text-amber-300" title={price.title}>
                <Coins className="size-3" />
                {compact ? price.compactLabel : price.label}
            </span>
        );
    }
    return (
        <span className="inline-flex shrink-0 items-center gap-0.5 text-[var(--fs-tiny)] font-bold tabular-nums text-amber-600 dark:text-amber-300" title={`每${price.unit}消耗 ${price.value.toLocaleString("zh-CN", { maximumFractionDigits: 6 })} 积分`}>
            <Coins className="size-3" />
            {price.value.toLocaleString("zh-CN", { maximumFractionDigits: compact ? 3 : 6 })}/{price.unit}
        </span>
    );
}

function modelQuoteRequest(config: AiConfig, value: string, capability?: ModelCapability, requirements?: ModelRequirements): { logicalModelID: string; intent: ModelRequestIntent } | undefined {
    if (!capability || !value) return undefined;
    const channel = resolveModelChannel(config, value);
    if (channel.scope !== "system") return undefined;
    const cost = channel.modelCosts?.find((item) => item.model === modelOptionName(value));
    if (!cost?.logicalModelId) return undefined;
    const input = requirements?.input;
    const intent: ModelRequestIntent = {
        capability,
        operation: requirements?.videoOperation,
        inputs: {
            image: (input?.imageCount || 0) + (input?.characterCount || 0),
            video: input?.videoCount || 0,
            audio: input?.audioCount || 0,
        },
        options: {
            ...modelRequestOptions(config, capability),
            ...(requirements?.options || {}),
            ...(requirements?.videoSeconds ? { videoSeconds: Number(requirements.videoSeconds) } : {}),
            ...(requirements?.imageSize ? { size: requirements.imageSize } : {}),
        },
    };
    return { logicalModelID: cost.logicalModelId, intent };
}

function modelMenuMeta(model: string, capability?: ModelCapability): { description: string; time?: string } {
    const name = modelOptionName(model).toLowerCase();
    if (capability === "image") {
        if (name.includes("nano banana") || name.includes("nanobanana") || name.includes("imagen")) return { description: "Gemini 高质量图片生成，适合角色和商业成片" };
        if (name.includes("nano") || name.includes("pro")) return { description: "高质量图片生成，适合角色和商业成片" };
        if (name.includes("seedream")) return { description: "快速出图，适合批量探索风格" };
        if (name.includes("gpt") || name.includes("image")) return { description: "通用图片模型，提示词理解稳定" };
        return { description: "图片生成模型" };
    }
    if (capability === "video") {
        if (name.includes("veo") || name.includes("omni flash") || name.includes("omni-flash")) return { description: "Gemini 镜头生成与图生视频，适合成片流程", time: "3m" };
        if (name.includes("seedance") || name.includes("sora")) return { description: "镜头生成与图生视频，适合成片流程", time: "3m" };
        return { description: "视频生成模型", time: "3m" };
    }
    if (capability === "audio") return { description: "语音、音效或音乐生成", time: "20s" };
    if (name.includes("claude")) return { description: "长文本、推理与创意写作", time: "10s" };
    if (name.includes("gemini")) return { description: "多模态理解与快速文本生成", time: "10s" };
    if (name.includes("deepseek")) return { description: "推理、代码和结构化文本", time: "10s" };
    return { description: capability === "text" ? "文本生成模型" : "当前模型", time: "10s" };
}

export function ModelIcon({ config, model, icon }: { config?: AiConfig; model?: string; icon?: string }) {
    return <ModelLogo icon={icon || (config && model ? modelIcon(config, model) : "")} size={14} className="opacity-80" />;
}
