package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tlmysten/worktree-tools/internal/proxy"
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
	if !strings.Contains(output, "READY  slush/feature -> localhost:") {
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

func TestListShowsConfiguredServicesWithoutInstances(t *testing.T) {
	tempDir := t.TempDir()
	stateDir := filepath.Join(tempDir, "state")

	mustExecute(t,
		"--state-dir", stateDir,
		"service", "add", "slush-backend",
		"--listen", "3003",
	)
	mustExecute(t,
		"--state-dir", stateDir,
		"service", "add", "slush-web",
		"--alias", "dev.slush.app",
	)

	output := mustExecute(t,
		"--state-dir", stateDir,
		"list",
	)
	for _, want := range []string{
		"SERVICE",
		"ENDPOINT",
		"ACTIVE",
		"slush-backend",
		"127.0.0.1:3003",
		"slush-web",
		"dev.slush.app",
		"none",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("list output %q did not include %q", output, want)
		}
	}
}

func TestServeStatusChecksConfiguredServices(t *testing.T) {
	tempDir := t.TempDir()
	stateDir := filepath.Join(tempDir, "state")

	mustExecute(t,
		"--state-dir", stateDir,
		"service", "add", "slush-backend",
		"--listen", "3003",
	)
	mustExecute(t,
		"--state-dir", stateDir,
		"service", "add", "slush-web",
		"--alias", "dev.slush.app",
	)

	output := mustExecute(t,
		"--state-dir", stateDir,
		"serve", "status",
	)
	for _, want := range []string{
		"SERVICE",
		"ENDPOINT",
		"STATUS",
		"slush-backend",
		"127.0.0.1:3003",
		"stopped",
		"slush-web",
		"dev.slush.app",
		"skip",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("serve status output %q did not include %q", output, want)
		}
	}
}

func TestSwitchAllByID(t *testing.T) {
	tempDir := t.TempDir()
	stateDir := filepath.Join(tempDir, "state")
	store, err := proxy.NewStore(stateDir)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	if _, err := proxy.UpsertService(testContext(), store, "backend", "", 3003); err != nil {
		t.Fatalf("upsert backend: %v", err)
	}
	if _, err := proxy.UpsertService(testContext(), store, "worker", "", 3004); err != nil {
		t.Fatalf("upsert worker: %v", err)
	}
	for _, serviceName := range []string{"backend", "worker"} {
		if err := proxy.RegisterInstance(testContext(), store, serviceName, proxy.Instance{
			ID:   "feature",
			Host: "localhost",
			Port: 5001,
			URL:  "http://localhost:5001",
			PID:  os.Getpid(),
		}); err != nil {
			t.Fatalf("register %s: %v", serviceName, err)
		}
	}

	output := mustExecute(t,
		"--state-dir", stateDir,
		"switch", "--id", "feature",
	)
	for _, want := range []string{
		"OK",
		"backend",
		"worker",
		"id=feature",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("switch output %q did not include %q", output, want)
		}
	}
	state, err := store.Load()
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	for _, serviceName := range []string{"backend", "worker"} {
		if state.Services[serviceName].ActiveID != "feature" {
			t.Fatalf("%s active id = %q, want feature", serviceName, state.Services[serviceName].ActiveID)
		}
	}
}

func TestCurrentPrintsActiveURL(t *testing.T) {
	tempDir := t.TempDir()
	stateDir := filepath.Join(tempDir, "state")
	store, err := proxy.NewStore(stateDir)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	if _, err := proxy.UpsertService(testContext(), store, "backend", "", 3003); err != nil {
		t.Fatalf("upsert backend: %v", err)
	}
	if err := proxy.RegisterInstance(testContext(), store, "backend", proxy.Instance{
		ID:   "feature",
		Host: "localhost",
		Port: 5001,
		URL:  "http://localhost:5001",
		ExtraPorts: map[string]proxy.ExtraPort{
			"metrics": {
				Name: "metrics",
				Host: "localhost",
				Port: 9090,
				URL:  "http://localhost:9090",
			},
		},
		PID: os.Getpid(),
	}); err != nil {
		t.Fatalf("register backend: %v", err)
	}
	if _, _, err := proxy.SwitchInstance(testContext(), store, nil, "backend", "feature"); err != nil {
		t.Fatalf("switch: %v", err)
	}

	output := mustExecute(t,
		"--state-dir", stateDir,
		"current", "backend",
	)
	if strings.TrimSpace(output) != "http://localhost:5001" {
		t.Fatalf("current output = %q", output)
	}

	output = mustExecute(t,
		"--state-dir", stateDir,
		"current", "backend", "--extra", "metrics", "--field", "port",
	)
	if strings.TrimSpace(output) != "9090" {
		t.Fatalf("current extra output = %q", output)
	}
}

func TestPruneRemovesStaleInstances(t *testing.T) {
	tempDir := t.TempDir()
	stateDir := filepath.Join(tempDir, "state")
	store, err := proxy.NewStore(stateDir)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	if _, err := proxy.UpsertService(testContext(), store, "backend", "", 3003); err != nil {
		t.Fatalf("upsert backend: %v", err)
	}
	if err := proxy.RegisterInstance(testContext(), store, "backend", proxy.Instance{
		ID:   "dead",
		Host: "localhost",
		Port: 5001,
		URL:  "http://localhost:5001",
		PID:  0,
	}); err != nil {
		t.Fatalf("register backend: %v", err)
	}

	output := mustExecute(t,
		"--state-dir", stateDir,
		"prune",
	)
	if !strings.Contains(output, "backend/dead") {
		t.Fatalf("prune output %q did not include backend/dead", output)
	}
	state, err := store.Load()
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if len(state.Services["backend"].Instances) != 0 {
		t.Fatalf("instances were not pruned: %v", state.Services["backend"].Instances)
	}
}

func TestServiceRename(t *testing.T) {
	tempDir := t.TempDir()
	stateDir := filepath.Join(tempDir, "state")

	mustExecute(t,
		"--state-dir", stateDir,
		"service", "add", "old",
		"--listen", "3003",
	)
	output := mustExecute(t,
		"--state-dir", stateDir,
		"service", "rename", "old", "new",
	)
	if !strings.Contains(output, "old -> new") {
		t.Fatalf("rename output %q did not include old -> new", output)
	}
	list := mustExecute(t,
		"--state-dir", stateDir,
		"service", "list",
	)
	if !strings.Contains(list, "new") || strings.Contains(list, "old") {
		t.Fatalf("service list after rename = %q", list)
	}
}

func TestDoctorReportsConfiguration(t *testing.T) {
	tempDir := t.TempDir()
	stateDir := filepath.Join(tempDir, "state")

	mustExecute(t,
		"--state-dir", stateDir,
		"service", "add", "backend",
		"--listen", "3003",
	)
	output := mustExecute(t,
		"--state-dir", stateDir,
		"doctor",
	)
	for _, want := range []string{
		"CHECK",
		"state",
		"serve:backend",
		"active:backend",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("doctor output %q did not include %q", output, want)
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

func testContext() context.Context {
	return context.Background()
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
