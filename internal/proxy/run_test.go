package proxy

import "testing"

func TestChildEnvIncludesExtraPorts(t *testing.T) {
	role := Role{
		Name: "backend",
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
		role,
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

func containsEnv(env []string, assignment string) bool {
	for _, value := range env {
		if value == assignment {
			return true
		}
	}
	return false
}
