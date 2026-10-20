// Package link is the imperative shell for a skills directory: it reads what
// is there, applies a plan, and remembers which links it created.
package link

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Uncool-dudes/loadout/internal/plan"
)

// MarkerName is the file that records which links loadout created. A marker is
// used instead of guessing from symlink targets because store paths change on
// every update, and deleting a link another tool made would be worse than
// leaving a stale one.
const MarkerName = ".loadout.json"

type marker struct {
	Managed []string `json:"managed"`
}

// State is what a skills directory holds right now.
type State struct {
	Existing []plan.Existing
	Managed  []string
}

// Scan reads dir. A missing directory is an empty state, not an error.
func Scan(dir string) (State, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return State{}, nil
	}
	if err != nil {
		return State{}, fmt.Errorf("scan %s: %w", dir, err)
	}

	var st State
	for _, e := range entries {
		if e.Name() == MarkerName {
			continue
		}
		ex := plan.Existing{Name: e.Name()}
		switch mode := e.Type(); {
		case mode&fs.ModeSymlink != 0:
			ex.Kind = plan.Symlink
			if ex.Target, err = os.Readlink(filepath.Join(dir, e.Name())); err != nil {
				return State{}, fmt.Errorf("scan %s: %w", dir, err)
			}
		case mode.IsDir():
			ex.Kind = plan.Directory
		default:
			ex.Kind = plan.File
		}
		st.Existing = append(st.Existing, ex)
	}

	st.Managed, err = readMarker(dir)
	return st, err
}

// Names lists the visible entries of dir, for detecting name collisions with
// another skills directory. A missing directory yields none.
func Names(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("list %s: %w", dir, err)
	}
	var out []string
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), ".") {
			out = append(out, e.Name())
		}
	}
	return out, nil
}

// Apply performs the plan's actions and rewrites the marker. It does not
// judge conflicts: refusing to run a plan that has them is the caller's call.
func Apply(dir string, p plan.Plan) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}
	for _, a := range p.Actions {
		path := filepath.Join(dir, a.Name)
		var err error
		switch a.Op {
		case plan.Add:
			err = os.Symlink(a.Target, path)
		case plan.Replace:
			err = replaceSymlink(path, a.Target)
		case plan.Remove:
			err = removeSymlink(path)
		case plan.Keep:
		}
		if err != nil {
			return fmt.Errorf("%s %s: %w", a.Op, a.Name, err)
		}
	}
	return writeMarker(dir, p.Managed)
}

// replaceSymlink swaps the target atomically, so a reader never sees the link
// missing.
func replaceSymlink(path, target string) error {
	tmp := path + ".loadout-tmp"
	_ = os.Remove(tmp)
	if err := os.Symlink(target, tmp); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// removeSymlink refuses to delete anything that is not a symlink, as a last
// guard behind the plan's own checks.
func removeSymlink(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&fs.ModeSymlink == 0 {
		return fmt.Errorf("%s is not a symlink", path)
	}
	return os.Remove(path)
}

func readMarker(dir string) ([]string, error) {
	data, err := os.ReadFile(filepath.Join(dir, MarkerName))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read marker: %w", err)
	}
	var m marker
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse %s: %w", filepath.Join(dir, MarkerName), err)
	}
	return m.Managed, nil
}

func writeMarker(dir string, managed []string) error {
	path := filepath.Join(dir, MarkerName)
	if len(managed) == 0 {
		if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("remove marker: %w", err)
		}
		return nil
	}
	data, err := json.MarshalIndent(marker{Managed: slices.Sorted(slices.Values(managed))}, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("write marker: %w", err)
	}
	return os.Rename(tmp, path)
}
