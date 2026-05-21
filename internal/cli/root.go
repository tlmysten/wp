package cli

import (
	"os"

	"github.com/spf13/cobra"
)

type globalOptions struct {
	stateDir       string
	localiasBin    string
	localiasConfig string
}

func NewRootCommand() *cobra.Command {
	opts := &globalOptions{
		stateDir:       os.Getenv("WP_STATE_DIR"),
		localiasBin:    envDefault("WP_LOCALIAS_BIN", "localias"),
		localiasConfig: os.Getenv("WP_LOCALIAS_CONFIG"),
	}

	cmd := &cobra.Command{
		Use:           "wp",
		Short:         "Worktree proxy utilities",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	cmd.PersistentFlags().StringVar(&opts.stateDir, "state-dir", opts.stateDir, "state directory; defaults to the user config directory")
	cmd.PersistentFlags().StringVar(&opts.localiasBin, "localias-bin", opts.localiasBin, "localias binary path")
	cmd.PersistentFlags().StringVar(&opts.localiasConfig, "localias-config", opts.localiasConfig, "localias config file path")

	cmd.AddCommand(newProxyCommand(opts))
	return cmd
}

func envDefault(name string, fallback string) string {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	return value
}
