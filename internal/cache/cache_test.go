package cache

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Uncool-dudes/loadout/internal/skill"
)

func testSkill(t *testing.T, name string) skill.Skill {
	t.Helper()
	dir := filepath.Join(t.TempDir(), name)
	if err := os.MkdirAll(filepath.Join(dir, "references"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: "+name+"\n---\nbody\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "extra.txt"), []byte("extra"), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := `{"version":1,"skills":[{"key":"src/` + name + `","path":"` + dir + `","descriptionBytes":4}]}`
	m, err := skill.ParseManifest([]byte(manifest))
	if err != nil {
		t.Fatal(err)
	}
	key, _ := skill.ParseKey("src/" + name)
	s, _ := m.Lookup(key)
	return s
}

func TestPath(t *testing.T) {
	c := New("/cache")
	a := testSkill(t, "semgrep")
	b := testSkill(t, "semgrep") // same name, different store path

	if got := c.Path(a, skill.User); filepath.Base(got) != "semgrep" || !strings.HasPrefix(got, "/cache/") {
		t.Errorf("Path = %s", got)
	}
	if c.Path(a, skill.User) != c.Path(a, skill.User) {
		t.Error("Path is not stable")
	}
	if c.Path(a, skill.User) == c.Path(a, skill.Model) {
		t.Error("different invocations share a path")
	}
	if c.Path(a, skill.User) == c.Path(b, skill.User) {
		t.Error("different store paths share a path")
	}
}

func TestEnsure(t *testing.T) {
	c := New(filepath.Join(t.TempDir(), "cache"))
	s := testSkill(t, "semgrep")

	if err := c.Ensure(s, skill.User); err != nil {
		t.Fatal(err)
	}
	dest := c.Path(s, skill.User)

	data, err := os.ReadFile(filepath.Join(dest, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "disable-model-invocation: true") {
		t.Errorf("SKILL.md not patched:\n%s", data)
	}

	// Everything else is a link into the original skill, never a copy.
	for _, name := range []string{"references", "extra.txt"} {
		got, err := os.Readlink(filepath.Join(dest, name))
		if err != nil || got != filepath.Join(s.Path(), name) {
			t.Errorf("%s -> %q, %v", name, got, err)
		}
	}
	if original, _ := os.ReadFile(filepath.Join(s.Path(), "SKILL.md")); strings.Contains(string(original), "disable-model") {
		t.Error("the original SKILL.md was modified")
	}

	if err := c.Ensure(s, skill.User); err != nil {
		t.Errorf("second Ensure: %v", err)
	}
	leftovers, _ := filepath.Glob(filepath.Join(c.root, ".tmp-*"))
	if len(leftovers) != 0 {
		t.Errorf("temporary directories left behind: %v", leftovers)
	}
}

func TestEnsureFailsWithoutFrontmatter(t *testing.T) {
	c := New(filepath.Join(t.TempDir(), "cache"))
	s := testSkill(t, "bare")
	if err := os.WriteFile(filepath.Join(s.Path(), "SKILL.md"), []byte("no header\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := c.Ensure(s, skill.Model); err == nil {
		t.Fatal("want an error")
	}
	if _, err := os.Stat(c.Path(s, skill.Model)); !os.IsNotExist(err) {
		t.Errorf("a failed build left a variant behind: %v", err)
	}
}

func TestStaleAndRemove(t *testing.T) {
	c := New(filepath.Join(t.TempDir(), "cache"))
	current := testSkill(t, "current")
	gone := testSkill(t, "gone")
	for _, s := range []skill.Skill{current, gone} {
		if err := c.Ensure(s, skill.User); err != nil {
			t.Fatal(err)
		}
	}
	oldTmp := filepath.Join(c.root, ".tmp-old")
	freshTmp := filepath.Join(c.root, ".tmp-fresh")
	for _, d := range []string{oldTmp, freshTmp} {
		if err := os.Mkdir(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	past := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(oldTmp, past, past); err != nil {
		t.Fatal(err)
	}

	stale, err := c.Stale([]string{c.Path(current, skill.User), c.Path(current, skill.Model)})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{filepath.Dir(c.Path(gone, skill.User)): true, oldTmp: true}
	if len(stale) != len(want) {
		t.Fatalf("stale = %v, want %v", stale, want)
	}
	for _, p := range stale {
		if !want[p] {
			t.Errorf("unexpected stale entry %s", p)
		}
	}

	if err := c.Remove(stale); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{c.Path(current, skill.User), freshTmp} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("%s was removed: %v", p, err)
		}
	}
	if _, err := os.Stat(c.Path(gone, skill.User)); !os.IsNotExist(err) {
		t.Errorf("stale variant survived: %v", err)
	}
}

func TestRemoveRefusesOutsideRoot(t *testing.T) {
	c := New(filepath.Join(t.TempDir(), "cache"))
	victim := t.TempDir()
	if err := c.Remove([]string{victim}); err == nil {
		t.Fatal("Remove accepted a path outside the cache")
	}
	if _, err := os.Stat(victim); err != nil {
		t.Errorf("victim directory was touched: %v", err)
	}
}
