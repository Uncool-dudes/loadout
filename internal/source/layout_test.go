package source

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func mkSkill(t *testing.T, root, rel string) {
	t.Helper()
	dir := filepath.Join(root, rel)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	name := filepath.Base(dir)
	if rel == "." {
		name = "rooty"
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: "+name+"\ndescription: d\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDetect(t *testing.T) {
	tests := []struct {
		name        string
		skills      []string
		wantPath    string
		wantInclude []string
		wantSkills  string
		wantErr     string
	}{
		{name: "nested plugins", skills: []string{"plugins/a/skills/x", "plugins/b/skills/y"}, wantPath: "plugins", wantSkills: "x,y"},
		{name: "flat skills directory", skills: []string{"skills/foo", "skills/bar"}, wantPath: "skills", wantSkills: "bar,foo"},
		{name: "a single skill", skills: []string{"skills/foo"}, wantPath: "skills", wantSkills: "foo"},
		{name: "skills at the top", skills: []string{"foo", "bar"}, wantPath: "", wantSkills: "bar,foo"},
		{name: "root SKILL.md only", skills: []string{"."}, wantInclude: []string{"SKILL.md"}, wantSkills: "rooty"},
		{name: "fixtures are ignored", skills: []string{"skills/foo", "tests/fixture"}, wantPath: "skills", wantSkills: "foo"},
		{name: "root and subdirectory", skills: []string{".", "skills/foo"}, wantErr: "both at the repository root"},
		{name: "no skills", skills: nil, wantErr: "no SKILL.md"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			for _, s := range tt.skills {
				mkSkill(t, root, s)
			}
			got, err := Detect(root)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.Path != tt.wantPath || !slices.Equal(got.Include, tt.wantInclude) {
				t.Errorf("layout = path %q include %v, want path %q include %v", got.Path, got.Include, tt.wantPath, tt.wantInclude)
			}
			names := slices.Clone(got.Skills)
			slices.Sort(names)
			if strings.Join(names, ",") != tt.wantSkills {
				t.Errorf("skills = %v, want %s", names, tt.wantSkills)
			}
		})
	}
}

func TestDescribe(t *testing.T) {
	l := Layout{Path: "plugins", Skills: []string{"b", "a"}}
	if got := l.Describe(); got != "2 skill(s) under plugins/: a, b" {
		t.Errorf("Describe = %q", got)
	}
	many := Layout{Skills: []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "10"}}
	if got := many.Describe(); !strings.Contains(got, "and 2 more") {
		t.Errorf("Describe = %q", got)
	}
}
