package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Uncool-dudes/loadout/internal/repo"
)

// syncEnv is a throwaway git repository, personal Claude directory, manifest
// and config. Tests run inside the repository.
type syncEnv struct {
	repo, claude, manifest, config string
}

func newSyncEnv(t *testing.T, configJSON string) syncEnv {
	t.Helper()
	dir := t.TempDir()
	e := syncEnv{
		repo:     filepath.Join(dir, "github.com", "owner", "proj"),
		claude:   filepath.Join(dir, "claude"),
		manifest: filepath.Join(dir, "manifest.json"),
		config:   filepath.Join(dir, "loadout.hjson"),
	}
	if err := os.MkdirAll(e.repo, 0o755); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "-C", e.repo, "init", "-q", "-b", "main").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	if configJSON == "" {
		configJSON = testConfig
	}
	for path, content := range map[string]string{e.manifest: testManifest, e.config: configJSON} {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("CLAUDE_CONFIG_DIR", e.claude)
	t.Setenv("LOADOUT_PROFILES", "")
	t.Chdir(e.repo)
	return e
}

func (e syncEnv) sync(t *testing.T, args ...string) (string, error) {
	t.Helper()
	all := append([]string{"sync", "--manifest", e.manifest, "--config", e.config}, args...)
	return run(t, all...)
}

func (e syncEnv) linked(t *testing.T) []string {
	t.Helper()
	var names []string
	entries, err := os.ReadDir(filepath.Join(e.repo, ".claude", "skills"))
	if err != nil {
		return nil
	}
	for _, en := range entries {
		if !strings.HasPrefix(en.Name(), ".") {
			names = append(names, en.Name())
		}
	}
	return names
}

func TestSyncLinksAndExcludes(t *testing.T) {
	e := newSyncEnv(t, "")
	out, err := e.sync(t, "--profile", "observability")
	if err != nil {
		t.Fatal(err)
	}
	if got := e.linked(t); strings.Join(got, ",") != "otel-collector,sentry-fix-issues" {
		t.Errorf("linked = %v", got)
	}
	if !strings.Contains(out, "2 added, 0 replaced, 0 removed, 0 unchanged") {
		t.Errorf("summary wrong:\n%s", out)
	}

	status, err := exec.Command("git", "-C", e.repo, "status", "--porcelain").Output()
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(status)) != "" {
		t.Errorf("links show in git status:\n%s", status)
	}

	out, err = e.sync(t, "--profile", "observability")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "0 added, 0 replaced, 0 removed, 2 unchanged") {
		t.Errorf("second run should change nothing:\n%s", out)
	}
}

func TestSyncSwitchingProfileRemovesOldLinks(t *testing.T) {
	e := newSyncEnv(t, "")
	if _, err := e.sync(t, "--profile", "observability"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.sync(t, "--profile", "security"); err != nil {
		t.Fatal(err)
	}
	if got := e.linked(t); strings.Join(got, ",") != "semgrep" {
		t.Errorf("linked = %v, want [semgrep]", got)
	}
}

func TestSyncDryRunChangesNothing(t *testing.T) {
	e := newSyncEnv(t, "")
	out, err := e.sync(t, "--profile", "security", "--dry-run")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "dry run: 1 added") {
		t.Errorf("output:\n%s", out)
	}
	if got := e.linked(t); len(got) != 0 {
		t.Errorf("dry run linked %v", got)
	}
}

func TestSyncConflicts(t *testing.T) {
	t.Run("real directory in the way", func(t *testing.T) {
		e := newSyncEnv(t, "")
		if err := os.MkdirAll(filepath.Join(e.repo, ".claude", "skills", "semgrep"), 0o755); err != nil {
			t.Fatal(err)
		}
		_, err := e.sync(t, "--profile", "security")
		if err == nil || !strings.Contains(err.Error(), "semgrep") {
			t.Fatalf("error = %v, want a semgrep conflict", err)
		}
	})
	t.Run("personal skill with the same name", func(t *testing.T) {
		e := newSyncEnv(t, "")
		personal := filepath.Join(e.claude, "skills", "semgrep")
		if err := os.MkdirAll(personal, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(personal, "SKILL.md"), []byte("---\nname: semgrep\n---\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := e.sync(t, "--profile", "security")
		if err == nil || !strings.Contains(err.Error(), "personal skill") {
			t.Fatalf("error = %v, want a personal-skill conflict", err)
		}
		if got := e.linked(t); len(got) != 0 {
			t.Errorf("a conflicting sync still linked %v", got)
		}
	})
}

func TestSyncProfileSelection(t *testing.T) {
	t.Run("no profile selected", func(t *testing.T) {
		e := newSyncEnv(t, "")
		_, err := e.sync(t)
		if err == nil || !strings.Contains(err.Error(), "no profile selected") {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("environment variable", func(t *testing.T) {
		e := newSyncEnv(t, "")
		t.Setenv("LOADOUT_PROFILES", "security")
		if _, err := e.sync(t); err != nil {
			t.Fatal(err)
		}
		if got := e.linked(t); strings.Join(got, ",") != "semgrep" {
			t.Errorf("linked = %v", got)
		}
	})
	t.Run("projects entry matched by repo identity", func(t *testing.T) {
		e := newSyncEnv(t, "")
		r, err := repo.Find(e.repo)
		if err != nil {
			t.Fatal(err)
		}
		cfg := fmt.Sprintf(`{"profiles":{"security":{"skills":["trailofbits/*"]}},"projects":{%q:["security"]}}`, r.Identity)
		if err := os.WriteFile(e.config, []byte(cfg), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := e.sync(t); err != nil {
			t.Fatal(err)
		}
		if got := e.linked(t); strings.Join(got, ",") != "semgrep" {
			t.Errorf("linked = %v", got)
		}
	})
}

func TestSyncGlobal(t *testing.T) {
	cfg := `{"global":["security"],"profiles":{"security":{"skills":["trailofbits/*"]}}}`
	e := newSyncEnv(t, cfg)
	if _, err := e.sync(t, "--global"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(e.claude, "skills", "semgrep")); err != nil {
		t.Errorf("global link missing: %v", err)
	}
	if got := e.linked(t); len(got) != 0 {
		t.Errorf("a global sync linked into the project: %v", got)
	}
}

// useRealSkill replaces the fixture manifest with one whose skill is a real
// directory, because applying an invocation rule reads its SKILL.md.
func useRealSkill(t *testing.T, e syncEnv) (skillDir, cacheDir string) {
	t.Helper()
	skillDir = filepath.Join(t.TempDir(), "semgrep")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: semgrep\n---\nbody\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := fmt.Sprintf(`{"version":1,"skills":[{"key":"trailofbits/semgrep","path":%q,"descriptionBytes":10}]}`, skillDir)
	if err := os.WriteFile(e.manifest, []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	cacheDir = t.TempDir()
	t.Setenv("LOADOUT_CACHE", cacheDir)
	return skillDir, cacheDir
}

const userOnlyConfig = `{"invocation":{"trailofbits/*":"user"},"profiles":{"security":{"skills":["trailofbits/*"]}}}`

func TestSyncAppliesInvocation(t *testing.T) {
	e := newSyncEnv(t, userOnlyConfig)
	skillDir, cacheDir := useRealSkill(t, e)

	if _, err := e.sync(t, "--profile", "security"); err != nil {
		t.Fatal(err)
	}

	linkPath := filepath.Join(e.repo, ".claude", "skills", "semgrep")
	target, err := os.Readlink(linkPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(target, cacheDir) {
		t.Errorf("link target %s is not in the cache %s", target, cacheDir)
	}
	patched, err := os.ReadFile(filepath.Join(linkPath, "SKILL.md"))
	if err != nil || !strings.Contains(string(patched), "disable-model-invocation: true") {
		t.Errorf("SKILL.md through the link = %q, %v", patched, err)
	}
	original, _ := os.ReadFile(filepath.Join(skillDir, "SKILL.md"))
	if strings.Contains(string(original), "disable-model") {
		t.Error("the original skill was modified")
	}
}

func TestSyncDryRunWritesNoCache(t *testing.T) {
	e := newSyncEnv(t, userOnlyConfig)
	_, cacheDir := useRealSkill(t, e)

	if _, err := e.sync(t, "--profile", "security", "--dry-run"); err != nil {
		t.Fatal(err)
	}
	if entries, _ := os.ReadDir(cacheDir); len(entries) != 0 {
		t.Errorf("dry run wrote to the cache: %v", entries)
	}
}

func TestSyncInvocationConflict(t *testing.T) {
	cfg := `{"profiles":{
  "a":{"skills":["trailofbits/*"],"invocation":{"trailofbits/*":"user"}},
  "b":{"skills":["trailofbits/*"],"invocation":{"trailofbits/*":"model"}}
}}`
	e := newSyncEnv(t, cfg)
	useRealSkill(t, e)

	_, err := e.sync(t, "--profile", "a", "--profile", "b")
	if err == nil || !strings.Contains(err.Error(), "conflicting invocation") {
		t.Fatalf("error = %v, want an invocation conflict", err)
	}
	if got := e.linked(t); len(got) != 0 {
		t.Errorf("a conflicting sync linked %v", got)
	}
}

func TestSyncWarnsAboutUnmatchedSelector(t *testing.T) {
	cfg := `{"profiles":{"security":{"skills":["trailofbits/*","trailofbits/typo"]}}}`
	e := newSyncEnv(t, cfg)
	var errBuf strings.Builder
	cmd := NewRootCmd()
	cmd.SetOut(&strings.Builder{})
	cmd.SetErr(&errBuf)
	cmd.SetArgs([]string{"sync", "--manifest", e.manifest, "--config", e.config, "--profile", "security"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(errBuf.String(), "trailofbits/typo matches no skill") {
		t.Errorf("stderr = %q", errBuf.String())
	}
}

func TestSyncIgnoresPersonalDirectoriesWithoutASkill(t *testing.T) {
	e := newSyncEnv(t, "")
	husk := filepath.Join(e.claude, "skills", "semgrep")
	if err := os.MkdirAll(husk, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(husk, "SKILL.md.bak"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := e.sync(t, "--profile", "security"); err != nil {
		t.Fatalf("a leftover directory blocked the sync: %v", err)
	}
}
