// Package cli wires the cobra command tree. Commands stay thin: they load
// inputs, call the pure planning code, and apply the result.
package cli

import "github.com/spf13/cobra"

// Version is set at build time: -ldflags "-X github.com/Uncool-dudes/loadout/internal/cli.Version=..."
var Version = "dev"

func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "loadout",
		Short: "Link a chosen set of Claude Code skills into a project",
		Long: "loadout reads a Nix-built skill manifest and a profiles file, then " +
			"symlinks the skills a directory's profile selects into .claude/skills/.",
		Version: Version,
		// Full usage text on every error is noise; --help still shows it.
		SilenceUsage: true,
		// main prints the error itself so the format stays in one place.
		SilenceErrors: true,
	}
	addInputFlags(root)
	root.AddCommand(newSyncCmd(), newExecCmd(), newStatusCmd(), newListCmd(), newGCCmd(),
		newSourcesCmd(), newAddCmd(), newUpdateCmd(), newRemoveCmd())
	return root
}
