package cli

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/Uncool-dudes/loadout/internal/source"
)

// Replaced in tests, which must not reach GitHub.
var newFetcher = func() source.Fetcher { return source.Git{} }

// sourcesPath finds sources.json: the flag, then LOADOUT_SOURCES, then next to
// the config file. The config is normally a link into the repository that
// holds the Nix setup, so following it lands in the right directory.
func sourcesPath(cmd *cobra.Command) (string, error) {
	if v, _ := cmd.Flags().GetString("sources"); v != "" {
		return v, nil
	}
	if v := os.Getenv("LOADOUT_SOURCES"); v != "" {
		return v, nil
	}
	configPath, _ := cmd.Flags().GetString("config")
	real, err := filepath.EvalSymlinks(configPath)
	if err != nil {
		return "", fmt.Errorf("cannot place sources.json next to %s (%v): pass --sources", configPath, err)
	}
	return filepath.Join(filepath.Dir(real), "sources.json"), nil
}

func loadSources(path string, mustExist bool) (source.Set, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) && !mustExist {
		return source.Parse(nil)
	}
	if err != nil {
		return source.Set{}, fmt.Errorf("read sources: %w", err)
	}
	set, err := source.Parse(data)
	if err != nil {
		return source.Set{}, fmt.Errorf("%s: %w", path, err)
	}
	return set, nil
}

func saveSources(path string, set source.Set) error {
	data, err := set.Marshal()
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func newAddCmd() *cobra.Command {
	var name, path string
	var include []string
	var noLibrary, pinned, dryRun bool
	cmd := &cobra.Command{
		Use:   "add owner/repo[@ref]",
		Short: "Pin a GitHub repository as a skill source in sources.json",
		Long: "add resolves the ref (default branch if none) to a commit, prefetches it " +
			"for the hash, works out where the skills are, and records the source in " +
			"sources.json. Select its skills in loadout.hjson afterwards as <name>/<skill>, " +
			"then rebuild.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			owner, repo, ref, err := parseSpec(args[0])
			if err != nil {
				return err
			}
			if name == "" {
				name = owner
			}
			file, err := sourcesPath(cmd)
			if err != nil {
				return err
			}
			set, err := loadSources(file, false)
			if err != nil {
				return err
			}
			if _, exists := set.Get(name); exists {
				return fmt.Errorf("a source named %q already exists: pass --name for a different name, or use `loadout update %s`", name, name)
			}

			fetcher := newFetcher()
			out := cmd.OutOrStdout()
			rev, err := fetcher.Resolve(cmd.Context(), owner, repo, ref)
			if err != nil {
				return fmt.Errorf("resolve %s/%s: %w", owner, repo, err)
			}
			hash, store, err := fetcher.Prefetch(cmd.Context(), owner, repo, rev)
			if err != nil {
				return fmt.Errorf("prefetch %s/%s: %w", owner, repo, err)
			}
			fmt.Fprintf(out, "resolved %s/%s at %s\n", owner, repo, rev[:12])

			src := source.Source{Owner: owner, Repo: repo, Ref: ref, Rev: rev, Hash: hash, Pinned: pinned}
			switch {
			case noLibrary:
				f := false
				src.Library = &f
				fmt.Fprintln(out, "fetched only: its skills are not offered by the library")
			case path != "" || len(include) > 0:
				src.Path, src.Include = path, include
				if err := checkInside(store, path, include); err != nil {
					return err
				}
			default:
				layout, err := source.Detect(store)
				if err != nil {
					return fmt.Errorf("%w (pass --path or --include, or --no-library)", err)
				}
				src.Path, src.Include = layout.Path, layout.Include
				fmt.Fprintln(out, layout.Describe())
				for _, w := range layout.Warnings {
					fmt.Fprintln(cmd.ErrOrStderr(), "warning:", w)
				}
			}

			if err := set.Add(name, src); err != nil {
				return err
			}
			if dryRun {
				fmt.Fprintf(out, "would add %s to %s\n", name, file)
				return nil
			}
			if err := saveSources(file, set); err != nil {
				return err
			}
			fmt.Fprintf(out, "added %s to %s\n", name, file)
			if !noLibrary {
				fmt.Fprintf(out, "next: select skills as %s/<skill> in a profile in loadout.hjson, then rebuild\n", name)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "source name used in selectors (default: the owner)")
	cmd.Flags().StringVar(&path, "path", "", "directory inside the repository to scan for skills")
	cmd.Flags().StringSliceVar(&include, "include", nil, "file or directory to copy into the source, repeatable")
	cmd.Flags().BoolVar(&noLibrary, "no-library", false, "fetch only, for plugins wanted for their hooks or commands")
	cmd.Flags().BoolVar(&pinned, "pinned", false, "keep `loadout update` from moving this source")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show what would be added without writing sources.json")
	return cmd
}

// parseSpec reads owner/repo[@ref], also accepting a github: prefix.
func parseSpec(spec string) (owner, repo, ref string, err error) {
	spec = strings.TrimPrefix(spec, "github:")
	spec, ref, _ = strings.Cut(spec, "@")
	owner, repo, ok := strings.Cut(spec, "/")
	if !ok || owner == "" || repo == "" || strings.Contains(repo, "/") {
		return "", "", "", fmt.Errorf("invalid source %q: want owner/repo or owner/repo@ref", spec)
	}
	return owner, repo, ref, nil
}

func checkInside(store, path string, include []string) error {
	for _, p := range append([]string{path}, include...) {
		if p == "" {
			continue
		}
		if _, err := os.Stat(filepath.Join(store, p)); err != nil {
			return fmt.Errorf("%s does not exist in the repository", p)
		}
	}
	return nil
}

func newUpdateCmd() *cobra.Command {
	var force, dryRun bool
	cmd := &cobra.Command{
		Use:   "update [name...]",
		Short: "Move sources to the latest commit of their ref",
		Long: "update re-resolves each source's ref (or default branch), and rewrites the " +
			"revision and hash when it moved. Pinned sources are skipped unless named " +
			"or --force is given.",
		Args:              cobra.ArbitraryArgs,
		ValidArgsFunction: completeSourceNames,
		RunE: func(cmd *cobra.Command, args []string) error {
			file, err := sourcesPath(cmd)
			if err != nil {
				return err
			}
			set, err := loadSources(file, true)
			if err != nil {
				return err
			}
			names := args
			explicit := len(args) > 0
			if !explicit {
				names = set.Names()
			}

			fetcher := newFetcher()
			out := cmd.OutOrStdout()
			changed := false
			for _, name := range names {
				src, ok := set.Get(name)
				if !ok {
					return fmt.Errorf("%w: %s", source.ErrNotFound, name)
				}
				if src.Pinned && !explicit && !force {
					fmt.Fprintf(out, "%-24s pinned\n", name)
					continue
				}
				rev, err := fetcher.Resolve(cmd.Context(), src.Owner, src.Repo, src.Ref)
				if err != nil {
					return fmt.Errorf("%s: resolve: %w", name, err)
				}
				if rev == src.Rev {
					fmt.Fprintf(out, "%-24s unchanged\n", name)
					continue
				}
				hash, _, err := fetcher.Prefetch(cmd.Context(), src.Owner, src.Repo, rev)
				if err != nil {
					return fmt.Errorf("%s: prefetch: %w", name, err)
				}
				fmt.Fprintf(out, "%-24s %s -> %s\n", name, src.Rev[:12], rev[:12])
				src.Rev, src.Hash = rev, hash
				if err := set.Replace(name, src); err != nil {
					return err
				}
				changed = true
			}
			if !changed || dryRun {
				return nil
			}
			return saveSources(file, set)
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "also move pinned sources")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show what would move without writing sources.json")
	return cmd
}

func newRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:               "remove name",
		Short:             "Remove a source from sources.json",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeSourceNames,
		RunE: func(cmd *cobra.Command, args []string) error {
			file, err := sourcesPath(cmd)
			if err != nil {
				return err
			}
			set, err := loadSources(file, true)
			if err != nil {
				return err
			}
			if err := set.Remove(args[0]); err != nil {
				return err
			}
			if err := saveSources(file, set); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "removed %s: also drop its skills from profiles in loadout.hjson, then rebuild\n", args[0])
			return nil
		},
	}
}

func newSourcesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "sources",
		Short: "List the pinned skill sources",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			file, err := sourcesPath(cmd)
			if err != nil {
				return err
			}
			set, err := loadSources(file, true)
			if err != nil {
				return err
			}
			return writeSources(cmd.OutOrStdout(), set)
		},
	}
}

func writeSources(w io.Writer, set source.Set) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tREPOSITORY\tREV\tWHERE\tNOTES")
	for _, name := range set.Names() {
		src, _ := set.Get(name)
		where := src.Path
		switch {
		case len(src.Include) > 0:
			where = "include " + strings.Join(src.Include, ",")
		case where == "":
			where = "."
		}
		var notes []string
		if !src.InLibrary() {
			notes = append(notes, "not in library")
		}
		if src.Pinned {
			notes = append(notes, "pinned")
		}
		if src.Ref != "" {
			notes = append(notes, "ref "+src.Ref)
		}
		fmt.Fprintf(tw, "%s\t%s/%s\t%s\t%s\t%s\n", name, src.Owner, src.Repo, src.Rev[:12], where, strings.Join(notes, ", "))
	}
	return tw.Flush()
}

// completeSourceNames completes names from sources.json, quietly.
func completeSourceNames(cmd *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	file, err := sourcesPath(cmd)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	set, err := loadSources(file, true)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	var out []string
	for _, n := range set.Names() {
		if strings.HasPrefix(n, toComplete) {
			out = append(out, n)
		}
	}
	return out, cobra.ShellCompDirectiveNoFileComp
}
