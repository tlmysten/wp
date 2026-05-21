package proxy

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocaliasBackendApply(t *testing.T) {
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "calls.log")
	binPath := filepath.Join(tempDir, "localias")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"" + logPath + "\"\n"
	if err := os.WriteFile(binPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake localias: %v", err)
	}

	backend := LocaliasBackend{
		Binary:     binPath,
		ConfigFile: filepath.Join(tempDir, "localias.yaml"),
		Reload:     true,
	}
	service := Service{Name: "slush", Alias: "dev.slush.app"}
	instance := Instance{ID: "feature", Host: "localhost", Port: 5173}

	if err := backend.Apply(context.Background(), service, instance); err != nil {
		t.Fatalf("apply: %v", err)
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read fake localias log: %v", err)
	}
	got := strings.Split(strings.TrimSpace(string(data)), "\n")
	want := []string{
		"--configfile " + backend.ConfigFile + " set dev.slush.app 5173",
		"--configfile " + backend.ConfigFile + " reload",
	}
	if len(got) != len(want) {
		t.Fatalf("got %d calls, want %d: %q", len(got), len(want), string(data))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("call %d = %q, want %q", i, got[i], want[i])
		}
	}
}
