package service

import (
	"testing"

	"infinite-canvas/backend/internal/model"
)

func TestEnrichAPICallLogUsesFinalSSEUsage(t *testing.T) {
	log := model.ApiCallLog{Status: model.ApiCallStatusSucceeded, Capability: "text"}
	response := []byte("data: {\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\ndata: {\"usage\":{\"prompt_tokens\":12,\"completion_tokens\":7,\"prompt_tokens_details\":{\"cached_tokens\":3}}}\n\ndata: [DONE]\n\n")

	(&Service{}).EnrichAPICallLog(&log, response)

	if !log.UsageAvailable || log.InputTokens != 12 || log.OutputTokens != 7 || log.CachedTokens != 3 {
		t.Fatalf("EnrichAPICallLog() = %#v", log)
	}
}

func TestEnrichAPICallLogKeepsUsageUnavailableWithoutSSEUsage(t *testing.T) {
	log := model.ApiCallLog{Status: model.ApiCallStatusSucceeded, Capability: "text"}

	(&Service{}).EnrichAPICallLog(&log, []byte("data: {\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\ndata: [DONE]\n\n"))

	if log.UsageAvailable {
		t.Fatalf("UsageAvailable = true, want false")
	}
}

func TestEnrichAPICallLogUsesArkVideoTotalTokensFallback(t *testing.T) {
	log := model.ApiCallLog{Status: model.ApiCallStatusSucceeded, Capability: "video", Path: "/contents/generations/tasks/task-1"}

	(&Service{}).EnrichAPICallLog(&log, []byte(`{"usage":{"completion_tokens":0,"total_tokens":456}}`))

	if !log.UsageAvailable || log.OutputTokens != 456 {
		t.Fatalf("EnrichAPICallLog() = %#v", log)
	}
}
