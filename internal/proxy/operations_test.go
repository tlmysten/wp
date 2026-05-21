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

func TestRegisterInstanceAndSwitchAliasService(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	if _, err := UpsertService(context.Background(), store, "slush-web", "dev.slush.app", 0); err != nil {
		t.Fatalf("upsert service: %v", err)
	}
	instance := Instance{ID: "feature", Host: "localhost", Port: 5002, URL: "http://localhost:5002"}
	if err := RegisterInstance(context.Background(), store, "slush-web", instance); err != nil {
		t.Fatalf("register instance: %v", err)
	}

	backend := &recordingBackend{}
	service, switched, err := SwitchInstance(context.Background(), store, backend, "slush-web", "feature")
	if err != nil {
		t.Fatalf("switch: %v", err)
	}
	if service.ActiveID != "feature" {
		t.Fatalf("active id = %q, want feature", service.ActiveID)
	}
	if switched.ID != "feature" {
		t.Fatalf("switched id = %q, want feature", switched.ID)
	}
	if backend.service.Alias != "dev.slush.app" {
		t.Fatalf("backend service alias = %q", backend.service.Alias)
	}
	if backend.instance.Port != 5002 {
		t.Fatalf("backend instance port = %d, want 5002", backend.instance.Port)
	}
}

func TestSwitchListenServiceDoesNotRequireBackend(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	if _, err := UpsertService(context.Background(), store, "slush-backend", "", 3003); err != nil {
		t.Fatalf("upsert service: %v", err)
	}
	instance := Instance{ID: "feature", Host: "localhost", Port: 5001, URL: "http://localhost:5001"}
	if err := RegisterInstance(context.Background(), store, "slush-backend", instance); err != nil {
		t.Fatalf("register instance: %v", err)
	}

	service, switched, err := SwitchInstance(context.Background(), store, nil, "slush-backend", "feature")
	if err != nil {
		t.Fatalf("switch: %v", err)
	}
	if service.ActiveID != "feature" {
		t.Fatalf("active id = %q, want feature", service.ActiveID)
	}
	if switched.Port != 5001 {
		t.Fatalf("switched port = %d, want 5001", switched.Port)
	}
}
