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
	"strings"
	"syscall"
	"time"
)

type RunOptions struct {
	ServiceName   string
	RoleName      string
	ID            string
	Port          int
	PortEnv       string
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
	service, roleName, err := resolveRunRole(store, opts.ServiceName, opts.RoleName)
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
	if err := validateRunTarget(store, service.Name, id, roleName); err != nil {
		return err
	}

	port := opts.Port
	if port == 0 {
		port, err = FindFreePort()
		if err != nil {
			return err
		}
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

	role := Role{
		Name:      roleName,
		Host:      "127.0.0.1",
		Port:      port,
		URL:       (&url.URL{Scheme: "http", Host: fmt.Sprintf("127.0.0.1:%d", port)}).String(),
		CWD:       cwd,
		Command:   append([]string(nil), opts.Command...),
		StartedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	resolvedEnv, err := resolveEnvAssignments(store, service.Name, id, role, opts.Env)
	if err != nil {
		return err
	}

	command := exec.Command(opts.Command[0], opts.Command[1:]...)
	command.Dir = cwd
	command.Env = childEnv(opts, id, role, resolvedEnv)
	command.Stdout = stdout
	command.Stderr = stderr
	command.Stdin = stdin

	if err := command.Start(); err != nil {
		return fmt.Errorf("start command: %w", err)
	}

	role.PID = command.Process.Pid

	registered := false
	if err := RegisterRole(ctx, store, service.Name, id, role); err != nil {
		_ = command.Process.Kill()
		_, _ = waitWithTimeout(command, 2*time.Second)
		return err
	}
	registered = true

	if opts.SwitchOnStart {
		if _, _, _, err := SwitchInstanceRole(ctx, store, backend, service.Name, id, roleName); err != nil {
			_ = UnregisterRole(context.Background(), store, service.Name, id, roleName)
			_ = command.Process.Kill()
			_, _ = waitWithTimeout(command, 2*time.Second)
			return err
		}
		fmt.Fprintf(stdout, "registered and switched %s/%s/%s -> 127.0.0.1:%d\n", service.Name, id, roleName, port)
	} else {
		fmt.Fprintf(stdout, "registered %s/%s/%s -> 127.0.0.1:%d\n", service.Name, id, roleName, port)
	}

	err = waitForCommand(ctx, command)
	if registered {
		if unregisterErr := UnregisterRole(context.Background(), store, service.Name, id, roleName); unregisterErr != nil {
			fmt.Fprintf(stderr, "wp: failed to unregister %s/%s/%s: %v\n", service.Name, id, roleName, unregisterErr)
		}
	}

	return commandResult(err)
}

func childEnv(opts RunOptions, id string, role Role, extraEnv []string) []string {
	env := append([]string(nil), os.Environ()...)
	env = append(env,
		fmt.Sprintf("%s=%d", opts.PortEnv, role.Port),
		fmt.Sprintf("WP_SERVICE=%s", opts.ServiceName),
		fmt.Sprintf("WP_ID=%s", id),
		fmt.Sprintf("WP_ROLE=%s", role.Name),
	)
	if opts.HostEnv != "" {
		env = append(env, fmt.Sprintf("%s=127.0.0.1", opts.HostEnv))
	}
	env = append(env, extraEnv...)
	return env
}

func resolveRunRole(store *Store, serviceName string, roleName string) (Service, string, error) {
	state, err := store.Load()
	if err != nil {
		return Service{}, "", err
	}
	service, ok := state.Services[serviceName]
	if !ok {
		return Service{}, "", fmt.Errorf("unknown service %q; add it with `wp service add %s --alias <domain>`", serviceName, serviceName)
	}
	if roleName == "" {
		roleName = service.AliasRole
	}
	if roleName == "" {
		roleName = DefaultAliasRole
	}
	return service, roleName, nil
}

func validateRunTarget(store *Store, serviceName string, id string, roleName string) error {
	state, err := store.Load()
	if err != nil {
		return err
	}
	service, ok := state.Services[serviceName]
	if !ok {
		return fmt.Errorf("unknown service %q; add it with `wp service add %s --alias <domain>`", serviceName, serviceName)
	}
	instance, ok := service.Instances[id]
	if !ok {
		return nil
	}
	if _, ok := instance.Roles[roleName]; ok {
		return fmt.Errorf("role %q is already registered for service %q instance %q", roleName, serviceName, id)
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
