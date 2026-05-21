package proxy

import (
	"context"
	"testing"
)

type recordingBackend struct {
	service  Service
	instance Instance
}

func (backend *recordingBackend) Apply(ctx context.Context, service Service, instance Instance) error {
	backend.service = service
	backend.instance = instance
	return nil
}

func TestRegisterAndSwitchInstance(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	if _, err := UpsertService(context.Background(), store, "slush", "dev.slush.app"); err != nil {
		t.Fatalf("upsert service: %v", err)
	}
	instance := Instance{ID: "feature", Host: "127.0.0.1", Port: 5001}
	if err := RegisterInstance(context.Background(), store, "slush", instance); err != nil {
		t.Fatalf("register: %v", err)
	}

	backend := &recordingBackend{}
	service, switched, err := SwitchInstance(context.Background(), store, backend, "slush", "feature")
	if err != nil {
		t.Fatalf("switch: %v", err)
	}
	if service.ActiveID != "feature" {
		t.Fatalf("active id = %q, want feature", service.ActiveID)
	}
	if switched.Port != 5001 {
		t.Fatalf("switched port = %d, want 5001", switched.Port)
	}
	if backend.service.Alias != "dev.slush.app" {
		t.Fatalf("backend service alias = %q", backend.service.Alias)
	}
	if backend.instance.ID != "feature" {
		t.Fatalf("backend instance id = %q", backend.instance.ID)
	}
}
