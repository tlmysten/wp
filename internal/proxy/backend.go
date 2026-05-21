package proxy

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

type Backend interface {
	Apply(ctx context.Context, service Service, role Role) error
}

type LocaliasBackend struct {
	Binary     string
	ConfigFile string
	Reload     bool
}

func (backend LocaliasBackend) Apply(ctx context.Context, service Service, role Role) error {
	if service.Alias == "" {
		return fmt.Errorf("service %q has no alias", service.Name)
	}

	binary := backend.Binary
	if binary == "" {
		binary = "localias"
	}

	setArgs := backend.baseArgs()
	setArgs = append(setArgs, "set", service.Alias, strconv.Itoa(role.Port))
	if output, err := exec.CommandContext(ctx, binary, setArgs...).CombinedOutput(); err != nil {
		return commandError("localias set", err, output)
	}

	if !backend.Reload {
		return nil
	}

	reloadArgs := backend.baseArgs()
	reloadArgs = append(reloadArgs, "reload")
	if output, err := exec.CommandContext(ctx, binary, reloadArgs...).CombinedOutput(); err != nil {
		return commandError("localias reload", err, output)
	}

	return nil
}

func (backend LocaliasBackend) baseArgs() []string {
	if backend.ConfigFile == "" {
		return nil
	}
	return []string{"--configfile", backend.ConfigFile}
}

func commandError(name string, err error, output []byte) error {
	text := strings.TrimSpace(string(output))
	if text == "" {
		return fmt.Errorf("%s failed: %w", name, err)
	}
	return fmt.Errorf("%s failed: %w\n%s", name, err, text)
}
