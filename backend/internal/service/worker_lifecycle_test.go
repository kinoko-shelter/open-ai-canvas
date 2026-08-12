package service

import (
	"context"
	"testing"
	"time"
)

func TestStopWorkerWaitsForWorkerCompletion(t *testing.T) {
	svc := &Service{workerStop: make(chan struct{}), workerDone: make(chan struct{})}

	done := make(chan error, 1)
	go func() { done <- svc.StopWorker(context.Background()) }()

	select {
	case err := <-done:
		t.Fatalf("StopWorker returned before the worker completed: %v", err)
	case <-time.After(20 * time.Millisecond):
	}

	close(svc.workerDone)
	if err := <-done; err != nil {
		t.Fatalf("StopWorker returned an unexpected error: %v", err)
	}
	if err := svc.StopWorker(context.Background()); err != nil {
		t.Fatalf("StopWorker should be idempotent: %v", err)
	}
}

func TestStopWorkerHonorsContext(t *testing.T) {
	svc := &Service{workerStop: make(chan struct{}), workerDone: make(chan struct{})}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	if err := svc.StopWorker(ctx); err != context.DeadlineExceeded {
		t.Fatalf("StopWorker error = %v, want %v", err, context.DeadlineExceeded)
	}
}
