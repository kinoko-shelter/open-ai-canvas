package service

import "testing"

func TestConfiguredTaskWorkerScope(t *testing.T) {
	t.Setenv(taskWorkerPoolEnv, "")
	scope, err := configuredTaskWorkerScope()
	if err != nil || scope != legacyTaskWorkerScope {
		t.Fatalf("configuredTaskWorkerScope() = %q, %v", scope, err)
	}

	t.Setenv(taskWorkerPoolEnv, "production_1")
	scope, err = configuredTaskWorkerScope()
	if err != nil || scope != "workers:production_1" {
		t.Fatalf("configuredTaskWorkerScope() = %q, %v", scope, err)
	}

	t.Setenv(taskWorkerPoolEnv, "production/invalid")
	if _, err := configuredTaskWorkerScope(); err == nil {
		t.Fatal("configuredTaskWorkerScope() accepted an invalid pool name")
	}
}

func TestTaskWorkerLegacyGuardEnabled(t *testing.T) {
	for value, want := range map[string]bool{"": false, "false": false, "1": true, "true": true, "ON": true} {
		t.Setenv(taskWorkerLegacyGuardEnv, value)
		if actual := taskWorkerLegacyGuardEnabled(); actual != want {
			t.Fatalf("taskWorkerLegacyGuardEnabled(%q) = %v, want %v", value, actual, want)
		}
	}
}

func TestStartWorkerClosesWhenLegacyGuardHasNoNamedPool(t *testing.T) {
	t.Setenv(taskWorkerLegacyGuardEnv, "true")
	t.Setenv(taskWorkerPoolEnv, "")
	svc := &Service{workerStop: make(chan struct{}), workerDone: make(chan struct{})}
	svc.StartWorker()
	select {
	case <-svc.workerDone:
	default:
		t.Fatal("StartWorker() left workerDone open after rejecting an unsafe guard configuration")
	}
}
