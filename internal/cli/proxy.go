package cli

import (
	"fmt"
	"os"
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

	cmd.AddCommand(addCmd, listCmd, rmCmd)
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
	cmd := &cobra.Command{
		Use:   "switch <service> <id>",
		Short: "Point a service endpoint at a registered instance",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSwitch(cmd, opts, args[0], args[1])
		},
	}

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
	fmt.Fprintf(cmd.OutOrStdout(), "%s switch     %s -> %s:%d (id=%s)\n", ui.Tag(cmd.OutOrStdout(), "OK"), formatEndpoint(service), instance.Host, instance.Port, instance.ID)
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

func newListCommand(opts *globalOptions) *cobra.Command {
	var serviceFilter string

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
			return writeInstanceList(cmd.OutOrStdout(), state, serviceFilter)
		},
	}

	cmd.Flags().StringVar(&serviceFilter, "service", "", "filter by service")
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
}, state proxy.State, serviceFilter string) error {
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "SERVICE\tENDPOINT\tACTIVE\tID\tPORT\tEXTRA PORTS\tPID\tCWD\tCOMMAND")
	for _, service := range state.SortedServices() {
		if serviceFilter != "" && service.Name != serviceFilter {
			continue
		}
		instances := service.SortedInstances()
		if len(instances) == 0 {
			fmt.Fprintf(w, "%s\t%s\tnone\t-\t-\t-\t-\t-\t-\n",
				service.Name,
				formatEndpoint(service),
			)
			continue
		}
		for _, instance := range instances {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%d\t%s\t%d\t%s\t%s\n",
				service.Name,
				formatEndpoint(service),
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
