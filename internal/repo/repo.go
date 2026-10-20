// Package repo answers git questions the sync command needs: where a project
// root is, which repository it belongs to, and how to keep links out of
// commits.
package repo

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	blockBegin = "# loadout begin"
	blockEnd   = "# loadout end"
)

type Repo struct {
	// Toplevel is the root of the current work tree. In a linked worktree it
	// is the worktree directory, which is where skills discovery stops.
	Toplevel string
	// Identity is "host/owner/repo", taken from the main work tree so every
	// linked worktree of a repository maps to the same entry in the config.
	Identity string
}

// Find locates the repository containing dir.
func Find(dir string) (Repo, error) {
	top, err := git(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return Repo{}, err
	}
	common, err := git(dir, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return Repo{}, err
	}
	return Repo{Toplevel: top, Identity: identity(filepath.Dir(common))}, nil
}

// identity keeps the last three path elements, which under a ghq root is
// host/owner/repo.
func identity(mainWorktree string) string {
	parts := strings.Split(filepath.ToSlash(mainWorktree), "/")
	if len(parts) > 3 {
		parts = parts[len(parts)-3:]
	}
	return strings.Join(parts, "/")
}

// UpdateExclude rewrites loadout's block in the repository's info/exclude file
// so linked skills never show up in git status. Passing no lines removes the
// block.
func (r Repo) UpdateExclude(lines []string) error {
	path, err := git(r.Toplevel, "rev-parse", "--path-format=absolute", "--git-path", "info/exclude")
	if err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("read %s: %w", path, err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(rewriteBlock(string(data), lines)), 0o644)
}

// rewriteBlock replaces loadout's delimited block in content, leaving every
// other line untouched.
func rewriteBlock(content string, lines []string) string {
	var kept []string
	skipping := false
	for _, line := range strings.Split(content, "\n") {
		switch {
		case line == blockBegin:
			skipping = true
		case skipping && line == blockEnd:
			skipping = false
		case !skipping:
			kept = append(kept, line)
		}
	}
	out := strings.TrimRight(strings.Join(kept, "\n"), "\n")
	if len(lines) > 0 {
		if out != "" {
			out += "\n"
		}
		out += blockBegin + "\n" + strings.Join(lines, "\n") + "\n" + blockEnd
	}
	if out == "" {
		return ""
	}
	return out + "\n"
}

func git(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), strings.TrimSpace(string(exitErr.Stderr)))
		}
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(out)), nil
}
