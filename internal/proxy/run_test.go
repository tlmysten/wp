package proxy

import "testing"

func TestChildEnvIncludesExtraPorts(t *testing.T) {
	instance := Instance{
		ID:   "feature",
		Port: 3003,
		ExtraPorts: map[string]ExtraPort{
			"prometheus": {
				Name: "prometheus",
				Port: 9090,
			},
		},
	}
	env := childEnv(
		RunOptions{
			ServiceName: "slush",
			PortEnv:     "PORT",
		},
		"feature",
		instance,
		[]extraPortSpec{{Name: "prometheus", Env: "PROMETHEUS_PORT"}},
		nil,
	)

	if !containsEnv(env, "PORT=3003") {
		t.Fatalf("env did not include PORT=3003: %v", env)
	}
	if !containsEnv(env, "PROMETHEUS_PORT=9090") {
		t.Fatalf("env did not include PROMETHEUS_PORT=9090: %v", env)
	}
}

func TestInstancePortsSummaryIncludesExtraPorts(t *testing.T) {
	summary := instancePortsSummary(Instance{
		ID:   "feature",
		Host: "localhost",
		Port: 3003,
		ExtraPorts: map[string]ExtraPort{
			"prometheus": {
				Name: "prometheus",
				Host: "localhost",
				Port: 9090,
			},
			"debug": {
				Name: "debug",
				Host: "localhost",
				Port: 9229,
			},
		},
	})

	want := "localhost:3003 debug=localhost:9229 prometheus=localhost:9090"
	if summary != want {
		t.Fatalf("summary = %q, want %q", summary, want)
	}
}

func containsEnv(env []string, assignment string) bool {
	for _, value := range env {
		if value == assignment {
			return true
		}
	}
	return false
}
