package main

import (
	"testing"
	"time"
)

func TestShutdownTimeout(t *testing.T) {
	t.Setenv("CANVAS_SHUTDOWN_TIMEOUT_SECONDS", "12")
	if got := shutdownTimeout(); got != 12*time.Second {
		t.Fatalf("shutdownTimeout() = %v, want 12s", got)
	}

	t.Setenv("CANVAS_SHUTDOWN_TIMEOUT_SECONDS", "invalid")
	if got := shutdownTimeout(); got != 10*time.Minute {
		t.Fatalf("shutdownTimeout() fallback = %v, want 10m", got)
	}

	t.Setenv("CANVAS_SHUTDOWN_TIMEOUT_SECONDS", "3600")
	if got := shutdownTimeout(); got != 10*time.Minute {
		t.Fatalf("shutdownTimeout() maximum = %v, want 10m", got)
	}
}
