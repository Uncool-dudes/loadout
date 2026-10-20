// Package plan is the pure core of sync: given the skills a profile selects
// and the links already present, it returns what to add, replace, remove, or
// report as a conflict. It does no I/O, so it is tested without a filesystem.
package plan

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/Uncool-dudes/loadout/internal/skill"
)

type Op string

const (
	Add     Op = "add"
	Replace Op = "replace"
	Remove  Op = "remove"
	Keep    Op = "keep"
)

// Kind is what currently sits at a name inside a skills directory.
type Kind int

const (
	Symlink Kind = iota + 1
	Directory
	File
)

// Desired is a skill that a profile selected, with the path to link to.
type Desired struct {
	Key    skill.Key
	Target string
}

// Existing is an entry already present in the skills directory.
type Existing struct {
	Name   string
	Kind   Kind
	Target string // set for symlinks
}

type Input struct {
	Desired  []Desired
	Existing []Existing
	// Managed holds the link names a previous sync created. Only these may be
	// replaced or removed, so skills a project owns are never touched.
	Managed []string
	// Shadowed holds names already provided by a higher-precedence location.
	// Claude Code prefers personal skills over project ones, so a project link
	// with the same name would silently never take effect.
	Shadowed []string
}

type Action struct {
	Op     Op
	Name   string
	Target string
}

type Conflict struct {
	Name   string
	Reason string
}

type Plan struct {
	Actions   []Action
	Conflicts []Conflict
	// Managed is the link set after the plan is applied, sorted.
	Managed []string
}

// HasChanges reports whether applying the plan would touch the filesystem.
func (p Plan) HasChanges() bool {
	return slices.ContainsFunc(p.Actions, func(a Action) bool { return a.Op != Keep })
}

func Compute(in Input) Plan {
	existing := make(map[string]Existing, len(in.Existing))
	for _, e := range in.Existing {
		existing[e.Name] = e
	}
	managed := set(in.Managed)
	shadowed := set(in.Shadowed)

	byName := map[string][]Desired{}
	for _, d := range in.Desired {
		byName[d.Key.Name()] = append(byName[d.Key.Name()], d)
	}

	var p Plan
	for _, name := range slices.Sorted(maps.Keys(byName)) {
		ds := byName[name]
		conflict := func(reason string) {
			p.Conflicts = append(p.Conflicts, Conflict{Name: name, Reason: reason})
		}
		if len(ds) > 1 {
			keys := make([]string, len(ds))
			for i, d := range ds {
				keys[i] = d.Key.String()
			}
			conflict("name clash between " + strings.Join(keys, " and "))
			continue
		}
		d := ds[0]
		if _, ok := shadowed[name]; ok {
			conflict("a personal skill with this name takes precedence over a project skill")
			continue
		}

		ex, found := existing[name]
		switch {
		case !found:
			p.Actions = append(p.Actions, Action{Op: Add, Name: name, Target: d.Target})
		case ex.Kind != Symlink:
			conflict("a real file or directory already exists here")
			continue
		case !has(managed, name):
			conflict("a symlink that loadout did not create already exists here")
			continue
		case ex.Target == d.Target:
			p.Actions = append(p.Actions, Action{Op: Keep, Name: name, Target: d.Target})
		default:
			p.Actions = append(p.Actions, Action{Op: Replace, Name: name, Target: d.Target})
		}
		p.Managed = append(p.Managed, name)
	}

	for _, name := range slices.Sorted(maps.Keys(managed)) {
		if _, wanted := byName[name]; wanted {
			continue
		}
		if ex, ok := existing[name]; ok && ex.Kind == Symlink {
			p.Actions = append(p.Actions, Action{Op: Remove, Name: name, Target: ex.Target})
		}
	}
	slices.SortFunc(p.Actions, func(a, b Action) int { return strings.Compare(a.Name, b.Name) })
	return p
}

// String renders a conflict for error output.
func (c Conflict) String() string { return fmt.Sprintf("%s: %s", c.Name, c.Reason) }

func set(in []string) map[string]struct{} {
	out := make(map[string]struct{}, len(in))
	for _, s := range in {
		out[s] = struct{}{}
	}
	return out
}

func has(s map[string]struct{}, k string) bool {
	_, ok := s[k]
	return ok
}
