package cli

import (
	"fmt"
	"os"
	"sort"
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
	var aliasRole string
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
			service, err := proxy.UpsertService(cmd.Context(), store, args[0], alias, aliasRole)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s -> %s (alias role: %s)\n", service.Name, service.Alias, service.AliasRole)
			return nil
		},
	}
	addCmd.Flags().StringVar(&alias, "alias", "", "localias domain, e.g. dev.slush.app")
	addCmd.Flags().StringVar(&aliasRole, "alias-role", proxy.DefaultAliasRole, "role used when switching the service alias")
	addCmd.Flags().StringVar(&aliasRole, "switch-role", proxy.DefaultAliasRole, "deprecated alias for --alias-role")
	_ = addCmd.Flags().MarkDeprecated("switch-role", "use --alias-role instead")

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
			fmt.Fprintln(w, "SERVICE\tALIAS\tALIAS ROLE\tACTIVE")
			for _, service := range state.SortedServices() {
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", service.Name, service.Alias, service.AliasRole, activeTarget(service))
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
	cmd.Flags().StringVar(&runOpts.RoleName, "role", "", "role name; defaults to the service alias role")
	cmd.Flags().StringVar(&runOpts.ID, "id", "", "instance id; defaults to the current git branch")
	cmd.Flags().IntVar(&runOpts.Port, "port", 0, "fixed child port; defaults to an available random port")
	cmd.Flags().StringVar(&runOpts.PortEnv, "port-env", runOpts.PortEnv, "environment variable used to pass the chosen port")
	cmd.Flags().StringArrayVar(&runOpts.ExtraPorts, "extra-port", nil, "extra random port as name:ENV_VAR, e.g. prometheus:PROMETHEUS_PORT")
	cmd.Flags().StringVar(&runOpts.HostEnv, "host-env", "", "optional environment variable used to pass 127.0.0.1")
	cmd.Flags().StringArrayVar(&runOpts.Env, "env", nil, "extra KEY=VALUE environment assignment; supports {{role.port}}, {{role.url}}, and {{role.extraName.url}}")
	cmd.Flags().StringVar(&runOpts.CWD, "cwd", "", "child working directory; defaults to the current directory")
	cmd.Flags().BoolVar(&runOpts.SwitchOnStart, "switch", runOpts.SwitchOnStart, "switch the service alias to this instance after registering")

	return cmd
}

func newRunCommand(opts *globalOptions) *cobra.Command {
	runOpts := proxy.RunOptions{
		PortEnv:       "PORT",
		SwitchOnStart: false,
	}

	cmd := &cobra.Command{
		Use:   "run <service>/<role> [flags] -- <command> [args...]",
		Short: "Run a command for a service role",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			serviceName, roleName, err := parseServiceRole(args[0])
			if err != nil {
				return err
			}
			store, err := storeFromOptions(opts)
			if err != nil {
				return err
			}
			runOpts.ServiceName = serviceName
			runOpts.RoleName = roleName
			runOpts.Command = args[1:]
			runOpts.Stdout = cmd.OutOrStdout()
			runOpts.Stderr = cmd.ErrOrStderr()
			runOpts.Stdin = os.Stdin
			return proxy.RunCommand(cmd.Context(), store, backendFromOptions(opts), runOpts)
		},
	}

	cmd.Flags().StringVar(&runOpts.ID, "id", "", "instance id; defaults to the current git branch")
	cmd.Flags().IntVar(&runOpts.Port, "port", 0, "fixed child port; defaults to an available random port")
	cmd.Flags().StringVar(&runOpts.PortEnv, "port-env", runOpts.PortEnv, "environment variable used to pass the chosen port")
	cmd.Flags().StringArrayVar(&runOpts.ExtraPorts, "extra-port", nil, "extra random port as name:ENV_VAR, e.g. prometheus:PROMETHEUS_PORT")
	cmd.Flags().StringVar(&runOpts.HostEnv, "host-env", "", "optional environment variable used to pass 127.0.0.1")
	cmd.Flags().StringArrayVar(&runOpts.Env, "env", nil, "extra KEY=VALUE environment assignment; supports {{role.port}}, {{role.url}}, and {{role.extraName.url}}")
	cmd.Flags().StringVar(&runOpts.CWD, "cwd", "", "child working directory; defaults to the current directory")
	cmd.Flags().BoolVar(&runOpts.SwitchOnStart, "switch", runOpts.SwitchOnStart, "switch the service alias to this instance after registering")

	return cmd
}

func newProxySwitchCommand(opts *globalOptions) *cobra.Command {
	var serviceName string
	var id string
	var roleName string

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
			service, instance, role, err := proxy.SwitchInstanceRole(cmd.Context(), store, backendFromOptions(opts), serviceName, id, roleName)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s -> %s:%d (%s/%s)\n", service.Alias, role.Host, role.Port, instance.ID, role.Name)
			return nil
		},
	}

	cmd.Flags().StringVar(&serviceName, "service", "", "service name")
	cmd.Flags().StringVar(&id, "id", "", "instance id")
	cmd.Flags().StringVar(&roleName, "role", "", "role name; defaults to the service alias role")
	return cmd
}

func newSwitchCommand(opts *globalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "switch <service>[/<role>] <id>",
		Short: "Point a service alias at a registered instance",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			serviceName, roleName, err := parseOptionalServiceRole(args[0])
			if err != nil {
				return err
			}
			store, err := storeFromOptions(opts)
			if err != nil {
				return err
			}
			service, instance, role, err := proxy.SwitchInstanceRole(cmd.Context(), store, backendFromOptions(opts), serviceName, args[1], roleName)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s -> %s:%d (%s/%s)\n", service.Alias, role.Host, role.Port, instance.ID, role.Name)
			return nil
		},
	}

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
			return writeInstanceList(cmd.OutOrStdout(), state, serviceFilter)
		},
	}

	cmd.Flags().StringVar(&serviceFilter, "service", "", "filter by service")
	return cmd
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

func newProxyUnregisterCommand(opts *globalOptions) *cobra.Command {
	var serviceName string
	var id string
	var roleName string

	cmd := &cobra.Command{
		Use:   "unregister --service <service> --id <id>",
		Short: "Remove an instance or role from wp state",
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
			if roleName == "" {
				if err := proxy.UnregisterInstance(cmd.Context(), store, serviceName, id); err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "unregistered %s/%s\n", serviceName, id)
				return nil
			}
			if err := proxy.UnregisterRole(cmd.Context(), store, serviceName, id, roleName); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "unregistered %s/%s/%s\n", serviceName, id, roleName)
			return nil
		},
	}

	cmd.Flags().StringVar(&serviceName, "service", "", "service name")
	cmd.Flags().StringVar(&id, "id", "", "instance id")
	cmd.Flags().StringVar(&roleName, "role", "", "role name; omit to unregister the entire instance")
	return cmd
}

func newUnregisterCommand(opts *globalOptions) *cobra.Command {
	var roleName string

	cmd := &cobra.Command{
		Use:   "unregister <service> <id>",
		Short: "Remove an instance or role from wp state",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := storeFromOptions(opts)
			if err != nil {
				return err
			}
			if roleName == "" {
				if err := proxy.UnregisterInstance(cmd.Context(), store, args[0], args[1]); err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "unregistered %s/%s\n", args[0], args[1])
				return nil
			}
			if err := proxy.UnregisterRole(cmd.Context(), store, args[0], args[1], roleName); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "unregistered %s/%s/%s\n", args[0], args[1], roleName)
			return nil
		},
	}

	cmd.Flags().StringVar(&roleName, "role", "", "role name; omit to unregister the entire instance")
	return cmd
}

func writeInstanceList(out interface {
	Write([]byte) (int, error)
}, state proxy.State, serviceFilter string) error {
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "SERVICE\tACTIVE\tID\tROLE\tPORT\tEXTRA PORTS\tPID\tCWD\tCOMMAND")
	for _, service := range state.SortedServices() {
		if serviceFilter != "" && service.Name != serviceFilter {
			continue
		}
		for _, instance := range service.SortedInstances() {
			active := ""
			if service.ActiveID == instance.ID && service.ActiveRole == "" {
				active = "*"
			}
			for _, role := range instance.SortedRoles() {
				roleActive := active
				if service.ActiveID == instance.ID && service.ActiveRole == role.Name {
					roleActive = "*"
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%d\t%s\t%d\t%s\t%s\n",
					service.Name,
					roleActive,
					instance.ID,
					role.Name,
					role.Port,
					formatExtraPorts(role.ExtraPorts),
					role.PID,
					role.CWD,
					strings.Join(role.Command, " "),
				)
			}
		}
	}
	return w.Flush()
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

func parseServiceRole(target string) (string, string, error) {
	serviceName, roleName, ok := strings.Cut(target, "/")
	if !ok || serviceName == "" || roleName == "" {
		return "", "", fmt.Errorf("target must be <service>/<role>")
	}
	if strings.Contains(roleName, "/") {
		return "", "", fmt.Errorf("target must be <service>/<role>")
	}
	return serviceName, roleName, nil
}

func parseOptionalServiceRole(target string) (string, string, error) {
	serviceName, roleName, ok := strings.Cut(target, "/")
	if !ok {
		if target == "" {
			return "", "", fmt.Errorf("service is required")
		}
		return target, "", nil
	}
	if serviceName == "" || roleName == "" || strings.Contains(roleName, "/") {
		return "", "", fmt.Errorf("target must be <service> or <service>/<role>")
	}
	return serviceName, roleName, nil
}

func activeTarget(service proxy.Service) string {
	if service.ActiveID == "" {
		return ""
	}
	if service.ActiveRole == "" {
		return service.ActiveID
	}
	return service.ActiveID + "/" + service.ActiveRole
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
