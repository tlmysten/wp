package proxy

import (
	"context"
	"testing"
)

type recordingBackend struct {
	service Service
	role    Role
}

func (backend *recordingBackend) Apply(ctx context.Context, service Service, role Role) error {
	backend.service = service
	backend.role = role
	return nil
}

func TestRegisterRoleAndSwitchInstance(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	if _, err := UpsertService(context.Background(), store, "slush", "dev.slush.app", "frontend"); err != nil {
		t.Fatalf("upsert service: %v", err)
	}
	backendRole := Role{Name: "backend", Host: "127.0.0.1", Port: 5001}
	if err := RegisterRole(context.Background(), store, "slush", "feature", backendRole); err != nil {
		t.Fatalf("register backend: %v", err)
	}
	frontendRole := Role{Name: "frontend", Host: "127.0.0.1", Port: 5002}
	if err := RegisterRole(context.Background(), store, "slush", "feature", frontendRole); err != nil {
		t.Fatalf("register frontend: %v", err)
	}

	backend := &recordingBackend{}
	service, switched, role, err := SwitchInstance(context.Background(), store, backend, "slush", "feature")
	if err != nil {
		t.Fatalf("switch: %v", err)
	}
	if service.ActiveID != "feature" {
		t.Fatalf("active id = %q, want feature", service.ActiveID)
	}
	if switched.ID != "feature" {
		t.Fatalf("switched id = %q, want feature", switched.ID)
	}
	if role.Port != 5002 {
		t.Fatalf("switched role port = %d, want 5002", role.Port)
	}
	if backend.service.Alias != "dev.slush.app" {
		t.Fatalf("backend service alias = %q", backend.service.Alias)
	}
	if backend.role.Name != "frontend" {
		t.Fatalf("backend role = %q, want frontend", backend.role.Name)
	}
}
