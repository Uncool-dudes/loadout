package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/Uncool-dudes/loadout/internal/cache"
	"github.com/Uncool-dudes/loadout/internal/skill"
)

func newGCCmd() *cobra.Command {
	var dryRun bool
	var olderThan time.Duration
	cmd := &cobra.Command{
		Use:   "gc",
		Short: "Remove patched skill copies and launch files nothing can use any more",
		Long: "gc removes patched skill variants whose store path is no longer in the " +
			"library (their links are already stale) and interrupted builds, plus " +
			"launch files nobody has regenerated for --older-than. It never touches " +
			"links in a project or the library itself.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			manifest, err := loadManifest(cmd)
			if err != nil {
				return err
			}

			store := cache.New(defaultCachePath())
			staleVariants, err := store.Stale(variantPaths(manifest, store))
			if err != nil {
				return err
			}
			staleLaunch, err := staleLaunchDirs(defaultLaunchPath(), time.Now().Add(-olderThan))
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			verb := "removed"
			if dryRun {
				verb = "would remove"
			}
			for _, p := range append(append([]string{}, staleVariants...), staleLaunch...) {
				fmt.Fprintf(out, "%s %s\n", verb, p)
			}
			fmt.Fprintf(out, "%s %d patched variant(s) and %d launch file set(s)\n", verb, len(staleVariants), len(staleLaunch))
			if dryRun {
				return nil
			}

			if err := store.Remove(staleVariants); err != nil {
				return err
			}
			for _, p := range staleLaunch {
				if err := os.RemoveAll(p); err != nil {
					return err
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "list what would be removed without removing it")
	cmd.Flags().DurationVar(&olderThan, "older-than", 30*24*time.Hour, "remove launch files not regenerated for this long")
	return cmd
}

// variantPaths lists every patched copy the library could currently need: one
// per skill and non-default invocation. A variant outside this set belongs to
// a store path that is gone.
func variantPaths(m *skill.Manifest, store cache.Cache) []string {
	var paths []string
	for _, s := range m.Skills() {
		for _, inv := range []skill.Invocation{skill.User, skill.Model} {
			paths = append(paths, store.Path(s, inv))
		}
	}
	return paths
}

// staleLaunchDirs lists launch file sets last written before cutoff. exec
// rewrites a set every time it launches the same profile, which refreshes the
// directory time, so only abandoned sets age out.
func staleLaunchDirs(dir string, cutoff time.Time) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var stale []string
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			stale = append(stale, filepath.Join(dir, e.Name()))
		}
	}
	return stale, nil
}
