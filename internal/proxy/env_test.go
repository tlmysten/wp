package proxy

import (
	"context"
	"testing"
)

func TestResolveEnvAssignmentsUsesRegisteredRoles(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	if _, err := UpsertService(context.Background(), store, "slush", "dev.slush.app", "frontend"); err != nil {
		t.Fatalf("upsert service: %v", err)
	}
	if err := RegisterRole(context.Background(), store, "slush", "feature", Role{
		Name: "backend",
		Host: "127.0.0.1",
		Port: 43103,
		URL:  "http://127.0.0.1:43103",
	}); err != nil {
		t.Fatalf("register backend: %v", err)
	}

	resolved, err := resolveEnvAssignments(store, "slush", "feature", Role{
		Name: "frontend",
		Host: "127.0.0.1",
		Port: 5173,
		URL:  "http://127.0.0.1:5173",
	}, []string{
		"EXPO_PUBLIC_APPS_BACKEND_URL={{backend.url}}",
		"FRONTEND_PORT={{frontend.port}}",
	})
	if err != nil {
		t.Fatalf("resolve env: %v", err)
	}

	want := []string{
		"EXPO_PUBLIC_APPS_BACKEND_URL=http://127.0.0.1:43103",
		"FRONTEND_PORT=5173",
	}
	if len(resolved) != len(want) {
		t.Fatalf("got %d assignments, want %d: %v", len(resolved), len(want), resolved)
	}
	for index := range want {
		if resolved[index] != want[index] {
			t.Fatalf("assignment %d = %q, want %q", index, resolved[index], want[index])
		}
	}
}
