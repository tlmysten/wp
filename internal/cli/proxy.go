package cli

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/tlmysten/worktree-tools/internal/proxy"
)

func newProxyCommand(opts *globalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "proxy",
		Short: "Register worktree-local dev servers and switch localias routes",
	}

	cmd.AddCommand(
		newProxyServiceCommand(opts),
		newProxyRunCommand(opts),
		newProxySwitchCommand(opts),
		newProxyListCommand(opts),
		newProxyUnregisterCommand(opts),
	)

	return cmd
}

func newProxyServiceCommand(opts *globalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "service",
		Short: "Manage proxy services",
	}

	var alias string
	addCmd := &cobra.Command{
		Use:   "add <service>",
		Short: "Add or update a service alias",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if alias == "" {
				return fmt.Errorf("--alias is required")
			}
			store, err := storeFromOptions(opts)
			if err != nil {
				return err
			}
			service, err := proxy.UpsertService(cmd.Context(), store, args[0], alias)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s -> %s\n", service.Name, service.Alias)
			return nil
		},
	}
	addCmd.Flags().StringVar(&alias, "alias", "", "localias domain, e.g. dev.slush.app")

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
			fmt.Fprintln(w, "SERVICE\tALIAS\tACTIVE")
			for _, service := range state.SortedServices() {
				fmt.Fprintf(w, "%s\t%s\t%s\n", service.Name, service.Alias, service.ActiveID)
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
			fmt.Fprintf(cmd.OutOrStdout(), "removed service %s\n", args[0])
			return nil
		},
	}

	cmd.AddCommand(addCmd, listCmd, rmCmd)
	return cmd
}

func newProxyRunCommand(opts *globalOptions) *cobra.Command {
	runOpts := proxy.RunOptions{
		PortEnv:       "PORT",
		SwitchOnStart: true,
	}

	cmd := &cobra.Command{
		Use:   "run --service <service> [flags] -- <command> [args...]",
		Short: "Run a command on a registered port",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if runOpts.ServiceName == "" {
				return fmt.Errorf("--service is required")
			}
			store, err := storeFromOptions(opts)
			if err != nil {
				return err
			}
			backend := backendFromOptions(opts)
			runOpts.Command = args
			runOpts.Stdout = cmd.OutOrStdout()
			runOpts.Stderr = cmd.ErrOrStderr()
			runOpts.Stdin = os.Stdin
			return proxy.RunCommand(cmd.Context(), store, backend, runOpts)
		},
	}

	cmd.Flags().StringVar(&runOpts.ServiceName, "service", "", "service name")
	cmd.Flags().StringVar(&runOpts.ID, "id", "", "instance id; defaults to the current git branch")
	cmd.Flags().IntVar(&runOpts.Port, "port", 0, "fixed child port; defaults to an available random port")
	cmd.Flags().StringVar(&runOpts.PortEnv, "port-env", runOpts.PortEnv, "environment variable used to pass the chosen port")
	cmd.Flags().StringVar(&runOpts.HostEnv, "host-env", "", "optional environment variable used to pass 127.0.0.1")
	cmd.Flags().StringVar(&runOpts.CWD, "cwd", "", "child working directory; defaults to the current directory")
	cmd.Flags().BoolVar(&runOpts.SwitchOnStart, "switch", runOpts.SwitchOnStart, "switch the service alias to this instance after registering")

	return cmd
}

func newProxySwitchCommand(opts *globalOptions) *cobra.Command {
	var serviceName string
	var id string

	cmd := &cobra.Command{
		Use:   "switch --service <service> --id <id>",
		Short: "Point a service alias at a registered instance",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if serviceName == "" {
				return fmt.Errorf("--service is required")
			}
			if id == "" {
				return fmt.Errorf("--id is required")
			}
			store, err := storeFromOptions(opts)
			if err != nil {
				return err
			}
			service, instance, err := proxy.SwitchInstance(cmd.Context(), store, backendFromOptions(opts), serviceName, id)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s -> %s:%d (%s)\n", service.Alias, instance.Host, instance.Port, instance.ID)
			return nil
		},
	}

	cmd.Flags().StringVar(&serviceName, "service", "", "service name")
	cmd.Flags().StringVar(&id, "id", "", "instance id")
	return cmd
}

func newProxyListCommand(opts *globalOptions) *cobra.Command {
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
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "SERVICE\tACTIVE\tID\tPORT\tPID\tCWD\tCOMMAND")
			for _, service := range state.SortedServices() {
				if serviceFilter != "" && service.Name != serviceFilter {
					continue
				}
				for _, instance := range service.SortedInstances() {
					active := ""
					if service.ActiveID == instance.ID {
						active = "*"
					}
					fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%d\t%s\t%s\n",
						service.Name,
						active,
						instance.ID,
						instance.Port,
						instance.PID,
						instance.CWD,
						strings.Join(instance.Command, " "),
					)
				}
			}
			return w.Flush()
		},
	}

	cmd.Flags().StringVar(&serviceFilter, "service", "", "filter by service")
	return cmd
}

func newProxyUnregisterCommand(opts *globalOptions) *cobra.Command {
	var serviceName string
	var id string

	cmd := &cobra.Command{
		Use:   "unregister --service <service> --id <id>",
		Short: "Remove an instance from wp state",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if serviceName == "" {
				return fmt.Errorf("--service is required")
			}
			if id == "" {
				return fmt.Errorf("--id is required")
			}
			store, err := storeFromOptions(opts)
			if err != nil {
				return err
			}
			if err := proxy.UnregisterInstance(cmd.Context(), store, serviceName, id); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "unregistered %s/%s\n", serviceName, id)
			return nil
		},
	}

	cmd.Flags().StringVar(&serviceName, "service", "", "service name")
	cmd.Flags().StringVar(&id, "id", "", "instance id")
	return cmd
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
