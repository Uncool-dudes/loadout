package profile

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/Uncool-dudes/loadout/internal/skill"
)

// Active is the set of profiles in effect for a directory. Each root profile
// is evaluated on its own, then combined, so one profile's exclude never
// removes a skill that a sibling profile selected.
type Active struct {
	cfg   *Config
	roots []Name
}

func (c *Config) Resolve(roots ...Name) (*Active, error) {
	for _, r := range roots {
		if _, ok := c.profiles[r]; !ok {
			return nil, fmt.Errorf("%w %q", ErrUnknownProfile, r)
		}
	}
	return &Active{cfg: c, roots: slices.Clone(roots)}, nil
}

// chain lists a profile followed by everything it extends, nearest first,
// each once.
func (c *Config) chain(root Name) []profile {
	var out []profile
	seen := map[Name]bool{}
	var walk func(Name)
	walk = func(n Name) {
		if seen[n] {
			return
		}
		seen[n] = true
		p := c.profiles[n]
		out = append(out, p)
		for _, e := range p.extends {
			walk(e)
		}
	}
	walk(root)
	return out
}

// Selects reports whether any active root profile includes k and none of that
// root's own chain excludes it.
func (a *Active) Selects(k skill.Key) bool {
	for _, root := range a.roots {
		var included, excluded bool
		for _, p := range a.cfg.chain(root) {
			included = included || slices.ContainsFunc(p.skills, func(s skill.Selector) bool { return s.Matches(k) })
			excluded = excluded || slices.ContainsFunc(p.exclude, func(s skill.Selector) bool { return s.Matches(k) })
		}
		if included && !excluded {
			return true
		}
	}
	return false
}

// InvocationFor resolves how k may be invoked. A profile beats the top-level
// default and a child beats the profile it extends; within one profile
// skill.Rules prefers an exact key over a wildcard. Roots that still disagree
// are an error, because picking one would silently change behaviour.
func (a *Active) InvocationFor(k skill.Key) (skill.Invocation, error) {
	found := map[skill.Invocation][]Name{}
	for _, root := range a.roots {
		for _, p := range a.cfg.chain(root) {
			if inv, ok := p.invocation.For(k); ok {
				found[inv] = append(found[inv], root)
				break
			}
		}
	}
	switch len(found) {
	case 0:
		inv, _ := a.cfg.invocation.For(k)
		return inv, nil
	case 1:
		for inv := range found {
			return inv, nil
		}
	}

	var parts []string
	for _, inv := range slices.Sorted(maps.Keys(found)) {
		names := make([]string, len(found[inv]))
		for i, n := range found[inv] {
			names[i] = string(n)
		}
		parts = append(parts, fmt.Sprintf("%s (%s)", inv, strings.Join(names, ", ")))
	}
	return skill.Both, fmt.Errorf("%w for %s: %s", ErrInvocationConflict, k, strings.Join(parts, " vs "))
}

// Unmatched lists skill selectors in the active profiles that match nothing in
// m: a typo, or a skill that left its source. It checks "skills" only, since an
// exclude that matches nothing is harmless.
func (a *Active) Unmatched(m *skill.Manifest) []skill.Selector {
	seen := map[skill.Selector]bool{}
	var out []skill.Selector
	for _, root := range a.roots {
		for _, p := range a.cfg.chain(root) {
			for _, sel := range p.skills {
				if seen[sel] {
					continue
				}
				seen[sel] = true
				if !slices.ContainsFunc(m.Skills(), func(s skill.Skill) bool { return sel.Matches(s.Key()) }) {
					out = append(out, sel)
				}
			}
		}
	}
	slices.SortFunc(out, func(x, y skill.Selector) int { return strings.Compare(x.String(), y.String()) })
	return out
}
