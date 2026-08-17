package service

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestEnsureChatCompletionStreamUsageRequestAddsUsageToStreamingRequest(t *testing.T) {
	body, err := EnsureChatCompletionStreamUsageRequest([]byte(`{"model":"test","stream":true,"stream_options":{"include_usage":false,"foo":"bar"}}`))
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatal(err)
	}
	options, _ := payload["stream_options"].(map[string]any)
	if options["include_usage"] != true || options["foo"] != "bar" {
		t.Fatalf("stream_options = %#v", options)
	}
}

func TestEnsureChatCompletionStreamUsageRequestLeavesNonStreamingRequestUntouched(t *testing.T) {
	body := []byte(`{"model":"test","stream":false}`)
	next, err := EnsureChatCompletionStreamUsageRequest(body)
	if err != nil {
		t.Fatal(err)
	}
	if string(next) != string(body) {
		t.Fatalf("non-stream request changed: %s", next)
	}
}

func TestEnsureChatCompletionStreamUsageRequestRejectsInvalidStreamOptions(t *testing.T) {
	_, err := EnsureChatCompletionStreamUsageRequest([]byte(`{"stream":true,"stream_options":[]}`))
	if err == nil || !strings.Contains(err.Error(), "stream_options") {
		t.Fatalf("EnsureChatCompletionStreamUsageRequest() error = %v", err)
	}
}
