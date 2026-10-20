package skill

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeSkill(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func real(t *testing.T, p string) string {
	t.Helper()
	r, err := filepath.EvalSymlinks(p)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestScanLibrary(t *testing.T) {
	lib := t.TempDir()

	// A source that is a real directory with nested and ignored skills.
	one := filepath.Join(lib, "one")
	writeSkill(t, filepath.Join(one, "plugins", "p", "skills", "alpha"), "---\nname: alpha\ndescription: first skill\n---\nbody\n")
	writeSkill(t, filepath.Join(one, "beta"), "---\nname: beta\ndescription: second\n---\n")
	writeSkill(t, filepath.Join(one, "tests", "fixture"), "---\nname: fixture\n---\n")
	writeSkill(t, filepath.Join(one, "node_modules", "dep"), "---\nname: dep\n---\n")
	writeSkill(t, filepath.Join(one, "zz", "beta"), "---\nname: beta\ndescription: duplicate\n---\n")
	writeSkill(t, filepath.Join(one, "broken"), "no header here\n")

	// A source that is a symlink, with a SKILL.md at its top.
	elsewhere := t.TempDir()
	writeSkill(t, elsewhere, "---\nname: rooty\ndescription: top level\n---\n")
	if err := os.Symlink(elsewhere, filepath.Join(lib, "two")); err != nil {
		t.Fatal(err)
	}

	// Things that must not become sources.
	if err := os.Mkdir(filepath.Join(lib, ".hidden"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lib, "a-file"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	m, warnings, err := ScanLibrary(lib)
	if err != nil {
		t.Fatal(err)
	}

	var keys []string
	for _, s := range m.Skills() {
		keys = append(keys, s.Key().String())
	}
	want := "one/alpha,one/beta,one/broken,two/rooty"
	if got := strings.Join(keys, ","); got != want {
		t.Fatalf("keys = %s, want %s", got, want)
	}

	key, _ := ParseKey("one/alpha")
	alpha, _ := m.Lookup(key)
	if wantPath := real(t, filepath.Join(one, "plugins", "p", "skills", "alpha")); alpha.Path() != wantPath {
		t.Errorf("alpha path = %s, want %s", alpha.Path(), wantPath)
	}
	if got := alpha.DescriptionBytes(); got != len("alpha")+len("first skill") {
		t.Errorf("alpha descriptionBytes = %d", got)
	}

	key, _ = ParseKey("two/rooty")
	rooty, _ := m.Lookup(key)
	if rooty.Path() != real(t, elsewhere) {
		t.Errorf("rooty path = %s, want the resolved symlink target", rooty.Path())
	}

	joined := strings.Join(warnings, "\n")
	for _, want := range []string{"duplicate skill one/beta", "no YAML frontmatter"} {
		if !strings.Contains(joined, want) {
			t.Errorf("warnings missing %q:\n%s", want, joined)
		}
	}
}

func TestScanLibraryMissingRoot(t *testing.T) {
	if _, _, err := ScanLibrary(filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Fatal("want an error")
	}
}
