package repo

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(
		os.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func TestIdentity(t *testing.T) {
	tests := map[string]string{
		"/home/u/projects/github.com/ratchio/gamma": "github.com/ratchio/gamma",
		"/a/b": "/a/b",
	}
	for in, want := range tests {
		if got := identity(in); got != want {
			t.Errorf("identity(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRewriteBlock(t *testing.T) {
	tests := []struct {
		name    string
		content string
		lines   []string
		want    string
	}{
		{"empty file adds block", "", []string{"/a"}, "# loadout begin\n/a\n# loadout end\n"},
		{"keeps other lines", "node_modules\n", []string{"/a"}, "node_modules\n# loadout begin\n/a\n# loadout end\n"},
		{"replaces old block", "x\n# loadout begin\n/old\n# loadout end\ny\n", []string{"/new"}, "x\ny\n# loadout begin\n/new\n# loadout end\n"},
		{"no lines removes block", "x\n# loadout begin\n/old\n# loadout end\n", nil, "x\n"},
		{"nothing left is empty", "# loadout begin\n/old\n# loadout end\n", nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := rewriteBlock(tt.content, tt.lines); got != tt.want {
				t.Errorf("rewriteBlock = %q, want %q", got, tt.want)
			}
			// Running it twice must not change the result.
			if got := rewriteBlock(tt.want, tt.lines); got != tt.want {
				t.Errorf("not idempotent: %q", got)
			}
		})
	}
}

func TestFindAndExclude(t *testing.T) {
	main := filepath.Join(t.TempDir(), "github.com", "owner", "proj")
	if err := os.MkdirAll(main, 0o755); err != nil {
		t.Fatal(err)
	}
	gitIn(t, main, "init", "-q", "-b", "main")
	gitIn(t, main, "commit", "-q", "--allow-empty", "-m", "init")

	// A linked worktree must resolve to the main work tree's identity.
	wt := filepath.Join(t.TempDir(), "proj.worktrees", "feature")
	gitIn(t, main, "worktree", "add", "-q", "-b", "feature", wt)

	for _, dir := range []string{main, wt} {
		r, err := Find(dir)
		if err != nil {
			t.Fatal(err)
		}
		if r.Identity != "github.com/owner/proj" {
			t.Errorf("Find(%s).Identity = %q", dir, r.Identity)
		}
	}

	r, err := Find(wt)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.UpdateExclude([]string{"/.claude/skills/semgrep"}); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(wt, ".claude", "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/nowhere", filepath.Join(wt, ".claude", "skills", "semgrep")); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("git", "-C", wt, "status", "--porcelain").Output()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "semgrep") {
		t.Errorf("excluded link shows in git status:\n%s", out)
	}

	if err := r.UpdateExclude(nil); err != nil {
		t.Fatal(err)
	}
	out, _ = exec.Command("git", "-C", wt, "status", "--porcelain").Output()
	if !strings.Contains(string(out), ".claude") {
		t.Errorf("link should show again once the block is removed:\n%s", out)
	}
}
