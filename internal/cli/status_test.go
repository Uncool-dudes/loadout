package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func (e syncEnv) status(t *testing.T, args ...string) statusReport {
	t.Helper()
	all := append([]string{"status", "-o", "json", "--manifest", e.manifest, "--config", e.config}, args...)
	out, err := run(t, all...)
	if err != nil {
		t.Fatal(err)
	}
	var r statusReport
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	return r
}

func rowNamed(t *testing.T, r statusReport, name string) statusRow {
	t.Helper()
	for _, row := range r.Skills {
		if row.Name == name {
			return row
		}
	}
	t.Fatalf("no row named %q in %+v", name, r.Skills)
	return statusRow{}
}

func TestStatusReportsLinks(t *testing.T) {
	e := newSyncEnv(t, "")
	if _, err := e.sync(t, "--profile", "observability"); err != nil {
		t.Fatal(err)
	}
	own := filepath.Join(e.repo, ".claude", "skills", "mine")
	if err := os.MkdirAll(own, 0o755); err != nil {
		t.Fatal(err)
	}

	r := e.status(t, "--profile", "observability")
	fix := rowNamed(t, r, "sentry-fix-issues")
	if fix.Key != "getsentry/sentry-fix-issues" || fix.Invocation != "both" || fix.EstimatedTokens != 25 {
		t.Errorf("sentry-fix-issues = %+v", fix)
	}
	if got := rowNamed(t, r, "mine").State; got != "project-owned" {
		t.Errorf("mine state = %q", got)
	}
	if r.TotalTokens != 35 {
		t.Errorf("TotalTokens = %d, want 35", r.TotalTokens)
	}
	if r.Pending == nil || r.Pending.Added+r.Pending.Replaced+r.Pending.Removed != 0 {
		t.Errorf("pending = %+v, want up to date", r.Pending)
	}
}

func TestStatusShowsPendingChanges(t *testing.T) {
	e := newSyncEnv(t, "")
	if _, err := e.sync(t, "--profile", "observability"); err != nil {
		t.Fatal(err)
	}
	r := e.status(t, "--profile", "security")
	if r.Pending == nil || r.Pending.Added != 1 || r.Pending.Removed != 2 {
		t.Errorf("pending = %+v, want 1 added and 2 removed", r.Pending)
	}
}

func TestStatusWithoutProfileExplainsWhy(t *testing.T) {
	e := newSyncEnv(t, "")
	r := e.status(t)
	if r.Pending == nil || !strings.Contains(r.Pending.Note, "no profile selected") {
		t.Errorf("pending = %+v", r.Pending)
	}
}

func TestStatusStaleAndDangling(t *testing.T) {
	e := newSyncEnv(t, "")
	if _, err := e.sync(t, "--profile", "observability"); err != nil {
		t.Fatal(err)
	}

	// Drop a skill from the manifest: its link is now stale.
	trimmed := `{"version":1,"skills":[{"key":"ollygarden/otel-collector","path":"/nix/store/c-otel","descriptionBytes":40}]}`
	if err := os.WriteFile(e.manifest, []byte(trimmed), 0o600); err != nil {
		t.Fatal(err)
	}
	r := e.status(t)
	if got := rowNamed(t, r, "sentry-fix-issues").State; got != "stale" {
		t.Errorf("sentry-fix-issues state = %q, want stale", got)
	}
	// The fixture store paths do not exist on disk, so the surviving link dangles.
	if got := rowNamed(t, r, "otel-collector").State; got != "dangling" {
		t.Errorf("otel-collector state = %q, want dangling", got)
	}
}

func TestStatusShowsAppliedInvocation(t *testing.T) {
	e := newSyncEnv(t, userOnlyConfig)
	useRealSkill(t, e)
	if _, err := e.sync(t, "--profile", "security"); err != nil {
		t.Fatal(err)
	}
	r := e.status(t, "--profile", "security")
	if got := rowNamed(t, r, "semgrep"); got.Invocation != "user" || got.State != "ok" {
		t.Errorf("semgrep = %+v", got)
	}
}

func TestStatusText(t *testing.T) {
	e := newSyncEnv(t, "")
	if _, err := e.sync(t, "--profile", "security"); err != nil {
		t.Fatal(err)
	}
	out, err := run(t, "status", "--manifest", e.manifest, "--config", e.config, "--profile", "security")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"NAME", "trailofbits/semgrep", "total: ~100", "pending: up to date"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}
