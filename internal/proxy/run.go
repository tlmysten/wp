package proxy

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"syscall"
	"time"
)

type RunOptions struct {
	ServiceName   string
	ID            string
	Port          int
	PortEnv       string
	ExtraPorts    []string
	HostEnv       string
	Env           []string
	CWD           string
	Command       []string
	SwitchOnStart bool
	Stdout        io.Writer
	Stderr        io.Writer
	Stdin         io.Reader
}

type CommandExitError struct {
	Code int
}

func (err CommandExitError) Error() string {
	return fmt.Sprintf("command exited with status %d", err.Code)
}

const defaultHost = "localhost"

func RunCommand(ctx context.Context, store *Store, backend Backend, opts RunOptions) error {
	if len(opts.Command) == 0 {
		return fmt.Errorf("command is required")
	}
	if opts.ServiceName == "" {
		return fmt.Errorf("service name is required")
	}
	if opts.PortEnv == "" {
		return fmt.Errorf("port environment variable is required")
	}
	service, err := resolveRunService(store, opts.ServiceName)
	if err != nil {
		return err
	}

	cwd, err := resolveCWD(opts.CWD)
	if err != nil {
		return err
	}

	id := opts.ID
	if id == "" {
		id = defaultID(cwd)
	}
	if err := validateRunTarget(store, service.Name, id); err != nil {
		return err
	}

	port := opts.Port
	if port == 0 {
		port, err = FindFreePort()
		if err != nil {
			return err
		}
	}
	extraPortSpecs, err := parseExtraPortSpecs(opts.ExtraPorts)
	if err != nil {
		return err
	}
	extraPorts, err := allocateExtraPorts(extraPortSpecs, map[int]bool{port: true})
	if err != nil {
		return err
	}

	stdout := opts.Stdout
	if stdout == nil {
		stdout = os.Stdout
	}
	stderr := opts.Stderr
	if stderr == nil {
		stderr = os.Stderr
	}
	stdin := opts.Stdin
	if stdin == nil {
		stdin = os.Stdin
	}

	instance := Instance{
		ID:         id,
		Host:       defaultHost,
		Port:       port,
		URL:        (&url.URL{Scheme: "http", Host: fmt.Sprintf("%s:%d", defaultHost, port)}).String(),
		ExtraPorts: extraPorts,
		CWD:        cwd,
		Command:    append([]string(nil), opts.Command...),
		StartedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	resolvedEnv, err := resolveEnvAssignments(store, service.Name, id, instance, opts.Env)
	if err != nil {
		return err
	}

	command := exec.Command(opts.Command[0], opts.Command[1:]...)
	command.Dir = cwd
	command.Env = childEnv(opts, id, instance, extraPortSpecs, resolvedEnv)
	command.Stdout = stdout
	command.Stderr = stderr
	command.Stdin = stdin

	if err := command.Start(); err != nil {
		return fmt.Errorf("start command: %w", err)
	}

	instance.PID = command.Process.Pid

	registered := false
	if err := RegisterInstance(ctx, store, service.Name, instance); err != nil {
		_ = command.Process.Kill()
		_, _ = waitWithTimeout(command, 2*time.Second)
		return err
	}
	registered = true

	if opts.SwitchOnStart {
		if _, _, err := SwitchInstance(ctx, store, backend, service.Name, id); err != nil {
			_ = UnregisterInstance(context.Background(), store, service.Name, id)
			_ = command.Process.Kill()
			_, _ = waitWithTimeout(command, 2*time.Second)
			return err
		}
		fmt.Fprintf(stdout, "registered and switched %s/%s -> %s\n", service.Name, id, instancePortsSummary(instance))
	} else {
		fmt.Fprintf(stdout, "registered %s/%s -> %s\n", service.Name, id, instancePortsSummary(instance))
	}

	err = waitForCommand(ctx, command)
	if registered {
		if unregisterErr := UnregisterInstance(context.Background(), store, service.Name, id); unregisterErr != nil {
			fmt.Fprintf(stderr, "wp: failed to unregister %s/%s: %v\n", service.Name, id, unregisterErr)
		}
	}

	return commandResult(err)
}

func instancePortsSummary(instance Instance) string {
	parts := []string{fmt.Sprintf("%s:%d", instance.Host, instance.Port)}
	if len(instance.ExtraPorts) == 0 {
		return parts[0]
	}
	names := make([]string, 0, len(instance.ExtraPorts))
	for name := range instance.ExtraPorts {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		extraPort := instance.ExtraPorts[name]
		parts = append(parts, fmt.Sprintf("%s=%s:%d", name, extraPort.Host, extraPort.Port))
	}
	return strings.Join(parts, " ")
}

func childEnv(opts RunOptions, id string, instance Instance, extraPortSpecs []extraPortSpec, extraEnv []string) []string {
	env := append([]string(nil), os.Environ()...)
	env = append(env,
		fmt.Sprintf("%s=%d", opts.PortEnv, instance.Port),
		fmt.Sprintf("WP_SERVICE=%s", opts.ServiceName),
		fmt.Sprintf("WP_ID=%s", id),
	)
	if opts.HostEnv != "" {
		env = append(env, fmt.Sprintf("%s=%s", opts.HostEnv, instance.Host))
	}
	for _, assignment := range extraPortEnvAssignments(extraPortSpecs, instance.ExtraPorts) {
		env = append(env, assignment)
	}
	env = append(env, extraEnv...)
	return env
}

type extraPortSpec struct {
	Name string
	Env  string
}

func allocateExtraPorts(specs []extraPortSpec, usedPorts map[int]bool) (map[string]ExtraPort, error) {
	if len(specs) == 0 {
		return nil, nil
	}
	extraPorts := make(map[string]ExtraPort, len(specs))
	for _, spec := range specs {
		if _, ok := extraPorts[spec.Name]; ok {
			return nil, fmt.Errorf("duplicate extra port %q", spec.Name)
		}
		port, err := findFreePortExcept(usedPorts)
		if err != nil {
			return nil, err
		}
		usedPorts[port] = true
		extraPorts[spec.Name] = ExtraPort{
			Name: spec.Name,
			Host: defaultHost,
			Port: port,
			URL:  (&url.URL{Scheme: "http", Host: fmt.Sprintf("%s:%d", defaultHost, port)}).String(),
		}
	}
	return extraPorts, nil
}

func findFreePortExcept(usedPorts map[int]bool) (int, error) {
	var lastErr error
	for range 20 {
		port, err := FindFreePort()
		if err != nil {
			lastErr = err
			continue
		}
		if !usedPorts[port] {
			return port, nil
		}
	}
	if lastErr != nil {
		return 0, lastErr
	}
	return 0, fmt.Errorf("could not allocate distinct free port")
}

func extraPortEnvAssignments(specs []extraPortSpec, extraPorts map[string]ExtraPort) []string {
	if len(specs) == 0 {
		return nil
	}
	assignments := make([]string, 0, len(specs))
	for _, spec := range specs {
		extraPort, ok := extraPorts[spec.Name]
		if !ok {
			continue
		}
		assignments = append(assignments, fmt.Sprintf("%s=%d", spec.Env, extraPort.Port))
	}
	return assignments
}

func parseExtraPortSpecs(specs []string) ([]extraPortSpec, error) {
	parsed := make([]extraPortSpec, 0, len(specs))
	for _, spec := range specs {
		name, envName, err := parseExtraPortSpec(spec)
		if err != nil {
			return nil, err
		}
		parsed = append(parsed, extraPortSpec{Name: name, Env: envName})
	}
	return parsed, nil
}

func parseExtraPortSpec(spec string) (string, string, error) {
	name, envName, ok := strings.Cut(spec, ":")
	if !ok || name == "" || envName == "" {
		return "", "", fmt.Errorf("extra port must be name:ENV_VAR: %q", spec)
	}
	if strings.Contains(envName, ":") {
		return "", "", fmt.Errorf("extra port must be name:ENV_VAR: %q", spec)
	}
	return name, envName, nil
}

func resolveRunService(store *Store, serviceName string) (Service, error) {
	state, err := store.Load()
	if err != nil {
		return Service{}, err
	}
	service, ok := state.Services[serviceName]
	if !ok {
		return Service{}, fmt.Errorf("unknown service %q; add it with `wp service add %s --alias <domain>` or `wp service add %s --listen <port>`", serviceName, serviceName, serviceName)
	}
	return service, nil
}

func validateRunTarget(store *Store, serviceName string, id string) error {
	state, err := store.Load()
	if err != nil {
		return err
	}
	service, ok := state.Services[serviceName]
	if !ok {
		return fmt.Errorf("unknown service %q; add it with `wp service add %s --alias <domain>` or `wp service add %s --listen <port>`", serviceName, serviceName, serviceName)
	}
	if _, ok := service.Instances[id]; ok {
		return fmt.Errorf("instance %q is already registered for service %q", id, serviceName)
	}
	return nil
}

func resolveCWD(cwd string) (string, error) {
	if cwd == "" {
		current, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("get working directory: %w", err)
		}
		cwd = current
	}
	abs, err := filepath.Abs(cwd)
	if err != nil {
		return "", fmt.Errorf("resolve cwd: %w", err)
	}
	return abs, nil
}

func waitForCommand(ctx context.Context, command *exec.Cmd) error {
	done := make(chan error, 1)
	go func() {
		done <- command.Wait()
	}()

	signals := make(chan os.Signal, 2)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)

	for {
		select {
		case sig := <-signals:
			if command.Process != nil {
				_ = command.Process.Signal(sig)
			}
		case <-ctx.Done():
			if command.Process != nil {
				_ = command.Process.Signal(syscall.SIGTERM)
			}
			select {
			case err := <-done:
				return err
			case <-time.After(2 * time.Second):
				if command.Process != nil {
					_ = command.Process.Kill()
				}
				return <-done
			}
		case err := <-done:
			return err
		}
	}
}

func waitWithTimeout(command *exec.Cmd, timeout time.Duration) (error, bool) {
	done := make(chan error, 1)
	go func() {
		done <- command.Wait()
	}()
	select {
	case err := <-done:
		return err, true
	case <-time.After(timeout):
		return nil, false
	}
}

func commandResult(err error) error {
	if err == nil {
		return nil
	}
	var exitErr *exec.ExitError
	if !errorAs(err, &exitErr) {
		return err
	}
	code := exitErr.ExitCode()
	if code < 0 {
		code = 1
	}
	return CommandExitError{Code: code}
}

func defaultID(cwd string) string {
	branch := gitOutput(cwd, "branch", "--show-current")
	if branch == "" {
		branch = gitOutput(cwd, "rev-parse", "--short", "HEAD")
	}
	if branch == "" {
		branch = filepath.Base(cwd)
	}
	return sanitizeID(branch)
}

func gitOutput(cwd string, args ...string) string {
	gitArgs := append([]string{"-C", cwd}, args...)
	output, err := exec.Command("git", gitArgs...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

var idInvalidPattern = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func sanitizeID(value string) string {
	value = strings.ReplaceAll(value, "/", "--")
	value = idInvalidPattern.ReplaceAllString(value, "-")
	value = strings.Trim(value, "-")
	if value == "" {
		return "default"
	}
	return value
}
