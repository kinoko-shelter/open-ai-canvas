import { describe, expect, test } from "bun:test";

import { defaultImageCapabilityConfig, normalizeModelCapabilityConfig } from "../src/lib/model-capabilities";

describe("defaultImageCapabilityConfig", () => {
    test("gpt-image-2 exposes its supported ratios and resolution tiers", () => {
        const image = defaultImageCapabilityConfig("openai-image", "gpt-image-2");

        expect(image.size.values).toEqual(["auto", "1:1", "3:2", "2:3", "4:3", "3:4", "5:4", "4:5", "16:9", "9:16", "21:9"]);
        expect(image.quality).toEqual({ supported: true, values: ["1k", "2k", "4k"], default: "1k" });
    });

    test("gpt-image-2 replaces legacy three-size configurations", () => {
        const legacy = defaultImageCapabilityConfig("openai-image", "legacy-image");
        legacy.size = { parameter: "size", values: ["1280x720", "720x1280", "1024x1024"], default: "1280x720", allowCustom: true };

        const normalized = normalizeModelCapabilityConfig({ version: 1, image: legacy }, "openai-image", "gpt-image-2-1k");

        expect(normalized.image?.size.values).toEqual(["auto", "1:1", "3:2", "2:3", "4:3", "3:4", "5:4", "4:5", "16:9", "9:16", "21:9"]);
        expect(normalized.image?.size.default).toBe("auto");
    });

    test("Gemini image models receive only their native aspect ratios", () => {
        const standard = defaultImageCapabilityConfig(undefined, "gemini-2.5-flash-image", "gemini");
        const nano2 = defaultImageCapabilityConfig(undefined, "gemini-3.1-flash-image-preview", "gemini");

        expect(standard.size.values).toEqual(["1:1", "16:9", "9:16", "4:3", "3:4", "21:9", "3:2", "2:3", "5:4", "4:5"]);
        expect(nano2.size.values).toEqual([...standard.size.values, "1:4", "1:8", "4:1", "8:1"]);
        expect(nano2.size.parameter).toBe("aspect_ratio");
    });
});
