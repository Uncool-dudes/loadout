package link

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Uncool-dudes/loadout/internal/plan"
	"github.com/Uncool-dudes/loadout/internal/skill"
)

func key(t *testing.T, s string) skill.Key {
	t.Helper()
	k, err := skill.ParseKey(s)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

// sync runs the same scan, plan, apply sequence the sync command does.
func sync(t *testing.T, dir string, desired ...plan.Desired) plan.Plan {
	t.Helper()
	st, err := Scan(dir)
	if err != nil {
		t.Fatal(err)
	}
	p := plan.Compute(plan.Input{Desired: desired, Existing: st.Existing, Managed: st.Managed})
	if len(p.Conflicts) > 0 {
		t.Fatalf("unexpected conflicts: %v", p.Conflicts)
	}
	if err := Apply(dir, p); err != nil {
		t.Fatal(err)
	}
	return p
}

func target(t *testing.T, path string) string {
	t.Helper()
	got, err := os.Readlink(path)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestScanMissingDir(t *testing.T) {
	st, err := Scan(filepath.Join(t.TempDir(), "nope"))
	if err != nil || len(st.Existing) != 0 || len(st.Managed) != 0 {
		t.Fatalf("Scan = %+v, %v; want empty", st, err)
	}
}

func TestSyncLifecycle(t *testing.T) {
	dir := filepath.Join(t.TempDir(), ".claude", "skills")
	semgrep := plan.Desired{Key: key(t, "t/semgrep"), Target: "/store/a"}
	otel := plan.Desired{Key: key(t, "o/otel"), Target: "/store/b"}

	sync(t, dir, semgrep, otel)
	if got := target(t, filepath.Join(dir, "semgrep")); got != "/store/a" {
		t.Errorf("semgrep -> %s", got)
	}

	// Second run with the same input changes nothing.
	if p := sync(t, dir, semgrep, otel); p.HasChanges() {
		t.Errorf("second sync has changes: %+v", p.Actions)
	}

	// A store update retargets the link.
	semgrep.Target = "/store/a2"
	sync(t, dir, semgrep, otel)
	if got := target(t, filepath.Join(dir, "semgrep")); got != "/store/a2" {
		t.Errorf("semgrep after update -> %s", got)
	}

	// Dropping a skill from the profile removes its link only.
	sync(t, dir, otel)
	if _, err := os.Lstat(filepath.Join(dir, "semgrep")); !os.IsNotExist(err) {
		t.Errorf("semgrep link still present: %v", err)
	}
	if got := target(t, filepath.Join(dir, "otel")); got != "/store/b" {
		t.Errorf("otel -> %s", got)
	}

	// Emptying the profile removes the marker too.
	sync(t, dir)
	if _, err := os.Stat(filepath.Join(dir, MarkerName)); !os.IsNotExist(err) {
		t.Errorf("marker still present: %v", err)
	}
}

func TestProjectOwnedSkillsSurvive(t *testing.T) {
	dir := filepath.Join(t.TempDir(), ".claude", "skills")
	own := filepath.Join(dir, "mine")
	if err := os.MkdirAll(own, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/elsewhere", filepath.Join(dir, "linked-by-hand")); err != nil {
		t.Fatal(err)
	}

	sync(t, dir, plan.Desired{Key: key(t, "t/semgrep"), Target: "/store/a"})
	sync(t, dir)

	if _, err := os.Stat(own); err != nil {
		t.Errorf("project-owned directory removed: %v", err)
	}
	if got := target(t, filepath.Join(dir, "linked-by-hand")); got != "/elsewhere" {
		t.Errorf("hand-made symlink changed: %s", got)
	}
}

func TestMarkerIsSorted(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "skills")
	sync(
		t, dir,
		plan.Desired{Key: key(t, "t/zeta"), Target: "/s/z"},
		plan.Desired{Key: key(t, "t/alpha"), Target: "/s/a"},
	)
	st, err := Scan(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(st.Managed, []string{"alpha", "zeta"}) {
		t.Errorf("managed = %v", st.Managed)
	}
}

func TestNames(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"a", "b", ".hidden"} {
		if err := os.Mkdir(filepath.Join(dir, n), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	got, err := Names(dir)
	if err != nil || !slices.Equal(got, []string{"a", "b"}) {
		t.Fatalf("Names = %v, %v", got, err)
	}
	if got, err := Names(filepath.Join(dir, "missing")); err != nil || got != nil {
		t.Fatalf("Names(missing) = %v, %v", got, err)
	}
}
