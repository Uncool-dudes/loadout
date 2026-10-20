package cli

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const launchConfig = `{"profiles":{
  "security":{
    "skills":["trailofbits/*"],
    "settings":{"permissions":{"defaultMode":"plan","allow":["Read"]}},
    "mcp_servers":{"dash0":{"command":"dash0-mcp","env":{"TOKEN":"${DASH0_TOKEN}"}}},
    "strict_mcp":true
  },
  "plain":{"skills":["trailofbits/*"]},
  "auto":{"settings":{"permissions":{"defaultMode":"auto"}}}
}}`

// fakeClaude stands in for the claude binary and records how it was started.
type fakeClaude struct {
	argv     []string
	started  bool
	notFound bool
}

func stubClaude(t *testing.T) *fakeClaude {
	t.Helper()
	f := &fakeClaude{}
	oldLook, oldExec := lookPath, execClaude
	t.Cleanup(func() { lookPath, execClaude = oldLook, oldExec })
	lookPath = func(name string) (string, error) {
		if f.notFound {
			return "", errors.New("executable file not found in $PATH")
		}
		return "/fake/" + name, nil
	}
	execClaude = func(path string, argv []string) error {
		f.started = true
		f.argv = argv
		return nil
	}
	return f
}

func (e syncEnv) exec(t *testing.T, args ...string) (string, error) {
	t.Helper()
	all := append([]string{"exec", "--manifest", e.manifest, "--config", e.config}, args...)
	return run(t, all...)
}

func readJSON(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return out
}

func TestExecLaunchesWithSettingsAndMCP(t *testing.T) {
	e := newSyncEnv(t, launchConfig)
	launchDir := t.TempDir()
	t.Setenv("LOADOUT_LAUNCH", launchDir)
	claude := stubClaude(t)

	if _, err := e.exec(t, "--profile", "security", "--", "--resume", "abc"); err != nil {
		t.Fatal(err)
	}
	if !claude.started {
		t.Fatal("claude was not started")
	}

	// argv: claude --settings f --mcp-config f --strict-mcp-config --resume abc
	argv := claude.argv
	if len(argv) != 8 || argv[0] != "claude" || argv[1] != "--settings" || argv[3] != "--mcp-config" ||
		argv[5] != "--strict-mcp-config" || argv[6] != "--resume" || argv[7] != "abc" {
		t.Fatalf("argv = %v", argv)
	}
	for _, path := range []string{argv[2], argv[4]} {
		if !strings.HasPrefix(path, launchDir) {
			t.Errorf("%s is outside the launch cache", path)
		}
	}

	perms := readJSON(t, argv[2])["permissions"].(map[string]any)
	if perms["defaultMode"] != "plan" {
		t.Errorf("settings = %v", perms)
	}
	// Secrets stay references: Claude expands them at launch, so the cached
	// file never holds a token.
	servers := readJSON(t, argv[4])["mcpServers"].(map[string]any)
	if got := servers["dash0"].(map[string]any)["env"].(map[string]any)["TOKEN"]; got != "${DASH0_TOKEN}" {
		t.Errorf("TOKEN = %v, want the unexpanded reference", got)
	}

	if got := e.linked(t); strings.Join(got, ",") != "semgrep" {
		t.Errorf("skills linked = %v", got)
	}
}

func TestExecWithoutLaunchConfigJustPassesArguments(t *testing.T) {
	e := newSyncEnv(t, launchConfig)
	t.Setenv("LOADOUT_LAUNCH", t.TempDir())
	claude := stubClaude(t)

	if _, err := e.exec(t, "--profile", "plain", "--", "-p", "hi"); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(claude.argv, " "); got != "claude -p hi" {
		t.Errorf("argv = %q", got)
	}
}

func TestExecDryRunStartsNothing(t *testing.T) {
	e := newSyncEnv(t, launchConfig)
	launchDir := t.TempDir()
	t.Setenv("LOADOUT_LAUNCH", launchDir)
	claude := stubClaude(t)

	out, err := e.exec(t, "--profile", "security", "--dry-run")
	if err != nil {
		t.Fatal(err)
	}
	if claude.started {
		t.Error("dry run started claude")
	}
	if entries, _ := os.ReadDir(launchDir); len(entries) != 0 {
		t.Errorf("dry run wrote launch files: %v", entries)
	}
	if got := e.linked(t); len(got) != 0 {
		t.Errorf("dry run linked %v", got)
	}
	for _, want := range []string{"claude --settings", "--strict-mcp-config", `"defaultMode": "plan"`, "${DASH0_TOKEN}"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestExecNoSync(t *testing.T) {
	e := newSyncEnv(t, launchConfig)
	t.Setenv("LOADOUT_LAUNCH", t.TempDir())
	stubClaude(t)

	if _, err := e.exec(t, "--profile", "security", "--no-sync"); err != nil {
		t.Fatal(err)
	}
	if got := e.linked(t); len(got) != 0 {
		t.Errorf("--no-sync linked %v", got)
	}
}

func TestExecErrors(t *testing.T) {
	t.Run("conflicting settings", func(t *testing.T) {
		e := newSyncEnv(t, launchConfig)
		t.Setenv("LOADOUT_LAUNCH", t.TempDir())
		claude := stubClaude(t)
		_, err := e.exec(t, "--profile", "security", "--profile", "auto")
		if err == nil || !strings.Contains(err.Error(), "conflicting settings") {
			t.Fatalf("error = %v", err)
		}
		if claude.started {
			t.Error("claude started despite the conflict")
		}
	})
	t.Run("claude not installed", func(t *testing.T) {
		e := newSyncEnv(t, launchConfig)
		t.Setenv("LOADOUT_LAUNCH", t.TempDir())
		claude := stubClaude(t)
		claude.notFound = true
		_, err := e.exec(t, "--profile", "plain")
		if err == nil || !strings.Contains(err.Error(), "find claude") {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestLaunchFilesAreContentAddressed(t *testing.T) {
	e := newSyncEnv(t, launchConfig)
	t.Setenv("LOADOUT_LAUNCH", t.TempDir())
	claude := stubClaude(t)

	if _, err := e.exec(t, "--profile", "security", "--no-sync"); err != nil {
		t.Fatal(err)
	}
	first := claude.argv[2]
	if _, err := e.exec(t, "--profile", "security", "--no-sync"); err != nil {
		t.Fatal(err)
	}
	if claude.argv[2] != first {
		t.Errorf("same profile produced two files: %s and %s", first, claude.argv[2])
	}
	if filepath.Base(first) != "settings.json" {
		t.Errorf("unexpected file name %s", first)
	}
}

func TestSyncNotesLaunchOnlyConfig(t *testing.T) {
	e := newSyncEnv(t, launchConfig)
	var errBuf strings.Builder
	cmd := NewRootCmd()
	cmd.SetOut(&strings.Builder{})
	cmd.SetErr(&errBuf)
	cmd.SetArgs([]string{"sync", "--manifest", e.manifest, "--config", e.config, "--profile", "security"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(errBuf.String(), "only `loadout exec` applies them") {
		t.Errorf("stderr = %q", errBuf.String())
	}
}
