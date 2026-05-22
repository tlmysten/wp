package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/tlmysten/worktree-tools/internal/proxy"
	"github.com/tlmysten/worktree-tools/internal/ui"
)

func newServiceCommand(opts *globalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "service",
		Short: "Manage proxy services",
	}

	var alias string
	var listenPort int
	addCmd := &cobra.Command{
		Use:   "add <service>",
		Short: "Add or update a service endpoint",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := storeFromOptions(opts)
			if err != nil {
				return err
			}
			service, err := proxy.UpsertService(cmd.Context(), store, args[0], alias, listenPort)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s service    %s -> %s\n", ui.Tag(cmd.OutOrStdout(), "OK"), service.Name, formatEndpoint(service))
			return nil
		},
	}
	addCmd.Flags().StringVar(&alias, "alias", "", "localias domain, e.g. dev.slush.app")
	addCmd.Flags().IntVar(&listenPort, "listen", 0, "local port for wp's built-in reverse proxy, e.g. 3003")

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List configured services",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := storeFromOptions(opts)
			if err != nil {
				return err
			}
			state, err := store.Load()
			if err != nil {
				return err
			}
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "SERVICE\tENDPOINT\tACTIVE\tINSTANCES")
			for _, service := range state.SortedServices() {
				fmt.Fprintf(w, "%s\t%s\t%s\t%d\n", service.Name, formatEndpoint(service), service.ActiveID, len(service.Instances))
			}
			return w.Flush()
		},
	}

	rmCmd := &cobra.Command{
		Use:   "rm <service>",
		Short: "Remove a configured service from wp state",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := storeFromOptions(opts)
			if err != nil {
				return err
			}
			if err := proxy.RemoveService(cmd.Context(), store, args[0]); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s removed    service %s\n", ui.Tag(cmd.OutOrStdout(), "OK"), args[0])
			return nil
		},
	}

	renameCmd := &cobra.Command{
		Use:   "rename <old-service> <new-service>",
		Short: "Rename a configured service",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := storeFromOptions(opts)
			if err != nil {
				return err
			}
			service, err := proxy.RenameService(cmd.Context(), store, args[0], args[1])
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s renamed    %s -> %s\n", ui.Tag(cmd.OutOrStdout(), "OK"), args[0], service.Name)
			return nil
		},
	}

	cmd.AddCommand(addCmd, listCmd, rmCmd, renameCmd)
	return cmd
}

func newRunCommand(opts *globalOptions) *cobra.Command {
	runOpts := proxy.RunOptions{
		PortEnv: "PORT",
	}

	cmd := &cobra.Command{
		Use:   "run <service> [flags] -- <command> [args...]",
		Short: "Run a command on a registered port",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := storeFromOptions(opts)
			if err != nil {
				return err
			}
			runOpts.ServiceName = args[0]
			runOpts.Command = args[1:]
			runOpts.Stdout = cmd.OutOrStdout()
			runOpts.Stderr = cmd.ErrOrStderr()
			runOpts.Stdin = os.Stdin
			return proxy.RunCommand(cmd.Context(), store, backendFromOptions(opts), runOpts)
		},
	}

	addRunFlags(cmd, &runOpts)
	return cmd
}

func addRunFlags(cmd *cobra.Command, runOpts *proxy.RunOptions) {
	cmd.Flags().StringVar(&runOpts.ID, "id", "", "instance id; defaults to the current git branch")
	cmd.Flags().IntVar(&runOpts.Port, "port", 0, "fixed child port; defaults to an available random port")
	cmd.Flags().StringVar(&runOpts.PortEnv, "port-env", runOpts.PortEnv, "environment variable used to pass the chosen port")
	cmd.Flags().StringArrayVar(&runOpts.ExtraPorts, "extra-port", nil, "extra random port as name:ENV_VAR, e.g. prometheus:PROMETHEUS_PORT")
	cmd.Flags().StringVar(&runOpts.HostEnv, "host-env", "", "optional environment variable used to pass the host")
	cmd.Flags().StringArrayVar(&runOpts.Env, "env", nil, "extra KEY=VALUE environment assignment; supports {{service.url}}, {{service.port}}, and {{service.extraName.url}}")
	cmd.Flags().StringVar(&runOpts.CWD, "cwd", "", "child working directory; defaults to the current directory")
	cmd.Flags().BoolVar(&runOpts.SwitchOnStart, "switch", false, "switch the service endpoint to this instance after registering")
}

func newSwitchCommand(opts *globalOptions) *cobra.Command {
	var id string

	cmd := &cobra.Command{
		Use:   "switch <service> <id>",
		Short: "Point a service endpoint at a registered instance",
		Args: func(cmd *cobra.Command, args []string) error {
			if id != "" {
				if len(args) != 0 {
					return fmt.Errorf("use either `wp switch <service> <id>` or `wp switch --id <id>`")
				}
				return nil
			}
			return cobra.ExactArgs(2)(cmd, args)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if id != "" {
				return runSwitchAll(cmd, opts, id)
			}
			return runSwitch(cmd, opts, args[0], args[1])
		},
	}

	cmd.Flags().StringVar(&id, "id", "", "switch every service with a matching instance id")
	return cmd
}

func runSwitch(cmd *cobra.Command, opts *globalOptions, serviceName string, id string) error {
	store, err := storeFromOptions(opts)
	if err != nil {
		return err
	}
	service, instance, err := proxy.SwitchInstance(cmd.Context(), store, backendFromOptions(opts), serviceName, id)
	if err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "%s switch     %s %s -> %s:%d (id=%s)\n", ui.Tag(cmd.OutOrStdout(), "OK"), service.Name, formatEndpoint(service), instance.Host, instance.Port, instance.ID)
	return nil
}

func runSwitchAll(cmd *cobra.Command, opts *globalOptions, id string) error {
	store, err := storeFromOptions(opts)
	if err != nil {
		return err
	}
	state, err := store.Load()
	if err != nil {
		return err
	}
	switched := 0
	for _, service := range state.SortedServices() {
		if _, ok := service.Instances[id]; !ok {
			continue
		}
		if err := runSwitch(cmd, opts, service.Name, id); err != nil {
			return err
		}
		switched++
	}
	if switched == 0 {
		return fmt.Errorf("no services have instance %q", id)
	}
	return nil
}

func newServeCommand(opts *globalOptions) *cobra.Command {
	var host string

	cmd := &cobra.Command{
		Use:   "serve <service>",
		Short: "Serve a fixed local port for a registered service",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runServe(cmd, opts, args[0], host)
		},
	}

	cmd.Flags().StringVar(&host, "host", "127.0.0.1", "host for the built-in reverse proxy")
	cmd.AddCommand(newServeStatusCommand(opts))
	return cmd
}

func newServeStatusCommand(opts *globalOptions) *cobra.Command {
	var host string

	cmd := &cobra.Command{
		Use:   "status [service]",
		Short: "Check whether wp serve is listening",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			serviceName := ""
			if len(args) > 0 {
				serviceName = args[0]
			}
			return runServeStatus(cmd, opts, serviceName, host)
		},
	}

	cmd.Flags().StringVar(&host, "host", "127.0.0.1", "host to probe")
	return cmd
}

func runServe(cmd *cobra.Command, opts *globalOptions, serviceName string, host string) error {
	store, err := storeFromOptions(opts)
	if err != nil {
		return err
	}
	return proxy.ServeService(cmd.Context(), store, proxy.ServeOptions{
		ServiceName: serviceName,
		Host:        host,
		Stdout:      cmd.OutOrStdout(),
	})
}

func runServeStatus(cmd *cobra.Command, opts *globalOptions, serviceName string, host string) error {
	store, err := storeFromOptions(opts)
	if err != nil {
		return err
	}
	state, err := store.Load()
	if err != nil {
		return err
	}
	services := state.SortedServices()
	if serviceName != "" {
		service, ok := state.Services[serviceName]
		if !ok {
			return fmt.Errorf("unknown service %q", serviceName)
		}
		services = []proxy.Service{service}
	}
	rows := collectServeStatus(cmd.Context(), services, host)
	if err := writeServeStatus(cmd.OutOrStdout(), rows); err != nil {
		return err
	}
	if serviceName != "" && (len(rows) == 0 || !rows[0].Running) {
		return proxy.CommandExitError{Code: 1}
	}
	return nil
}

type serveStatusRow struct {
	Service  proxy.Service
	Endpoint string
	Status   string
	Detail   string
	Running  bool
}

func collectServeStatus(ctx context.Context, services []proxy.Service, host string) []serveStatusRow {
	rows := make([]serveStatusRow, 0, len(services))
	for _, service := range services {
		row := serveStatusRow{
			Service:  service,
			Endpoint: formatEndpoint(service),
		}
		if service.ListenPort <= 0 {
			row.Status = "skip"
			row.Detail = "alias service"
			rows = append(rows, row)
			continue
		}
		status := proxy.CheckServeStatus(ctx, service, host)
		row.Running = status.Running
		if status.Running {
			row.Status = "running"
			row.Detail = fmt.Sprintf("listening on %s:%d", status.Host, service.ListenPort)
		} else {
			row.Status = "stopped"
			row.Detail = status.Error.Error()
		}
		rows = append(rows, row)
	}
	return rows
}

func writeServeStatus(out io.Writer, rows []serveStatusRow) error {
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "SERVICE\tENDPOINT\tSTATUS\tDETAIL")
	for _, row := range rows {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", row.Service.Name, row.Endpoint, row.Status, row.Detail)
	}
	return w.Flush()
}

func newCurrentCommand(opts *globalOptions) *cobra.Command {
	var field string
	var extraPortName string

	cmd := &cobra.Command{
		Use:   "current <service>",
		Short: "Print the active target for a service",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := storeFromOptions(opts)
			if err != nil {
				return err
			}
			state, err := store.Load()
			if err != nil {
				return err
			}
			_, instance, err := proxy.ActiveInstance(state, args[0])
			if err != nil {
				return err
			}
			value, err := currentValue(instance, field, extraPortName)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), value)
			return nil
		},
	}

	cmd.Flags().StringVar(&field, "field", "url", "field to print: url, host, port, or id")
	cmd.Flags().StringVar(&extraPortName, "extra", "", "print a named extra port instead of the primary port")
	return cmd
}

func currentValue(instance proxy.Instance, field string, extraPortName string) (string, error) {
	if extraPortName != "" {
		extraPort, ok := instance.ExtraPorts[extraPortName]
		if !ok {
			return "", fmt.Errorf("unknown extra port %q for instance %q", extraPortName, instance.ID)
		}
		switch field {
		case "url":
			return extraPort.URL, nil
		case "host":
			return extraPort.Host, nil
		case "port":
			return fmt.Sprintf("%d", extraPort.Port), nil
		case "id":
			return instance.ID, nil
		default:
			return "", fmt.Errorf("unknown field %q", field)
		}
	}
	switch field {
	case "url":
		return instance.URL, nil
	case "host":
		return instance.Host, nil
	case "port":
		return fmt.Sprintf("%d", instance.Port), nil
	case "id":
		return instance.ID, nil
	default:
		return "", fmt.Errorf("unknown field %q", field)
	}
}

func newListCommand(opts *globalOptions) *cobra.Command {
	var serviceFilter string
	var host string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List registered instances",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := storeFromOptions(opts)
			if err != nil {
				return err
			}
			state, err := store.Load()
			if err != nil {
				return err
			}
			return writeInstanceList(cmd.OutOrStdout(), cmd.Context(), state, serviceFilter, host)
		},
	}

	cmd.Flags().StringVar(&serviceFilter, "service", "", "filter by service")
	cmd.Flags().StringVar(&host, "host", "127.0.0.1", "host to probe for wp serve status")
	return cmd
}

func newUnregisterCommand(opts *globalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "unregister <service> <id>",
		Short: "Remove an instance from wp state",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUnregister(cmd, opts, args[0], args[1])
		},
	}

	return cmd
}

func newPruneCommand(opts *globalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "prune [service]",
		Short: "Remove registered instances whose process is gone",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			serviceName := ""
			if len(args) > 0 {
				serviceName = args[0]
			}
			store, err := storeFromOptions(opts)
			if err != nil {
				return err
			}
			pruned, err := proxy.PruneInstances(cmd.Context(), store, serviceName)
			if err != nil {
				return err
			}
			if len(pruned) == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "%s prune     no stale instances\n", ui.Tag(cmd.OutOrStdout(), "OK"))
				return nil
			}
			for _, item := range pruned {
				fmt.Fprintf(cmd.OutOrStdout(), "%s prune     %s/%s (pid=%d)\n", ui.Tag(cmd.OutOrStdout(), "OK"), item.ServiceName, item.Instance.ID, item.Instance.PID)
			}
			return nil
		},
	}
	return cmd
}

func newDoctorCommand(opts *globalOptions) *cobra.Command {
	var host string

	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Check wp configuration and active targets",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := storeFromOptions(opts)
			if err != nil {
				return err
			}
			state, err := store.Load()
			if err != nil {
				return err
			}
			return writeDoctor(cmd.OutOrStdout(), cmd.Context(), opts, state, host)
		},
	}

	cmd.Flags().StringVar(&host, "host", "127.0.0.1", "host to probe for wp serve status")
	return cmd
}

func writeDoctor(out io.Writer, ctx context.Context, opts *globalOptions, state proxy.State, host string) error {
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "CHECK\tSTATUS\tDETAIL")
	services := state.SortedServices()
	fmt.Fprintf(w, "state\tok\t%d configured services\n", len(services))

	needsLocalias := false
	for _, service := range services {
		if service.Alias != "" {
			needsLocalias = true
			break
		}
	}
	if needsLocalias {
		if path, err := exec.LookPath(opts.localiasBin); err == nil {
			fmt.Fprintf(w, "localias\tok\t%s\n", path)
		} else {
			fmt.Fprintf(w, "localias\twarn\t%s\n", err)
		}
	} else {
		fmt.Fprintln(w, "localias\tskip\tno alias services")
	}

	serveRows := collectServeStatus(ctx, services, host)
	for _, row := range serveRows {
		if row.Status == "skip" {
			continue
		}
		status := "ok"
		if !row.Running {
			status = "warn"
		}
		fmt.Fprintf(w, "serve:%s\t%s\t%s\n", row.Service.Name, status, row.Detail)
	}

	for _, service := range services {
		if service.ActiveID == "" {
			fmt.Fprintf(w, "active:%s\twarn\tno active instance\n", service.Name)
			continue
		}
		instance, ok := service.Instances[service.ActiveID]
		if !ok {
			fmt.Fprintf(w, "active:%s\twarn\tactive instance %q is missing\n", service.Name, service.ActiveID)
			continue
		}
		if err := proxy.CheckTCP(ctx, instance.Host, instance.Port); err != nil {
			fmt.Fprintf(w, "target:%s\twarn\t%s\n", service.Name, err)
			continue
		}
		fmt.Fprintf(w, "target:%s\tok\t%s\n", service.Name, instance.URL)
	}
	return w.Flush()
}

func runUnregister(cmd *cobra.Command, opts *globalOptions, serviceName string, id string) error {
	store, err := storeFromOptions(opts)
	if err != nil {
		return err
	}
	if err := proxy.UnregisterInstance(cmd.Context(), store, serviceName, id); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "%s removed    %s/%s\n", ui.Tag(cmd.OutOrStdout(), "OK"), serviceName, id)
	return nil
}

func writeInstanceList(out interface {
	Write([]byte) (int, error)
}, ctx context.Context, state proxy.State, serviceFilter string, host string) error {
	serveStatusByService := make(map[string]string)
	serveRows := collectServeStatus(ctx, state.SortedServices(), host)
	for _, row := range serveRows {
		if row.Status == "skip" {
			serveStatusByService[row.Service.Name] = "-"
		} else {
			serveStatusByService[row.Service.Name] = row.Status
		}
	}

	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "SERVICE\tENDPOINT\tSERVE\tACTIVE\tID\tPORT\tEXTRA PORTS\tPID\tCWD\tCOMMAND")
	for _, service := range state.SortedServices() {
		if serviceFilter != "" && service.Name != serviceFilter {
			continue
		}
		instances := service.SortedInstances()
		if len(instances) == 0 {
			fmt.Fprintf(w, "%s\t%s\t%s\tnone\t-\t-\t-\t-\t-\t-\n",
				service.Name,
				formatEndpoint(service),
				serveStatusByService[service.Name],
			)
			continue
		}
		for _, instance := range instances {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%d\t%s\t%d\t%s\t%s\n",
				service.Name,
				formatEndpoint(service),
				serveStatusByService[service.Name],
				formatActive(service, instance),
				instance.ID,
				instance.Port,
				formatExtraPorts(instance.ExtraPorts),
				instance.PID,
				instance.CWD,
				strings.Join(instance.Command, " "),
			)
		}
	}
	return w.Flush()
}

func formatActive(service proxy.Service, instance proxy.Instance) string {
	if service.ActiveID == instance.ID {
		return "yes"
	}
	return "no"
}

func formatEndpoint(service proxy.Service) string {
	if service.Alias != "" {
		return service.Alias
	}
	if service.ListenPort > 0 {
		return fmt.Sprintf("127.0.0.1:%d", service.ListenPort)
	}
	return ""
}

func formatExtraPorts(extraPorts map[string]proxy.ExtraPort) string {
	if len(extraPorts) == 0 {
		return ""
	}
	names := make([]string, 0, len(extraPorts))
	for name := range extraPorts {
		names = append(names, name)
	}
	sort.Strings(names)
	parts := make([]string, 0, len(names))
	for _, name := range names {
		parts = append(parts, fmt.Sprintf("%s:%d", name, extraPorts[name].Port))
	}
	return strings.Join(parts, ",")
}

func storeFromOptions(opts *globalOptions) (*proxy.Store, error) {
	return proxy.NewStore(opts.stateDir)
}

func backendFromOptions(opts *globalOptions) proxy.Backend {
	return proxy.LocaliasBackend{
		Binary:     opts.localiasBin,
		ConfigFile: opts.localiasConfig,
		Reload:     true,
	}
}
