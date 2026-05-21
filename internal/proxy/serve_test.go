package proxy

import (
	"context"
	"testing"
)

func TestActiveProxyTargetUsesCurrentActiveInstance(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	if _, err := UpsertService(context.Background(), store, "slush-backend", "", 3003); err != nil {
		t.Fatalf("upsert service: %v", err)
	}
	if err := RegisterInstance(context.Background(), store, "slush-backend", Instance{
		ID:   "feature-a",
		Host: "localhost",
		Port: 5001,
		URL:  "http://localhost:5001",
	}); err != nil {
		t.Fatalf("register feature-a: %v", err)
	}
	if err := RegisterInstance(context.Background(), store, "slush-backend", Instance{
		ID:   "feature-b",
		Host: "localhost",
		Port: 5002,
		URL:  "http://localhost:5002",
	}); err != nil {
		t.Fatalf("register feature-b: %v", err)
	}
	if _, _, err := SwitchInstance(context.Background(), store, nil, "slush-backend", "feature-b"); err != nil {
		t.Fatalf("switch: %v", err)
	}

	target, err := activeProxyTarget(store, "slush-backend")
	if err != nil {
		t.Fatalf("active target: %v", err)
	}
	if target.String() != "http://localhost:5002" {
		t.Fatalf("target = %q, want http://localhost:5002", target.String())
	}
}
