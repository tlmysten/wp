package proxy

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
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
	if target.URL.String() != "http://localhost:5002" {
		t.Fatalf("target = %q, want http://localhost:5002", target.URL.String())
	}
	if target.Instance.ID != "feature-b" {
		t.Fatalf("target instance = %q, want feature-b", target.Instance.ID)
	}
}

func TestReverseProxyHandlerLogsMissingActiveInstance(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	if _, err := UpsertService(context.Background(), store, "slush-backend", "", 3003); err != nil {
		t.Fatalf("upsert service: %v", err)
	}

	var logs bytes.Buffer
	handler := NewReverseProxyHandler(store, "slush-backend", &logs)
	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:3003/graphql?query=test", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadGateway)
	}
	logLine := logs.String()
	for _, want := range []string{
		"service=slush-backend",
		"id=-",
		"method=GET",
		"path=/graphql?query=test",
		"status=502",
		"target=-",
		"error=",
	} {
		if !strings.Contains(logLine, want) {
			t.Fatalf("log %q did not include %q", logLine, want)
		}
	}
}
