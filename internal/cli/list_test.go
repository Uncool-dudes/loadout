package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testManifest = `{
  "version": 1,
  "skills": [
    {"key": "trailofbits/semgrep", "path": "/nix/store/a-semgrep", "descriptionBytes": 400},
    {"key": "getsentry/sentry-fix-issues", "path": "/nix/store/b-fix", "descriptionBytes": 100},
    {"key": "ollygarden/otel-collector", "path": "/nix/store/c-otel", "descriptionBytes": 40}
  ]
}`

const testConfig = `{"profiles":{
  "observability":{"skills":["ollygarden/*","getsentry/sentry-fix-issues"]},
  "security":{"skills":["trailofbits/*"]}
}}`

func writeFixtures(t *testing.T) (manifest, config string) {
	t.Helper()
	dir := t.TempDir()
	manifest = filepath.Join(dir, "manifest.json")
	config = filepath.Join(dir, "loadout.hjson")
	if err := os.WriteFile(manifest, []byte(testManifest), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(config, []byte(testConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	return manifest, config
}

func run(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	cmd := NewRootCmd()
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

func TestListText(t *testing.T) {
	manifest, config := writeFixtures(t)
	out, err := run(t, "list", "--manifest", manifest, "--config", config)
	if err != nil {
		t.Fatal(err)
	}
	// tabwriter pads columns to the widest cell, so compare on fields.
	lines := map[string]bool{}
	for _, line := range strings.Split(out, "\n") {
		lines[strings.Join(strings.Fields(line), " ")] = true
	}
	for _, want := range []string{
		"KEY TOKENS PROFILES",
		"getsentry/sentry-fix-issues ~25 observability",
		"ollygarden/otel-collector ~10 observability",
		"trailofbits/semgrep ~100 security",
	} {
		if !lines[want] {
			t.Errorf("output missing line %q:\n%s", want, out)
		}
	}
}

func TestListFilters(t *testing.T) {
	manifest, config := writeFixtures(t)

	out, err := run(t, "list", "--manifest", manifest, "--config", config, "--profile", "observability")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "semgrep") || !strings.Contains(out, "otel-collector") {
		t.Errorf("--profile observability output wrong:\n%s", out)
	}

	out, err = run(t, "list", "--manifest", manifest, "--config", config, "--source", "trailofbits")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "otel") || !strings.Contains(out, "semgrep") {
		t.Errorf("--source trailofbits output wrong:\n%s", out)
	}
}

func TestListJSON(t *testing.T) {
	manifest, config := writeFixtures(t)
	out, err := run(t, "list", "-o", "json", "--manifest", manifest, "--config", config)
	if err != nil {
		t.Fatal(err)
	}
	var rows []listRow
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if len(rows) != 3 || rows[0].Key != "getsentry/sentry-fix-issues" || rows[0].Profiles[0] != "observability" {
		t.Errorf("rows = %+v", rows)
	}
}

func TestListWithoutDefaultConfig(t *testing.T) {
	manifest, _ := writeFixtures(t)
	t.Setenv("LOADOUT_CONFIG", filepath.Join(t.TempDir(), "missing.hjson"))
	out, err := run(t, "list", "--manifest", manifest)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "PROFILES") {
		t.Errorf("profiles column shown without a config:\n%s", out)
	}
}

func TestListErrors(t *testing.T) {
	manifest, config := writeFixtures(t)
	missing := filepath.Join(t.TempDir(), "nope.hjson")

	tests := map[string][]string{
		"missing manifest":        {"list", "--manifest", filepath.Join(t.TempDir(), "nope.json"), "--config", config},
		"explicit missing config": {"list", "--manifest", manifest, "--config", missing},
		"unknown profile":         {"list", "--manifest", manifest, "--config", config, "--profile", "ghost"},
		"unknown output":          {"list", "--manifest", manifest, "--config", config, "-o", "yaml"},
	}
	for name, args := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := run(t, args...); err == nil {
				t.Fatal("want error")
			}
		})
	}
}

func TestListScansLibrary(t *testing.T) {
	t.Setenv("LOADOUT_MANIFEST", "")
	t.Setenv("LOADOUT_CONFIG", filepath.Join(t.TempDir(), "missing.hjson"))
	lib := t.TempDir()
	dir := filepath.Join(lib, "src", "skills", "alpha")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: alpha\ndescription: does things\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := run(t, "list", "--library", lib)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "src/alpha") {
		t.Errorf("scanned skill missing:\n%s", out)
	}
}

func TestListMissingLibraryExplainsHowToFix(t *testing.T) {
	t.Setenv("LOADOUT_MANIFEST", "")
	t.Setenv("LOADOUT_CONFIG", filepath.Join(t.TempDir(), "missing.hjson"))
	_, err := run(t, "list", "--library", filepath.Join(t.TempDir(), "nope"))
	if err == nil || !strings.Contains(err.Error(), "hms") {
		t.Fatalf("error = %v, want a hint about hms", err)
	}
}
