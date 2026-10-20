package source

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Uncool-dudes/loadout/internal/skill"
)

// Layout says where inside a repository the skills live.
type Layout struct {
	Path    string
	Include []string
	// Skills holds the skill names found, for showing the user what they get.
	Skills   []string
	Warnings []string
}

// Detect finds the directory to scan by looking at where the skills are. The
// scan root must be the parent of the skill directories, because a skill
// named after its directory only works when its SKILL.md is below the root.
func Detect(root string) (Layout, error) {
	lib, err := os.MkdirTemp("", "loadout-detect-")
	if err != nil {
		return Layout{}, err
	}
	defer os.RemoveAll(lib)
	if err := os.Symlink(root, filepath.Join(lib, "probe")); err != nil {
		return Layout{}, err
	}
	m, warnings, err := skill.ScanLibrary(lib)
	if err != nil {
		return Layout{}, err
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return Layout{}, err
	}

	layout := Layout{Warnings: warnings}
	var parents []string
	atRoot := false
	for _, s := range m.Skills() {
		layout.Skills = append(layout.Skills, s.Key().Name())
		rel, err := filepath.Rel(realRoot, s.Path())
		if err != nil {
			return Layout{}, err
		}
		if rel == "." {
			atRoot = true
			continue
		}
		parents = append(parents, filepath.ToSlash(filepath.Dir(rel)))
	}

	switch {
	case len(layout.Skills) == 0:
		return Layout{}, errors.New("no SKILL.md found in the repository")
	case atRoot && len(parents) == 0:
		layout.Include = []string{"SKILL.md"}
	case atRoot:
		return Layout{}, errors.New("skills are both at the repository root and in subdirectories: pass --path or --include")
	default:
		layout.Path = commonDir(parents)
	}
	return layout, nil
}

// commonDir returns the longest directory that every path is inside, with "."
// meaning the repository root becoming the empty string.
func commonDir(dirs []string) string {
	common := strings.Split(dirs[0], "/")
	for _, d := range dirs[1:] {
		parts := strings.Split(d, "/")
		n := 0
		for n < len(common) && n < len(parts) && common[n] == parts[n] {
			n++
		}
		common = common[:n]
	}
	joined := path.Join(common...)
	if joined == "." {
		return ""
	}
	return joined
}

// Describe summarises a layout for the add command's output.
func (l Layout) Describe() string {
	where := "the repository root"
	switch {
	case len(l.Include) > 0:
		where = "include " + strings.Join(l.Include, ", ")
	case l.Path != "":
		where = l.Path + "/"
	}
	names := slices.Clone(l.Skills)
	slices.Sort(names)
	shown := names
	if len(shown) > 8 {
		shown = append(shown[:8:8], fmt.Sprintf("and %d more", len(names)-8))
	}
	return fmt.Sprintf("%d skill(s) under %s: %s", len(names), where, strings.Join(shown, ", "))
}
