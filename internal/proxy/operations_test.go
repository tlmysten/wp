package proxy

import (
	"context"
	"os"
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

func TestRenameService(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	if _, err := UpsertService(context.Background(), store, "old", "", 3003); err != nil {
		t.Fatalf("upsert service: %v", err)
	}

	renamed, err := RenameService(context.Background(), store, "old", "new")
	if err != nil {
		t.Fatalf("rename: %v", err)
	}
	if renamed.Name != "new" {
		t.Fatalf("renamed name = %q, want new", renamed.Name)
	}
	state, err := store.Load()
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if _, ok := state.Services["old"]; ok {
		t.Fatalf("old service still exists")
	}
	if _, ok := state.Services["new"]; !ok {
		t.Fatalf("new service missing")
	}
}

func TestPruneInstancesRemovesDeadProcess(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	if _, err := UpsertService(context.Background(), store, "slush-backend", "", 3003); err != nil {
		t.Fatalf("upsert service: %v", err)
	}
	if err := RegisterInstance(context.Background(), store, "slush-backend", Instance{
		ID:   "dead",
		Host: "localhost",
		Port: 5001,
		URL:  "http://localhost:5001",
		PID:  0,
	}); err != nil {
		t.Fatalf("register dead instance: %v", err)
	}
	if err := RegisterInstance(context.Background(), store, "slush-backend", Instance{
		ID:   "alive",
		Host: "localhost",
		Port: 5002,
		URL:  "http://localhost:5002",
		PID:  os.Getpid(),
	}); err != nil {
		t.Fatalf("register alive instance: %v", err)
	}
	if _, _, err := SwitchInstance(context.Background(), store, nil, "slush-backend", "dead"); err != nil {
		t.Fatalf("switch: %v", err)
	}

	pruned, err := PruneInstances(context.Background(), store, "")
	if err != nil {
		t.Fatalf("prune: %v", err)
	}
	if len(pruned) != 1 {
		t.Fatalf("pruned %d instances, want 1", len(pruned))
	}
	if pruned[0].Instance.ID != "dead" {
		t.Fatalf("pruned %q, want dead", pruned[0].Instance.ID)
	}
	state, err := store.Load()
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	service := state.Services["slush-backend"]
	if _, ok := service.Instances["dead"]; ok {
		t.Fatalf("dead instance was not pruned")
	}
	if _, ok := service.Instances["alive"]; !ok {
		t.Fatalf("alive instance was pruned")
	}
	if service.ActiveID != "" {
		t.Fatalf("active id = %q, want empty", service.ActiveID)
	}
}
