package cli

import (
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Uncool-dudes/loadout/internal/profile"
)

// Completion runs inside the user's prompt, so every failure here means "no
// suggestions", never an error message.

func completeProfiles(cmd *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	cfg := configQuietly(cmd)
	if cfg == nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	var out []string
	for _, n := range cfg.Names() {
		if strings.HasPrefix(string(n), toComplete) {
			out = append(out, string(n))
		}
	}
	return out, cobra.ShellCompDirectiveNoFileComp
}

func completeSources(cmd *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	manifest, err := loadManifestTo(cmd, io.Discard)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	seen := map[string]bool{}
	var out []string
	for _, s := range manifest.Skills() {
		source := s.Key().Source()
		if !seen[source] && strings.HasPrefix(source, toComplete) {
			seen[source] = true
			out = append(out, source)
		}
	}
	return out, cobra.ShellCompDirectiveNoFileComp
}

// configQuietly reads the config the way loadInputs does, minus the library
// scan that profile names do not need.
func configQuietly(cmd *cobra.Command) *profile.Config {
	path, _ := cmd.Flags().GetString("config")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	cfg, err := profile.Parse(data)
	if err != nil {
		return nil
	}
	return cfg
}
