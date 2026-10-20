// Package source is the pinned list of skill sources: which GitHub repository,
// at which revision, and where inside it the skills live. It is plain data in
// sources.json, so Nix reads it and `loadout add` edits it without touching Nix
// code.
package source

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"path"
	"regexp"
	"slices"
	"strings"

	"github.com/Uncool-dudes/loadout/internal/ident"
)

var (
	ErrExists   = errors.New("source already exists")
	ErrNotFound = errors.New("no such source")

	revPattern  = regexp.MustCompile(`^[0-9a-f]{40}$`)
	repoPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
	refPattern  = regexp.MustCompile(`^[A-Za-z0-9._/-]+$`)
)

// Source pins one GitHub repository.
type Source struct {
	Owner string `json:"owner"`
	Repo  string `json:"repo"`
	// Ref is the branch or tag `loadout update` follows; empty means the
	// default branch.
	Ref  string `json:"ref,omitempty"`
	Rev  string `json:"rev"`
	Hash string `json:"hash"`
	// Path is the directory inside the repository to scan for skills.
	Path string `json:"path,omitempty"`
	// Include lists files or directories to copy into the source, each under
	// its base name. It is for what a plain Path cannot express, such as a
	// SKILL.md that sits at the repository root. It excludes Path.
	Include []string `json:"include,omitempty"`
	// Library false fetches the repository without offering its skills, for
	// plugins that are only wanted for their hooks or commands.
	Library *bool `json:"library,omitempty"`
	// Pinned keeps `loadout update` from moving the revision.
	Pinned bool `json:"pinned,omitempty"`
}

func (s Source) InLibrary() bool { return s.Library == nil || *s.Library }

// Validate reports the first thing wrong with s. Names and paths end up in
// Nix strings and shell commands, so they are held to plain characters.
func (s Source) Validate() error {
	if !repoPattern.MatchString(s.Owner) || !repoPattern.MatchString(s.Repo) {
		return fmt.Errorf("invalid owner/repo %q/%q", s.Owner, s.Repo)
	}
	if s.Ref != "" && !refPattern.MatchString(s.Ref) {
		return fmt.Errorf("invalid ref %q", s.Ref)
	}
	if !revPattern.MatchString(s.Rev) {
		return fmt.Errorf("rev %q is not a 40-character commit hash", s.Rev)
	}
	if !strings.HasPrefix(s.Hash, "sha256-") {
		return fmt.Errorf("hash %q is not an SRI sha256 hash", s.Hash)
	}
	if s.Path != "" && len(s.Include) > 0 {
		return errors.New("path and include are mutually exclusive")
	}
	for _, p := range append([]string{s.Path}, s.Include...) {
		if err := validatePath(p); err != nil {
			return err
		}
	}
	return nil
}

func validatePath(p string) error {
	if p == "" {
		return nil
	}
	if path.IsAbs(p) || path.Clean(p) != p {
		return fmt.Errorf("path %q must be relative and clean", p)
	}
	for _, part := range strings.Split(p, "/") {
		if part == ".." || strings.ContainsAny(part, "\"'`$\\ \t\n;&|<>*?") {
			return fmt.Errorf("path %q has an unsafe element %q", p, part)
		}
	}
	return nil
}

// Set is the whole sources.json: sources by name.
type Set struct {
	sources map[string]Source
}

// Parse decodes sources.json strictly and validates every entry.
func Parse(data []byte) (Set, error) {
	raw := map[string]Source{}
	if len(bytes.TrimSpace(data)) > 0 {
		dec := json.NewDecoder(bytes.NewReader(data))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&raw); err != nil {
			return Set{}, fmt.Errorf("parse sources: %w", err)
		}
	}
	for _, name := range slices.Sorted(maps.Keys(raw)) {
		if err := validateName(name); err != nil {
			return Set{}, err
		}
		if err := raw[name].Validate(); err != nil {
			return Set{}, fmt.Errorf("source %s: %w", name, err)
		}
	}
	return Set{sources: raw}, nil
}

func validateName(name string) error {
	if !ident.Valid(name) || strings.ContainsAny(name, "/*.") {
		return fmt.Errorf("invalid source name %q", name)
	}
	return nil
}

// Marshal renders the set with sorted keys and two-space indentation, so edits
// by hand and by tool produce small, stable diffs.
func (s Set) Marshal() ([]byte, error) {
	m := s.sources
	if m == nil {
		m = map[string]Source{}
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	if err := enc.Encode(m); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (s Set) Get(name string) (Source, bool) {
	src, ok := s.sources[name]
	return src, ok
}

func (s Set) Names() []string { return slices.Sorted(maps.Keys(s.sources)) }

// Add inserts a new source.
func (s *Set) Add(name string, src Source) error {
	if err := validateName(name); err != nil {
		return err
	}
	if err := src.Validate(); err != nil {
		return err
	}
	if _, ok := s.sources[name]; ok {
		return fmt.Errorf("%w: %s", ErrExists, name)
	}
	if s.sources == nil {
		s.sources = map[string]Source{}
	}
	s.sources[name] = src
	return nil
}

// Replace overwrites an existing source, for updates.
func (s *Set) Replace(name string, src Source) error {
	if _, ok := s.sources[name]; !ok {
		return fmt.Errorf("%w: %s", ErrNotFound, name)
	}
	if err := src.Validate(); err != nil {
		return err
	}
	s.sources[name] = src
	return nil
}

func (s *Set) Remove(name string) error {
	if _, ok := s.sources[name]; !ok {
		return fmt.Errorf("%w: %s", ErrNotFound, name)
	}
	delete(s.sources, name)
	return nil
}
