package proxy

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
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
		"ERR  502",
		"GET",
		"/graphql?query=test",
		"slush-backend/- -> -",
		"error=",
	} {
		if !strings.Contains(logLine, want) {
			t.Fatalf("log %q did not include %q", logLine, want)
		}
	}
}

func TestProxyLoggerFormatsRequestLog(t *testing.T) {
	var logs bytes.Buffer
	logger := &proxyLogger{out: &logs}

	logger.Log(proxyRequestLog{
		Time:      time.Date(2026, 5, 21, 16, 29, 13, 0, time.Local),
		Service:   "slush-backend",
		Instance:  "feature",
		Method:    http.MethodPost,
		Path:      "/graphql",
		Status:    http.StatusOK,
		Bytes:     1536,
		Duration:  3 * time.Millisecond,
		TargetURL: "http://localhost:60678",
	})

	logLine := logs.String()
	for _, want := range []string{
		"16:29:13",
		"OK   200",
		"POST",
		"/graphql",
		"3ms",
		"1.5KB",
		"slush-backend/feature -> http://localhost:60678",
	} {
		if !strings.Contains(logLine, want) {
			t.Fatalf("log %q did not include %q", logLine, want)
		}
	}
}
