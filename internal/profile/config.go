// Package profile parses loadout.hjson and resolves which profiles apply to a
// project. It is pure: callers read the file and hand over the bytes.
package profile

import (
	"errors"
	"fmt"
	"maps"
	"path"
	"slices"
	"strings"

	"github.com/hjson/hjson-go/v4"

	"github.com/Uncool-dudes/loadout/internal/ident"
	"github.com/Uncool-dudes/loadout/internal/skill"
)

var (
	ErrUnknownProfile     = errors.New("unknown profile")
	ErrCycle              = errors.New("extends cycle")
	ErrAmbiguousProject   = errors.New("ambiguous project match")
	ErrInvocationConflict = errors.New("conflicting invocation")
)

// Name identifies a profile.
type Name string

func ParseName(s string) (Name, error) {
	if !ident.Valid(s) || strings.ContainsAny(s, "/*") {
		return "", fmt.Errorf("invalid profile name %q", s)
	}
	return Name(s), nil
}

type profile struct {
	extends    []Name
	skills     []skill.Selector
	exclude    []skill.Selector
	invocation skill.Rules
	settings   map[string]any
	mcpServers map[string]any
	strictMCP  bool
}

type projectRule struct {
	pattern  string
	profiles []Name
}

// Config is a validated loadout.hjson. Every profile name it mentions exists
// and the extends graph has no cycles.
type Config struct {
	global     []Name
	invocation skill.Rules
	profiles   map[Name]profile
	projects   []projectRule
}

func (c *Config) Global() []Name { return slices.Clone(c.global) }

// Names lists every defined profile, sorted.
func (c *Config) Names() []Name { return slices.Sorted(maps.Keys(c.profiles)) }

type rawConfig struct {
	Global     []string              `json:"global"`
	Invocation map[string]string     `json:"invocation"`
	Profiles   map[string]rawProfile `json:"profiles"`
	Projects   map[string][]string   `json:"projects"`
}

type rawProfile struct {
	Extends    []string          `json:"extends"`
	Skills     []string          `json:"skills"`
	Exclude    []string          `json:"exclude"`
	Invocation map[string]string `json:"invocation"`
	Settings   map[string]any    `json:"settings"`
	MCPServers map[string]any    `json:"mcp_servers"`
	StrictMCP  bool              `json:"strict_mcp"`
}

// Parse decodes HJSON strictly: unknown keys are errors, so a typo such as
// "exlcude" fails loudly instead of being ignored.
func Parse(data []byte) (*Config, error) {
	var raw rawConfig
	err := hjson.UnmarshalWithOptions(data, &raw, hjson.DecoderOptions{DisallowUnknownFields: true})
	if err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	return build(raw)
}

func build(raw rawConfig) (*Config, error) {
	var err error
	c := &Config{profiles: map[Name]profile{}}

	if c.global, err = parseNames("global", raw.Global); err != nil {
		return nil, err
	}
	if c.invocation, err = parseRules("invocation", raw.Invocation); err != nil {
		return nil, err
	}
	for _, key := range slices.Sorted(maps.Keys(raw.Profiles)) {
		name, err := ParseName(key)
		if err != nil {
			return nil, err
		}
		p, err := buildProfile("profiles."+key, raw.Profiles[key])
		if err != nil {
			return nil, err
		}
		c.profiles[name] = p
	}
	for _, pattern := range slices.Sorted(maps.Keys(raw.Projects)) {
		if _, err := path.Match(pattern, ""); err != nil {
			return nil, fmt.Errorf("projects: invalid pattern %q: %w", pattern, err)
		}
		names, err := parseNames("projects."+pattern, raw.Projects[pattern])
		if err != nil {
			return nil, err
		}
		c.projects = append(c.projects, projectRule{pattern: pattern, profiles: names})
	}

	if err := c.checkReferences(); err != nil {
		return nil, err
	}
	if err := c.checkCycles(); err != nil {
		return nil, err
	}
	return c, nil
}

func buildProfile(field string, raw rawProfile) (profile, error) {
	var (
		p   profile
		err error
	)
	if p.extends, err = parseNames(field+".extends", raw.Extends); err != nil {
		return p, err
	}
	if p.skills, err = parseSelectors(field+".skills", raw.Skills); err != nil {
		return p, err
	}
	if p.exclude, err = parseSelectors(field+".exclude", raw.Exclude); err != nil {
		return p, err
	}
	if p.invocation, err = parseRules(field+".invocation", raw.Invocation); err != nil {
		return p, err
	}
	p.settings = raw.Settings
	p.mcpServers = raw.MCPServers
	p.strictMCP = raw.StrictMCP
	return p, nil
}

func parseNames(field string, in []string) ([]Name, error) {
	out := make([]Name, 0, len(in))
	for _, s := range in {
		n, err := ParseName(s)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", field, err)
		}
		out = append(out, n)
	}
	return out, nil
}

func parseSelectors(field string, in []string) ([]skill.Selector, error) {
	out := make([]skill.Selector, 0, len(in))
	for _, s := range in {
		sel, err := skill.ParseSelector(s)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", field, err)
		}
		out = append(out, sel)
	}
	return out, nil
}

func parseRules(field string, in map[string]string) (skill.Rules, error) {
	out := make(skill.Rules, len(in))
	for _, key := range slices.Sorted(maps.Keys(in)) {
		sel, err := skill.ParseSelector(key)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", field, err)
		}
		inv, err := skill.ParseInvocation(in[key])
		if err != nil {
			return nil, fmt.Errorf("%s.%s: %w", field, key, err)
		}
		out[sel] = inv
	}
	return out, nil
}

func (c *Config) checkReferences() error {
	known := func(where string, names []Name) error {
		for _, n := range names {
			if _, ok := c.profiles[n]; !ok {
				return fmt.Errorf("%s: %w %q", where, ErrUnknownProfile, n)
			}
		}
		return nil
	}
	if err := known("global", c.global); err != nil {
		return err
	}
	for _, name := range slices.Sorted(maps.Keys(c.profiles)) {
		if err := known("profiles."+string(name)+".extends", c.profiles[name].extends); err != nil {
			return err
		}
	}
	for _, rule := range c.projects {
		if err := known("projects."+rule.pattern, rule.profiles); err != nil {
			return err
		}
	}
	return nil
}

func (c *Config) checkCycles() error {
	const (
		visiting = iota + 1
		done
	)
	state := map[Name]int{}
	var visit func(n Name, trail []Name) error
	visit = func(n Name, trail []Name) error {
		switch state[n] {
		case done:
			return nil
		case visiting:
			cycle := append(slices.Clone(trail), n)
			parts := make([]string, len(cycle))
			for i, name := range cycle {
				parts[i] = string(name)
			}
			return fmt.Errorf("%w: %s", ErrCycle, strings.Join(parts, " -> "))
		}
		state[n] = visiting
		next := append(slices.Clone(trail), n)
		for _, e := range c.profiles[n].extends {
			if err := visit(e, next); err != nil {
				return err
			}
		}
		state[n] = done
		return nil
	}
	for _, n := range slices.Sorted(maps.Keys(c.profiles)) {
		if err := visit(n, nil); err != nil {
			return err
		}
	}
	return nil
}

// ProjectProfiles returns the profiles configured for a repo path such as
// "github.com/ratchio/gamma". An exact entry beats a wildcard; among wildcards
// the longest pattern wins, and equal-length matches are an error.
func (c *Config) ProjectProfiles(repo string) ([]Name, bool, error) {
	var matches []projectRule
	for _, rule := range c.projects {
		if rule.pattern == repo {
			return slices.Clone(rule.profiles), true, nil
		}
		if ok, _ := path.Match(rule.pattern, repo); ok {
			matches = append(matches, rule)
		}
	}
	if len(matches) == 0 {
		return nil, false, nil
	}
	slices.SortFunc(matches, func(a, b projectRule) int { return len(b.pattern) - len(a.pattern) })
	if len(matches) > 1 && len(matches[0].pattern) == len(matches[1].pattern) {
		return nil, false, fmt.Errorf("%w for %q: %q and %q", ErrAmbiguousProject, repo, matches[0].pattern, matches[1].pattern)
	}
	return slices.Clone(matches[0].profiles), true, nil
}
