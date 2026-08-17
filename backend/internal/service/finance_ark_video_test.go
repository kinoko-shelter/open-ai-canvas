package service

import (
	"testing"

	"infinite-canvas/backend/internal/model"
)

func TestEstimateArkVideoTokensScalesWithVideoWork(t *testing.T) {
	base := estimateArkVideoTokens(map[string]any{
		"config": map[string]any{"videoSeconds": 5, "vquality": "480p", "size": "16:9", "model": "seedance-2.0"},
	})
	higherResolution := estimateArkVideoTokens(map[string]any{
		"config": map[string]any{"videoSeconds": 5, "vquality": "1080p", "size": "16:9", "model": "seedance-2.0"},
	})
	withReference := estimateArkVideoTokens(map[string]any{
		"config":          map[string]any{"videoSeconds": 5, "vquality": "480p", "size": "16:9", "model": "seedance-2.0"},
		"referenceVideos": []any{map[string]any{"durationMs": 5_000}},
	})

	if base.OutputTokens <= 0 {
		t.Fatalf("base output tokens = %d, want positive", base.OutputTokens)
	}
	if higherResolution.OutputTokens <= base.OutputTokens {
		t.Fatalf("higher resolution tokens = %d, base = %d", higherResolution.OutputTokens, base.OutputTokens)
	}
	if withReference.OutputTokens <= base.OutputTokens {
		t.Fatalf("reference tokens = %d, base = %d", withReference.OutputTokens, base.OutputTokens)
	}
}

func TestEstimateArkVideoTokensRequiresKnownVideoShape(t *testing.T) {
	got := estimateArkVideoTokens(map[string]any{"config": map[string]any{"videoSeconds": 0, "vquality": "480p", "size": "16:9"}})
	if got.OutputTokens != 0 {
		t.Fatalf("output tokens = %d, want 0", got.OutputTokens)
	}
}

func TestSupportsTokenBillingRestrictsVideoToArk(t *testing.T) {
	if !supportsTokenBilling("text", model.ChannelInterfaceChatCompletion) {
		t.Fatal("text Token billing should be supported")
	}
	if !supportsTokenBilling("video", model.ChannelInterfaceVolcengineArkVideo) {
		t.Fatal("Ark video Token billing should be supported")
	}
	if supportsTokenBilling("video", model.ChannelInterfaceNewAPIVideo) {
		t.Fatal("non-Ark video Token billing should be rejected")
	}
}
