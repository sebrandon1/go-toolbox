// Package cobrautil removes the boilerplate repeated by every cobra-based CLI
// in the toolbox family: signal-aware execution, version output, and a standard
// shell-completion command.
package cobrautil

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
)

// Execute runs root with a context that is cancelled on SIGINT or SIGTERM, so
// in-flight HTTP requests and other work stop promptly.
func Execute(root *cobra.Command) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return root.ExecuteContext(ctx)
}

// SetVersion sets root.Version and a "name version X" template. An empty
// version becomes "dev".
func SetVersion(root *cobra.Command, version string) {
	if version == "" {
		version = "dev"
	}
	root.Version = version
	root.SetVersionTemplate("{{.Name}} version {{.Version}}\n")
}

// NewCompletionCommand returns the standard completion subcommand for root.
// parentName is only used in the help text.
func NewCompletionCommand(root *cobra.Command, parentName string) *cobra.Command {
	return &cobra.Command{
		Use:   "completion [bash|zsh|fish|powershell]",
		Short: "Generate shell completion scripts",
		Long: fmt.Sprintf(`Generate shell completion scripts for %s.

To load completions in the current shell, source the generated script or follow
your shell's instructions for installing it.`, parentName),
		Args:      cobra.ExactArgs(1),
		ValidArgs: []string{"bash", "zsh", "fish", "powershell"},
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			switch args[0] {
			case "bash":
				return root.GenBashCompletion(out)
			case "zsh":
				return root.GenZshCompletion(out)
			case "fish":
				return root.GenFishCompletion(out, true)
			case "powershell":
				return root.GenPowerShellCompletion(out)
			default:
				return fmt.Errorf("unsupported shell %q: choose bash, zsh, fish, or powershell", args[0])
			}
		},
	}
}
