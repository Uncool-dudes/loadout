package skill

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// skipDirs hold fixtures or tooling, never skills a profile should offer.
var skipDirs = map[string]bool{
	".git":         true,
	"node_modules": true,
	"tests":        true,
	"test":         true,
	"testdata":     true,
	"__tests__":    true,
	"fixtures":     true,
}

var errNoHeader = errors.New("no YAML frontmatter")

type header struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

// ScanLibrary indexes a library laid out as <root>/<source>/..., where each
// source is a directory or a symlink to one. Every directory holding a
// SKILL.md is a skill, named after that directory. A SKILL.md at the top of a
// source names the skill after its frontmatter instead, or after the source.
//
// A problem with one skill is a warning, not an error, so a single broken
// SKILL.md never hides the rest of the library.
func ScanLibrary(root string) (*Manifest, []string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, nil, fmt.Errorf("scan library: %w", err)
	}

	m := &Manifest{byKey: map[Key]Skill{}}
	var warnings []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		source := e.Name()
		if !plainPart(source) {
			warnings = append(warnings, fmt.Sprintf("skipping source %q: not a plain name", source))
			continue
		}
		// Resolve first so skill paths are the real store paths, which stay
		// stable no matter how the library is laid out.
		real, err := filepath.EvalSymlinks(filepath.Join(root, source))
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("skipping source %s: %v", source, err))
			continue
		}
		if info, err := os.Stat(real); err != nil || !info.IsDir() {
			continue
		}
		warnings = append(warnings, m.scanSource(source, real)...)
	}
	slices.SortFunc(m.skills, func(a, b Skill) int { return strings.Compare(a.key.String(), b.key.String()) })
	return m, warnings, nil
}

func (m *Manifest) scanSource(source, root string) []string {
	var warnings []string
	warn := func(format string, args ...any) { warnings = append(warnings, fmt.Sprintf(format, args...)) }

	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			warn("%s: %v", path, err)
			return nil
		}
		if d.IsDir() {
			if path != root && skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() != "SKILL.md" {
			return nil
		}

		dir := filepath.Dir(path)
		h, herr := readHeader(path)
		if herr != nil {
			warn("%s: %v", path, herr)
		}
		name := filepath.Base(dir)
		if dir == root {
			name = source
			if plainPart(h.Name) {
				name = h.Name
			}
		}
		key, err := ParseKey(source + "/" + name)
		if err != nil {
			warn("%s: %v", path, err)
			return nil
		}
		if kept, dup := m.byKey[key]; dup {
			warn("duplicate skill %s: keeping %s, ignoring %s", key, kept.path, dir)
			return nil
		}
		s := Skill{key: key, path: dir, descriptionBytes: len(name) + len(h.Description)}
		m.byKey[key] = s
		m.skills = append(m.skills, s)
		return nil
	})
	return warnings
}

func readHeader(path string) (header, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return header{}, err
	}
	lines := strings.Split(string(data), "\n")
	if strings.TrimSuffix(lines[0], "\r") != "---" {
		return header{}, errNoHeader
	}
	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSuffix(lines[i], "\r") == "---" {
			end = i
			break
		}
	}
	if end < 0 {
		return header{}, errNoHeader
	}
	var h header
	if err := yaml.Unmarshal([]byte(strings.Join(lines[1:end], "\n")), &h); err != nil {
		return header{}, fmt.Errorf("invalid frontmatter: %w", err)
	}
	return h, nil
}
