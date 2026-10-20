// Package cache materializes patched copies of skills. A copy is a directory
// of symlinks into the original skill, except SKILL.md, which is rewritten.
package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Uncool-dudes/loadout/internal/patch"
	"github.com/Uncool-dudes/loadout/internal/skill"
)

// version is part of every cache key. Bump it when patch.Invocation changes,
// so stale copies are no longer reused.
const version = "1"

type Cache struct {
	root string
}

func New(root string) Cache { return Cache{root: root} }

// Path is where the variant of s with invocation inv lives. It does no I/O, so
// a dry run can show the plan without writing anything. The last path element
// is the skill name, because Claude Code names a skill after its directory.
func (c Cache) Path(s skill.Skill, inv skill.Invocation) string {
	sum := sha256.Sum256([]byte(version + "\x00" + s.Path() + "\x00" + inv.String()))
	return filepath.Join(c.root, hex.EncodeToString(sum[:])[:16], s.Key().Name())
}

// Ensure builds the variant if it is not there yet. It builds in a temporary
// directory and renames it into place, so an interrupted run never leaves a
// half-written variant behind.
func (c Cache) Ensure(s skill.Skill, inv skill.Invocation) error {
	dest := c.Path(s, inv)
	if _, err := os.Stat(dest); err == nil {
		return nil
	}

	entries, err := os.ReadDir(s.Path())
	if err != nil {
		return fmt.Errorf("read skill %s: %w", s.Key(), err)
	}
	if err := os.MkdirAll(c.root, 0o755); err != nil {
		return err
	}
	tmp, err := os.MkdirTemp(c.root, ".tmp-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)

	skillDir := filepath.Join(tmp, s.Key().Name())
	if err := os.Mkdir(skillDir, 0o755); err != nil {
		return err
	}
	for _, e := range entries {
		src := filepath.Join(s.Path(), e.Name())
		if e.Name() != "SKILL.md" {
			if err := os.Symlink(src, filepath.Join(skillDir, e.Name())); err != nil {
				return err
			}
			continue
		}
		data, err := os.ReadFile(src)
		if err != nil {
			return fmt.Errorf("read %s: %w", src, err)
		}
		patched, err := patch.Invocation(data, inv)
		if err != nil {
			return fmt.Errorf("%s: %w", s.Key(), err)
		}
		if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), patched, 0o644); err != nil {
			return err
		}
	}

	if err := os.Chmod(tmp, 0o755); err != nil {
		return err
	}
	if err := os.Rename(tmp, filepath.Dir(dest)); err != nil {
		// A concurrent sync may have built the same variant first.
		if _, statErr := os.Stat(dest); statErr == nil {
			return nil
		}
		return err
	}
	return nil
}

// tmpGrace keeps Stale from touching a build another sync is still running.
const tmpGrace = time.Hour

// Stale lists cache entries that no path in keep lives under: variants of
// skills that left the library or moved to a new store path, and builds that
// were interrupted long ago. keep holds values returned by Path.
func (c Cache) Stale(keep []string) ([]string, error) {
	live := map[string]bool{}
	for _, p := range keep {
		live[filepath.Dir(p)] = true
	}
	entries, err := os.ReadDir(c.root)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var stale []string
	for _, e := range entries {
		full := filepath.Join(c.root, e.Name())
		if live[full] {
			continue
		}
		if strings.HasPrefix(e.Name(), ".tmp-") {
			info, err := e.Info()
			if err != nil || time.Since(info.ModTime()) < tmpGrace {
				continue
			}
		}
		stale = append(stale, full)
	}
	return stale, nil
}

// Remove deletes entries returned by Stale. It refuses anything outside the
// cache root, so a bad path can never delete something else.
func (c Cache) Remove(paths []string) error {
	for _, p := range paths {
		if filepath.Dir(p) != c.root {
			return fmt.Errorf("refusing to remove %s: not directly under %s", p, c.root)
		}
		if err := os.RemoveAll(p); err != nil {
			return err
		}
	}
	return nil
}
