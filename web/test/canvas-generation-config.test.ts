import { describe, expect, test } from "bun:test";

import { buildGenerationConfig, generationModelSelectionPatch } from "../src/lib/canvas/canvas-project-generation";
import { defaultModelCapabilityConfig } from "../src/lib/model-capabilities";
import { defaultConfig, type AiConfig, type ModelChannel } from "../src/stores/use-config-store";
import { CanvasNodeType, type CanvasNodeData } from "../src/types/canvas";

function node(type: CanvasNodeType, metadata: CanvasNodeData["metadata"] = {}): CanvasNodeData {
    return { id: "node", type, title: "node", position: { x: 0, y: 0 }, width: 320, height: 180, metadata };
}

function configFor(model: string, capability: "image" | "video", channel: ModelChannel): AiConfig {
    const option = `${channel.id}::${model}`;
    return {
        ...defaultConfig,
        model: option,
        imageModel: capability === "image" ? option : defaultConfig.imageModel,
        videoModel: capability === "video" ? option : defaultConfig.videoModel,
        models: [option],
        imageModels: capability === "image" ? [option] : [],
        videoModels: capability === "video" ? [option] : [],
        channels: [channel],
        size: "1:1",
        vquality: "720",
    };
}

function imageChannel() {
    const model = "gemini-3.1-flash-image-preview";
    const capabilityConfig = defaultModelCapabilityConfig("openai-image", model);
    capabilityConfig.image = {
        ...capabilityConfig.image!,
        size: { parameter: "size", values: ["1:1", "16:9"], default: "16:9", allowCustom: false },
    };
    const channel: ModelChannel = {
        id: "image",
        name: "测试图像渠道",
        baseUrl: "https://api.example.com",
        apiKey: "test-key",
        apiFormat: "openai",
        interfaceType: "openai-image",
        models: [model],
        scope: "user",
        modelCosts: [{ model, capability: "image", protocol: "openai-image", billingMode: "fixed_request", unitPriceMicrocredits: 1, capabilityConfig }],
    };
    return { model, channel };
}

function videoChannel() {
    const model = "seedance-2.0-480p";
    const capabilityConfig = defaultModelCapabilityConfig("newapi-channel-2", model);
    capabilityConfig.video = {
        ...capabilityConfig.video!,
        ratios: ["16:9", "9:16"],
        defaultRatio: "16:9",
        resolutions: ["480p"],
        defaultResolution: "480p",
    };
    const channel: ModelChannel = {
        id: "video",
        name: "测试视频渠道",
        baseUrl: "https://api.example.com",
        apiKey: "test-key",
        apiFormat: "openai",
        interfaceType: "newapi-channel-2",
        models: [model],
        scope: "user",
        modelCosts: [{ model, capability: "video", protocol: "newapi-channel-2", billingMode: "per_second", unitPriceMicrocredits: 1, capabilityConfig }],
    };
    return { model, channel };
}

describe("canvas generation model settings", () => {
    test("an unconfigured image node adopts the selected model default ratio", () => {
        const { model, channel } = imageChannel();
        const config = configFor(model, "image", channel);

        expect(generationModelSelectionPatch(config, node(CanvasNodeType.Image), "image", `image::${model}`)).toMatchObject({
            model: `image::${model}`,
            size: "16:9",
        });
    });

    test("a 480p video model overrides stale 720p settings in both UI state and request config", () => {
        const { model, channel } = videoChannel();
        const config = configFor(model, "video", channel);
        const videoNode = node(CanvasNodeType.Video, { model: `video::${model}`, size: "16:9", seconds: "6", vquality: "720" });

        expect(generationModelSelectionPatch(config, videoNode, "video", `video::${model}`)).toMatchObject({ vquality: "480", size: "16:9", seconds: "6" });
        expect(buildGenerationConfig(config, videoNode, "video")).toMatchObject({ model: `video::${model}`, vquality: "480", size: "16:9", videoSeconds: "6" });
    });
});
