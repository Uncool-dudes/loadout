package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/Uncool-dudes/loadout/internal/profile"
	"github.com/Uncool-dudes/loadout/internal/skill"
)

type listRow struct {
	Key              string   `json:"key"`
	Path             string   `json:"path"`
	DescriptionBytes int      `json:"descriptionBytes"`
	EstimatedTokens  int      `json:"estimatedTokens"`
	Profiles         []string `json:"profiles"`
}

func newListCmd() *cobra.Command {
	var output, source, profileName string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List every skill in the manifest and the profiles using it",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			in, err := loadInputs(cmd)
			if err != nil {
				return err
			}
			rows, err := buildListRows(in, source, profileName)
			if err != nil {
				return err
			}
			return writeRows(cmd.OutOrStdout(), rows, output, in.config != nil)
		},
	}
	cmd.Flags().StringVarP(&output, "output", "o", "text", "output format: text or json")
	cmd.Flags().StringVar(&source, "source", "", "only skills from this source")
	cmd.Flags().StringVar(&profileName, "profile", "", "only skills this profile selects")
	_ = cmd.RegisterFlagCompletionFunc("profile", completeProfiles)
	_ = cmd.RegisterFlagCompletionFunc("source", completeSources)
	_ = cmd.RegisterFlagCompletionFunc("output", cobra.FixedCompletions([]string{"text", "json"}, cobra.ShellCompDirectiveNoFileComp))
	return cmd
}

func buildListRows(in inputs, source, profileName string) ([]listRow, error) {
	var only *profile.Active
	if profileName != "" {
		if in.config == nil {
			return nil, errors.New("--profile needs a config file, and none was found")
		}
		a, err := in.config.Resolve(profile.Name(profileName))
		if err != nil {
			return nil, err
		}
		only = a
	}

	var rows []listRow
	for _, s := range in.manifest.Skills() {
		key := s.Key()
		if source != "" && key.Source() != source {
			continue
		}
		if only != nil && !only.Selects(key) {
			continue
		}
		rows = append(rows, listRow{
			Key:              key.String(),
			Path:             s.Path(),
			DescriptionBytes: s.DescriptionBytes(),
			EstimatedTokens:  s.EstimatedTokens(),
			Profiles:         profilesSelecting(in.config, key),
		})
	}
	return rows, nil
}

// profilesSelecting names every profile that selects key, each evaluated on
// its own so the list shows what a profile would link when used alone.
func profilesSelecting(cfg *profile.Config, key skill.Key) []string {
	if cfg == nil {
		return nil
	}
	var names []string
	for _, name := range cfg.Names() {
		a, err := cfg.Resolve(name)
		if err == nil && a.Selects(key) {
			names = append(names, string(name))
		}
	}
	return names
}

func writeRows(w io.Writer, rows []listRow, format string, withProfiles bool) error {
	switch format {
	case "json":
		if rows == nil {
			rows = []listRow{}
		}
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(rows)
	case "text":
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		header := "KEY\tTOKENS"
		if withProfiles {
			header += "\tPROFILES"
		}
		fmt.Fprintln(tw, header)
		for _, r := range rows {
			line := fmt.Sprintf("%s\t~%d", r.Key, r.EstimatedTokens)
			if withProfiles {
				profiles := "-"
				if len(r.Profiles) > 0 {
					profiles = strings.Join(r.Profiles, ",")
				}
				line += "\t" + profiles
			}
			fmt.Fprintln(tw, line)
		}
		return tw.Flush()
	}
	return fmt.Errorf("unknown output format %q: want text or json", format)
}
