package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestGCRemovesOnlyUnusableEntries(t *testing.T) {
	e := newSyncEnv(t, userOnlyConfig)
	skillDir, cacheDir := useRealSkill(t, e)
	launchDir := t.TempDir()
	t.Setenv("LOADOUT_LAUNCH", launchDir)

	if _, err := e.sync(t, "--profile", "security"); err != nil {
		t.Fatal(err)
	}
	live, err := os.ReadDir(cacheDir)
	if err != nil || len(live) != 1 {
		t.Fatalf("cache after sync = %v, %v", live, err)
	}

	// A variant for a store path that left the library.
	orphan := filepath.Join(cacheDir, "deadbeefdeadbeef", "gone")
	if err := os.MkdirAll(orphan, 0o755); err != nil {
		t.Fatal(err)
	}

	// Launch sets: one abandoned, one recent.
	oldSet := filepath.Join(launchDir, "oldset")
	newSet := filepath.Join(launchDir, "newset")
	for _, d := range []string{oldSet, newSet} {
		if err := os.Mkdir(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	past := time.Now().Add(-90 * 24 * time.Hour)
	if err := os.Chtimes(oldSet, past, past); err != nil {
		t.Fatal(err)
	}

	gc := func(args ...string) string {
		out, err := run(t, append([]string{"gc", "--manifest", e.manifest}, args...)...)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}

	out := gc("--dry-run")
	if !strings.Contains(out, "would remove 1 patched variant(s) and 1 launch file set(s)") {
		t.Errorf("dry run summary:\n%s", out)
	}
	if _, err := os.Stat(orphan); err != nil {
		t.Fatalf("dry run removed the orphan: %v", err)
	}

	gc()
	if _, err := os.Stat(filepath.Dir(orphan)); !os.IsNotExist(err) {
		t.Errorf("orphan variant survived: %v", err)
	}
	if _, err := os.Stat(oldSet); !os.IsNotExist(err) {
		t.Errorf("abandoned launch set survived: %v", err)
	}
	if _, err := os.Stat(newSet); err != nil {
		t.Errorf("recent launch set was removed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cacheDir, live[0].Name())); err != nil {
		t.Errorf("the variant a link points to was removed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(skillDir, "SKILL.md")); err != nil {
		t.Errorf("the library skill was touched: %v", err)
	}
	linkTarget, err := os.Readlink(filepath.Join(e.repo, ".claude", "skills", "semgrep"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(linkTarget); err != nil {
		t.Errorf("project link now dangles: %v", err)
	}
}

func TestCompletion(t *testing.T) {
	e := newSyncEnv(t, launchConfig)

	complete := func(args ...string) string {
		out, err := run(t, append([]string{"__complete"}, args...)...)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}

	got := complete("sync", "--config", e.config, "--profile", "")
	for _, want := range []string{"security", "plain", "auto"} {
		if !strings.Contains(got, want+"\n") {
			t.Errorf("profile completion missing %q:\n%s", want, got)
		}
	}
	if got := complete("sync", "--config", e.config, "--profile", "se"); strings.Contains(got, "plain") || !strings.Contains(got, "security") {
		t.Errorf("prefix filter wrong:\n%s", got)
	}

	sources := complete("list", "--manifest", e.manifest, "--source", "")
	for _, want := range []string{"getsentry", "ollygarden", "trailofbits"} {
		if !strings.Contains(sources, want+"\n") {
			t.Errorf("source completion missing %q:\n%s", want, sources)
		}
	}

	if got := complete("sync", "--config", filepath.Join(t.TempDir(), "missing.hjson"), "--profile", ""); strings.Contains(got, "security") {
		t.Errorf("completed profiles from a missing config:\n%s", got)
	}
}
