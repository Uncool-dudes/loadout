package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/Uncool-dudes/loadout/internal/cache"
	"github.com/Uncool-dudes/loadout/internal/link"
	"github.com/Uncool-dudes/loadout/internal/plan"
	"github.com/Uncool-dudes/loadout/internal/skill"
)

type statusRow struct {
	Name            string `json:"name"`
	Key             string `json:"key,omitempty"`
	Invocation      string `json:"invocation,omitempty"`
	EstimatedTokens int    `json:"estimatedTokens"`
	// State is ok, dangling (the target is gone), stale (the target is not in
	// the manifest), or project-owned (loadout did not create it).
	State string `json:"state"`
}

type pending struct {
	Added     int    `json:"added"`
	Replaced  int    `json:"replaced"`
	Removed   int    `json:"removed"`
	Conflicts int    `json:"conflicts"`
	Note      string `json:"note,omitempty"`
}

type statusReport struct {
	Dir         string      `json:"dir"`
	Skills      []statusRow `json:"skills"`
	TotalTokens int         `json:"totalTokens"`
	Pending     *pending    `json:"pending,omitempty"`
}

func newStatusCmd() *cobra.Command {
	var profiles []string
	var global bool
	var output string
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show linked skills, their invocation, and description token cost",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			in, err := loadInputs(cmd)
			if err != nil {
				return err
			}
			target, err := resolveTarget(global)
			if err != nil {
				return err
			}
			store := cache.New(defaultCachePath())
			state, err := link.Scan(target.skillsDir)
			if err != nil {
				return err
			}

			report := statusReport{Dir: target.skillsDir}
			report.Skills, report.TotalTokens = statusRows(state, indexTargets(in.manifest, store), target.skillsDir)
			if in.config != nil {
				report.Pending = pendingChanges(in, target, profiles, store)
			}
			return writeStatus(cmd.OutOrStdout(), report, output)
		},
	}
	cmd.Flags().StringSliceVar(&profiles, "profile", nil, "profile to compare against (default: LOADOUT_PROFILES, then the projects entry)")
	_ = cmd.RegisterFlagCompletionFunc("profile", completeProfiles)
	cmd.Flags().BoolVar(&global, "global", false, "report on ~/.claude/skills")
	cmd.Flags().StringVarP(&output, "output", "o", "text", "output format: text or json")
	_ = cmd.RegisterFlagCompletionFunc("output", cobra.FixedCompletions([]string{"text", "json"}, cobra.ShellCompDirectiveNoFileComp))
	return cmd
}

type resolvedLink struct {
	skill      skill.Skill
	invocation skill.Invocation
}

// indexTargets maps every path loadout could have linked to the skill and
// invocation behind it, so a link on disk can be traced back to the manifest.
func indexTargets(m *skill.Manifest, store cache.Cache) map[string]resolvedLink {
	idx := map[string]resolvedLink{}
	for _, s := range m.Skills() {
		idx[s.Path()] = resolvedLink{s, skill.Both}
		for _, inv := range []skill.Invocation{skill.User, skill.Model} {
			idx[store.Path(s, inv)] = resolvedLink{s, inv}
		}
	}
	return idx
}

func statusRows(state link.State, idx map[string]resolvedLink, dir string) ([]statusRow, int) {
	managed := map[string]bool{}
	for _, n := range state.Managed {
		managed[n] = true
	}

	var rows []statusRow
	total := 0
	for _, ex := range state.Existing {
		row := statusRow{Name: ex.Name, State: "project-owned"}
		if managed[ex.Name] && ex.Kind == plan.Symlink {
			if r, ok := idx[ex.Target]; ok {
				row.Key = r.skill.Key().String()
				row.Invocation = r.invocation.String()
				row.EstimatedTokens = r.skill.EstimatedTokens()
				total += row.EstimatedTokens
				row.State = "ok"
				if _, err := os.Stat(filepath.Join(dir, ex.Name)); err != nil {
					row.State = "dangling"
				}
			} else {
				row.State = "stale"
			}
		}
		rows = append(rows, row)
	}
	return rows, total
}

// pendingChanges compares disk with what the chosen profiles want. Failing to
// work that out is not a status failure, so the reason goes in the report.
func pendingChanges(in inputs, target syncTarget, flagNames []string, store cache.Cache) *pending {
	names, err := selectProfiles(in.config, flagNames, target)
	if err != nil {
		return &pending{Note: err.Error()}
	}
	p, _, err := buildPlan(in, target, names, store)
	if err != nil {
		return &pending{Note: err.Error()}
	}
	out := &pending{Conflicts: len(p.Conflicts)}
	for _, a := range p.Actions {
		switch a.Op {
		case plan.Add:
			out.Added++
		case plan.Replace:
			out.Replaced++
		case plan.Remove:
			out.Removed++
		}
	}
	return out
}

func writeStatus(w io.Writer, r statusReport, format string) error {
	switch format {
	case "json":
		if r.Skills == nil {
			r.Skills = []statusRow{}
		}
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(r)
	case "text":
		fmt.Fprintf(w, "%s\n", r.Dir)
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "NAME\tKEY\tINVOCATION\tTOKENS\tSTATE")
		for _, row := range r.Skills {
			key, inv, tokens := "-", "-", "-"
			if row.Key != "" {
				key, inv, tokens = row.Key, row.Invocation, fmt.Sprintf("~%d", row.EstimatedTokens)
			}
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", row.Name, key, inv, tokens, row.State)
		}
		if err := tw.Flush(); err != nil {
			return err
		}
		fmt.Fprintf(w, "total: ~%d description tokens\n", r.TotalTokens)
		if p := r.Pending; p != nil {
			fmt.Fprintln(w, describePending(*p))
		}
		return nil
	}
	return fmt.Errorf("unknown output format %q: want text or json", format)
}

func describePending(p pending) string {
	switch {
	case p.Note != "":
		return "pending: unknown (" + p.Note + ")"
	case p.Conflicts > 0:
		return fmt.Sprintf("pending: %d to add, %d to replace, %d to remove, %d conflict(s); run `loadout sync --dry-run`",
			p.Added, p.Replaced, p.Removed, p.Conflicts)
	case p.Added+p.Replaced+p.Removed == 0:
		return "pending: up to date"
	}
	return fmt.Sprintf("pending: %d to add, %d to replace, %d to remove; run `loadout sync`", p.Added, p.Replaced, p.Removed)
}
