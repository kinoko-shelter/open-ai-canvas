package service

import "testing"

func TestModelRequestIntentNormalizesVideoResolution(t *testing.T) {
	input := map[string]any{
		"mode":   "video",
		"config": map[string]any{"vquality": "480", "videoSeconds": "6", "size": "16:9"},
	}
	intent := ModelRequestIntentFromTaskInput(input, "video_generate", "text_to_video")
	if got := intent.Options["vquality"]; got != "480p" {
		t.Fatalf("vquality = %#v, want 480p", got)
	}
}

func TestNormalizeLogicalModelIntentNormalizesVideoResolution(t *testing.T) {
	intent := normalizeLogicalModelIntent(ModelRequestIntent{
		Capability: "video",
		Options:    map[string]any{"vquality": "720", "videoSeconds": 6},
	})
	if got := intent.Options["vquality"]; got != "720p" {
		t.Fatalf("vquality = %#v, want 720p", got)
	}
}
