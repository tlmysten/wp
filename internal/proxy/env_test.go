package proxy

import (
	"context"
	"testing"
)

func TestResolveEnvAssignmentsUsesRegisteredServices(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	if _, err := UpsertService(context.Background(), store, "slush-backend", "", 3003); err != nil {
		t.Fatalf("upsert backend service: %v", err)
	}
	if err := RegisterInstance(context.Background(), store, "slush-backend", Instance{
		ID:   "feature",
		Host: "localhost",
		Port: 43103,
		URL:  "http://localhost:43103",
		ExtraPorts: map[string]ExtraPort{
			"prometheus": {
				Name: "prometheus",
				Host: "localhost",
				Port: 43104,
				URL:  "http://localhost:43104",
			},
		},
	}); err != nil {
		t.Fatalf("register backend: %v", err)
	}

	resolved, err := resolveEnvAssignments(store, "slush-web", "feature", Instance{
		ID:   "feature",
		Host: "localhost",
		Port: 5173,
		URL:  "http://localhost:5173",
	}, []string{
		"EXPO_PUBLIC_APPS_BACKEND_URL={{slush-backend.url}}",
		"PROMETHEUS_URL={{slush-backend.prometheus.url}}",
		"FRONTEND_PORT={{slush-web.port}}",
	})
	if err != nil {
		t.Fatalf("resolve env: %v", err)
	}

	want := []string{
		"EXPO_PUBLIC_APPS_BACKEND_URL=http://localhost:43103",
		"PROMETHEUS_URL=http://localhost:43104",
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
