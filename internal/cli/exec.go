package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/Uncool-dudes/loadout/internal/profile"
)

// Replaced in tests, which must not start a real Claude Code.
var (
	lookPath   = exec.LookPath
	execClaude = func(path string, argv []string) error {
		return syscall.Exec(path, argv, os.Environ())
	}
)

func newExecCmd() *cobra.Command {
	var profiles []string
	var noSync, dryRun, optional bool
	cmd := &cobra.Command{
		Use:   "exec [flags] [-- claude-args...]",
		Short: "Sync skills, then start claude with the profiles' settings and MCP servers",
		Long: "exec links the skills the profiles select, merges their settings and MCP " +
			"servers into files under the launch cache, and replaces itself with " +
			"`claude --settings <file> --mcp-config <file>`. Arguments after -- go to claude.\n\n" +
			"With --optional it never stands in the way of claude: when no profile applies " +
			"it starts plain claude, and on any other failure it says so and starts plain " +
			"claude too. That makes `claude() { loadout exec --optional -- \"$@\"; }` safe.\n\n" +
			"Settings and MCP servers only reach sessions started this way: an IDE or " +
			"any other launcher does not apply them.",
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, claudeArgs []string) error {
			argv, files, err := prepareLaunch(cmd, profiles, noSync, dryRun, claudeArgs)
			if err != nil {
				if !optional {
					return err
				}
				if !notApplicable(err) {
					fmt.Fprintf(cmd.ErrOrStderr(), "loadout: %v\nloadout: starting claude without a profile\n", err)
				}
				argv, files = append([]string{"claude"}, claudeArgs...), nil
			}
			if dryRun {
				return printLaunch(cmd.OutOrStdout(), argv, files)
			}
			for _, f := range files {
				if err := writeLaunchFile(f); err != nil {
					return err
				}
			}
			path, err := lookPath(argv[0])
			if err != nil {
				return fmt.Errorf("find claude: %w", err)
			}
			return execClaude(path, argv)
		},
	}
	cmd.Flags().StringSliceVar(&profiles, "profile", nil, "profile to apply, repeatable (default: LOADOUT_PROFILES, then the projects entry)")
	_ = cmd.RegisterFlagCompletionFunc("profile", completeProfiles)
	cmd.Flags().BoolVar(&noSync, "no-sync", false, "skip linking skills and only launch")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print the command and generated files without writing or starting anything")
	cmd.Flags().BoolVar(&optional, "optional", false, "never block claude: start it plain when no profile applies or anything fails")
	return cmd
}

// prepareLaunch syncs skills and works out the claude command line. It is the
// part of exec that can fail, so --optional can fall back around it.
func prepareLaunch(cmd *cobra.Command, profiles []string, noSync, dryRun bool, claudeArgs []string) ([]string, []launchFile, error) {
	in, err := loadInputs(cmd)
	if err != nil {
		return nil, nil, err
	}
	if in.config == nil {
		return nil, nil, errNoConfig
	}
	target, err := resolveTarget(false)
	if err != nil {
		return nil, nil, err
	}
	names, err := selectProfiles(in.config, profiles, target)
	if err != nil {
		return nil, nil, err
	}
	active, err := in.config.Resolve(names...)
	if err != nil {
		return nil, nil, err
	}
	launch, err := active.Launch()
	if err != nil {
		return nil, nil, err
	}

	if !noSync {
		// Launching is routine, so say something only when links change; a
		// dry run shows the whole plan.
		if err := syncSkills(cmd, in, target, names, syncOptions{dryRun: dryRun, quiet: !dryRun}); err != nil {
			return nil, nil, err
		}
	}
	return claudeCommand(launch, defaultLaunchPath(), claudeArgs)
}

// launchFile is a generated JSON file passed to claude.
type launchFile struct {
	path    string
	content []byte
}

// claudeCommand builds the claude command line. Generated files are named by
// a hash of their content, so identical profiles reuse a file and an edited
// profile never rewrites one an open session still reads.
func claudeCommand(l profile.Launch, dir string, passthrough []string) ([]string, []launchFile, error) {
	argv := []string{"claude"}
	var files []launchFile

	add := func(flag, name string, tree map[string]any) error {
		if len(tree) == 0 {
			return nil
		}
		content, err := json.MarshalIndent(tree, "", "  ")
		if err != nil {
			return err
		}
		content = append(content, '\n')
		sum := sha256.Sum256(content)
		path := filepath.Join(dir, hex.EncodeToString(sum[:])[:16], name)
		files = append(files, launchFile{path: path, content: content})
		argv = append(argv, flag, path)
		return nil
	}
	if err := add("--settings", "settings.json", l.Settings); err != nil {
		return nil, nil, err
	}
	if len(l.MCPServers) > 0 {
		if err := add("--mcp-config", "mcp.json", map[string]any{"mcpServers": l.MCPServers}); err != nil {
			return nil, nil, err
		}
		if l.StrictMCP {
			argv = append(argv, "--strict-mcp-config")
		}
	}
	return append(argv, passthrough...), files, nil
}

func writeLaunchFile(f launchFile) error {
	if err := os.MkdirAll(filepath.Dir(f.path), 0o755); err != nil {
		return err
	}
	tmp := f.path + ".tmp"
	if err := os.WriteFile(tmp, f.content, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, f.path)
}

func printLaunch(w io.Writer, argv []string, files []launchFile) error {
	fmt.Fprintln(w, strings.Join(argv, " "))
	for _, f := range files {
		fmt.Fprintf(w, "\n# %s\n%s", f.path, f.content)
	}
	return nil
}
