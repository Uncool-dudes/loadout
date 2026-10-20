package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Uncool-dudes/loadout/internal/cache"
	"github.com/Uncool-dudes/loadout/internal/link"
	"github.com/Uncool-dudes/loadout/internal/plan"
	"github.com/Uncool-dudes/loadout/internal/profile"
	"github.com/Uncool-dudes/loadout/internal/repo"
	"github.com/Uncool-dudes/loadout/internal/skill"
)

// syncTarget is the skills directory a sync writes to, plus what surrounds it.
type syncTarget struct {
	skillsDir string
	// shadowDir is the personal skills directory whose names beat project
	// skills; empty when syncing that directory itself.
	shadowDir string
	// repo is nil for a global sync.
	repo *repo.Repo
}

func newSyncCmd() *cobra.Command {
	var profiles []string
	var dryRun, global bool
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Link this directory's profile skills into .claude/skills/",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			in, err := loadInputs(cmd)
			if err != nil {
				return err
			}
			if in.config == nil {
				return errors.New("sync needs a config file, and none was found")
			}

			target, err := resolveTarget(global)
			if err != nil {
				return err
			}
			names, err := selectProfiles(in.config, profiles, target)
			if err != nil {
				return err
			}
			return syncSkills(cmd, in, target, names, dryRun, true)
		},
	}
	cmd.Flags().StringSliceVar(&profiles, "profile", nil, "profile to apply, repeatable (default: LOADOUT_PROFILES, then the projects entry)")
	_ = cmd.RegisterFlagCompletionFunc("profile", completeProfiles)
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show the plan without changing anything")
	cmd.Flags().BoolVar(&global, "global", false, "sync the global profiles into ~/.claude/skills")
	return cmd
}

// syncSkills links the skills the named profiles select. noteLaunch says
// whether to point out settings and MCP servers that only `loadout exec`
// applies; exec passes false because it applies them itself.
func syncSkills(cmd *cobra.Command, in inputs, target syncTarget, names []profile.Name, dryRun, noteLaunch bool) error {
	store := cache.New(defaultCachePath())
	p, patched, err := buildPlan(in, target, names, store)
	if err != nil {
		return err
	}

	active, err := in.config.Resolve(names...)
	if err != nil {
		return err
	}
	for _, sel := range active.Unmatched(in.manifest) {
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: selector %s matches no skill in the library\n", sel)
	}

	if noteLaunch {
		if l, err := active.Launch(); err == nil && !l.IsEmpty() {
			fmt.Fprintln(cmd.ErrOrStderr(), "note: these profiles also define settings or MCP servers; only `loadout exec` applies them")
		}
	}

	printPlan(cmd.OutOrStdout(), p, dryRun)
	if len(p.Conflicts) > 0 {
		return conflictError(p.Conflicts)
	}
	if dryRun {
		return nil
	}
	for _, v := range patched {
		if err := store.Ensure(v.skill, v.invocation); err != nil {
			return err
		}
	}
	if err := link.Apply(target.skillsDir, p); err != nil {
		return err
	}
	if target.repo != nil {
		return target.repo.UpdateExclude(excludeLines(p.Managed))
	}
	return nil
}

// buildPlan works out what linking the named profiles would change. It reads
// the skills directories but writes nothing, so sync and status share it.
func buildPlan(in inputs, target syncTarget, names []profile.Name, store cache.Cache) (plan.Plan, []variant, error) {
	active, err := in.config.Resolve(names...)
	if err != nil {
		return plan.Plan{}, nil, err
	}
	desired, patched, err := desiredFor(in.manifest, active, store)
	if err != nil {
		return plan.Plan{}, nil, err
	}
	state, err := link.Scan(target.skillsDir)
	if err != nil {
		return plan.Plan{}, nil, err
	}
	var shadowed []string
	if target.shadowDir != "" {
		if shadowed, err = link.Names(target.shadowDir); err != nil {
			return plan.Plan{}, nil, err
		}
	}
	return plan.Compute(plan.Input{
		Desired:  desired,
		Existing: state.Existing,
		Managed:  state.Managed,
		Shadowed: shadowed,
	}), patched, nil
}

func claudeDir() (string, error) {
	if v := os.Getenv("CLAUDE_CONFIG_DIR"); v != "" {
		return v, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".claude"), nil
}

func resolveTarget(global bool) (syncTarget, error) {
	claude, err := claudeDir()
	if err != nil {
		return syncTarget{}, err
	}
	personal := filepath.Join(claude, "skills")
	if global {
		return syncTarget{skillsDir: personal}, nil
	}

	cwd, err := os.Getwd()
	if err != nil {
		return syncTarget{}, err
	}
	r, err := repo.Find(cwd)
	if err != nil {
		return syncTarget{}, fmt.Errorf("sync links into a project, and %s is not inside a git repository (use --global for ~/.claude): %w", cwd, err)
	}
	return syncTarget{skillsDir: filepath.Join(r.Toplevel, ".claude", "skills"), shadowDir: personal, repo: &r}, nil
}

// selectProfiles picks profile names in order of how explicit they are: the
// flag, then the environment, then the config. A global sync falls back to the
// config's global list instead of a project entry.
func selectProfiles(cfg *profile.Config, flagNames []string, target syncTarget) ([]profile.Name, error) {
	if len(flagNames) > 0 {
		return parseProfileNames(flagNames)
	}
	if env := os.Getenv("LOADOUT_PROFILES"); env != "" {
		return parseProfileNames(strings.FieldsFunc(env, func(r rune) bool { return r == ',' || r == ' ' }))
	}
	if target.repo == nil {
		global := cfg.Global()
		if len(global) == 0 {
			return nil, errors.New("no global profiles configured: add a global list to the config or pass --profile")
		}
		return global, nil
	}
	names, ok, err := cfg.ProjectProfiles(target.repo.Identity)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("no profile selected for %s: pass --profile, set LOADOUT_PROFILES, or add a projects entry", target.repo.Identity)
	}
	return names, nil
}

func parseProfileNames(in []string) ([]profile.Name, error) {
	out := make([]profile.Name, 0, len(in))
	for _, s := range in {
		n, err := profile.ParseName(s)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, nil
}

// variant is a skill whose invocation rule needs a patched copy.
type variant struct {
	skill      skill.Skill
	invocation skill.Invocation
}

// desiredFor lists the manifest skills the active profiles select, with the
// path each should link to: the store path as is, or a cache path when an
// invocation rule applies. It writes nothing, so a dry run stays read-only;
// the returned variants are built later. Conflicting rules fail here.
func desiredFor(m *skill.Manifest, active *profile.Active, store cache.Cache) ([]plan.Desired, []variant, error) {
	var desired []plan.Desired
	var patched []variant
	for _, s := range m.Skills() {
		key := s.Key()
		if !active.Selects(key) {
			continue
		}
		inv, err := active.InvocationFor(key)
		if err != nil {
			return nil, nil, err
		}
		target := s.Path()
		if inv != skill.Both {
			target = store.Path(s, inv)
			patched = append(patched, variant{skill: s, invocation: inv})
		}
		desired = append(desired, plan.Desired{Key: key, Target: target})
	}
	return desired, patched, nil
}

func printPlan(w io.Writer, p plan.Plan, dryRun bool) {
	counts := map[plan.Op]int{}
	for _, a := range p.Actions {
		counts[a.Op]++
		if a.Op != plan.Keep {
			fmt.Fprintf(w, "%-8s %s\n", a.Op, a.Name)
		}
	}
	prefix := ""
	if dryRun {
		prefix = "dry run: "
	}
	fmt.Fprintf(w, "%s%d added, %d replaced, %d removed, %d unchanged\n",
		prefix, counts[plan.Add], counts[plan.Replace], counts[plan.Remove], counts[plan.Keep])
}

func conflictError(conflicts []plan.Conflict) error {
	lines := make([]string, len(conflicts))
	for i, c := range conflicts {
		lines[i] = "  " + c.String()
	}
	return fmt.Errorf("%d conflict(s), nothing was changed:\n%s", len(conflicts), strings.Join(lines, "\n"))
}

func excludeLines(managed []string) []string {
	if len(managed) == 0 {
		return nil
	}
	lines := make([]string, 0, len(managed)+1)
	for _, name := range managed {
		lines = append(lines, "/.claude/skills/"+name)
	}
	return append(lines, "/.claude/skills/"+link.MarkerName)
}
