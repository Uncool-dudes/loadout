package skill

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
)

const manifestVersion = 1

// Skill is one entry of the manifest: where it lives in the Nix store and how
// much context its description costs.
type Skill struct {
	key              Key
	path             string
	descriptionBytes int
}

func (s Skill) Key() Key              { return s.key }
func (s Skill) Path() string          { return s.path }
func (s Skill) DescriptionBytes() int { return s.descriptionBytes }

// EstimatedTokens uses the common four-bytes-per-token rule of thumb, so it is
// a budget indicator, not an exact count.
func (s Skill) EstimatedTokens() int { return (s.descriptionBytes + 3) / 4 }

// Manifest is the Nix-built catalogue of every skill available to link.
type Manifest struct {
	skills []Skill // sorted by key
	byKey  map[Key]Skill
}

type rawManifest struct {
	Version int        `json:"version"`
	Skills  []rawSkill `json:"skills"`
}

type rawSkill struct {
	Key              string `json:"key"`
	Path             string `json:"path"`
	DescriptionBytes int    `json:"descriptionBytes"`
}

// ParseManifest decodes manifest.json strictly. Machines write this file, so
// anything unexpected is a bug worth failing on.
func ParseManifest(data []byte) (*Manifest, error) {
	var raw rawManifest
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&raw); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	if raw.Version != manifestVersion {
		return nil, fmt.Errorf("parse manifest: version %d, want %d", raw.Version, manifestVersion)
	}

	m := &Manifest{byKey: make(map[Key]Skill, len(raw.Skills))}
	for _, rs := range raw.Skills {
		key, err := ParseKey(rs.Key)
		if err != nil {
			return nil, fmt.Errorf("parse manifest: %w", err)
		}
		if !filepath.IsAbs(rs.Path) {
			return nil, fmt.Errorf("parse manifest: %s: path %q is not absolute", key, rs.Path)
		}
		if rs.DescriptionBytes < 0 {
			return nil, fmt.Errorf("parse manifest: %s: negative descriptionBytes", key)
		}
		if _, dup := m.byKey[key]; dup {
			return nil, fmt.Errorf("parse manifest: duplicate skill %s", key)
		}
		s := Skill{key: key, path: rs.Path, descriptionBytes: rs.DescriptionBytes}
		m.byKey[key] = s
		m.skills = append(m.skills, s)
	}
	slices.SortFunc(m.skills, func(a, b Skill) int { return strings.Compare(a.key.String(), b.key.String()) })
	return m, nil
}

// Skills returns every skill, sorted by key.
func (m *Manifest) Skills() []Skill { return slices.Clone(m.skills) }

func (m *Manifest) Lookup(k Key) (Skill, bool) {
	s, ok := m.byKey[k]
	return s, ok
}
