package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunAppliesLocaliasAndPassesPort(t *testing.T) {
	tempDir := t.TempDir()
	localiasLogPath := filepath.Join(tempDir, "localias.log")
	envPath := filepath.Join(tempDir, "env.json")
	localiasPath := filepath.Join(tempDir, "localias")
	writeExecutable(t, localiasPath, "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \""+localiasLogPath+"\"\n")

	stateDir := filepath.Join(tempDir, "state")
	mustExecute(t,
		"--state-dir", stateDir,
		"--localias-bin", localiasPath,
		"service", "add", "slush",
		"--alias", "http://wp-test.localhost",
	)

	output := mustExecute(t,
		"--state-dir", stateDir,
		"--localias-bin", localiasPath,
		"run", "slush",
		"--id", "feature",
		"--port", "56666",
		"--switch",
		"--",
		os.Args[0],
		"-test.run", "TestProxyRunHelperProcess",
		"--",
		envPath,
	)
	if !strings.Contains(output, "registered and switched slush/feature -> localhost:") {
		t.Fatalf("run output did not include registration line: %q", output)
	}

	envData, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("read helper env: %v", err)
	}
	var helperEnv map[string]string
	if err := json.Unmarshal(envData, &helperEnv); err != nil {
		t.Fatalf("parse helper env: %v", err)
	}
	port := helperEnv["PORT"]
	if port == "" {
		t.Fatalf("helper did not receive PORT: %v", helperEnv)
	}
	if helperEnv["WP_SERVICE"] != "slush" {
		t.Fatalf("WP_SERVICE = %q, want slush", helperEnv["WP_SERVICE"])
	}
	if helperEnv["WP_ID"] != "feature" {
		t.Fatalf("WP_ID = %q, want feature", helperEnv["WP_ID"])
	}

	logData, err := os.ReadFile(localiasLogPath)
	if err != nil {
		t.Fatalf("read localias log: %v", err)
	}
	got := strings.Split(strings.TrimSpace(string(logData)), "\n")
	want := []string{
		"set http://wp-test.localhost " + port,
		"reload",
	}
	if len(got) != len(want) {
		t.Fatalf("got %d localias calls, want %d: %q", len(got), len(want), string(logData))
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("localias call %d = %q, want %q", index, got[index], want[index])
		}
	}
}

func TestProxyRunHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_WP_HELPER_PROCESS") != "1" {
		return
	}
	args := os.Args
	envPath := args[len(args)-1]
	data, err := json.Marshal(map[string]string{
		"PORT":       os.Getenv("PORT"),
		"WP_SERVICE": os.Getenv("WP_SERVICE"),
		"WP_ID":      os.Getenv("WP_ID"),
	})
	if err != nil {
		os.Exit(2)
	}
	if err := os.WriteFile(envPath, data, 0o644); err != nil {
		os.Exit(3)
	}
	os.Exit(0)
}

func mustExecute(t *testing.T, args ...string) string {
	t.Helper()

	cmd := NewRootCommand()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs(args)
	t.Setenv("GO_WANT_WP_HELPER_PROCESS", "1")

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute %q: %v\nstdout:\n%s\nstderr:\n%s", strings.Join(args, " "), err, stdout.String(), stderr.String())
	}
	return stdout.String()
}

func writeExecutable(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("write executable %s: %v", path, err)
	}
}
