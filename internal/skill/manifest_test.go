package skill

import "testing"

const manifestFixture = `{
  "version": 1,
  "skills": [
    {"key": "trailofbits/semgrep", "path": "/nix/store/a-semgrep", "descriptionBytes": 401},
    {"key": "getsentry/sentry-fix-issues", "path": "/nix/store/b-fix", "descriptionBytes": 100}
  ]
}`

func TestParseManifest(t *testing.T) {
	m, err := ParseManifest([]byte(manifestFixture))
	if err != nil {
		t.Fatal(err)
	}
	skills := m.Skills()
	if len(skills) != 2 || skills[0].Key().String() != "getsentry/sentry-fix-issues" {
		t.Fatalf("skills not sorted by key: %v", skills)
	}
	if got := skills[1].EstimatedTokens(); got != 101 {
		t.Errorf("EstimatedTokens = %d, want 101", got)
	}
	key, _ := ParseKey("trailofbits/semgrep")
	if s, ok := m.Lookup(key); !ok || s.Path() != "/nix/store/a-semgrep" {
		t.Errorf("Lookup = %v, %v", s, ok)
	}
}

func TestParseManifestRejects(t *testing.T) {
	tests := map[string]string{
		"wrong version":  `{"version": 2, "skills": []}`,
		"unknown field":  `{"version": 1, "skills": [], "extra": true}`,
		"bad key":        `{"version": 1, "skills": [{"key": "nosource", "path": "/p", "descriptionBytes": 1}]}`,
		"relative path":  `{"version": 1, "skills": [{"key": "a/b", "path": "p", "descriptionBytes": 1}]}`,
		"negative bytes": `{"version": 1, "skills": [{"key": "a/b", "path": "/p", "descriptionBytes": -1}]}`,
		"duplicate key":  `{"version": 1, "skills": [{"key": "a/b", "path": "/p", "descriptionBytes": 1}, {"key": "a/b", "path": "/q", "descriptionBytes": 1}]}`,
		"not json":       `version = 1`,
	}
	for name, src := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseManifest([]byte(src)); err == nil {
				t.Fatal("ParseManifest succeeded, want error")
			}
		})
	}
}
