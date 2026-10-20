package profile

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
)

func launchOf(t *testing.T, src string, roots ...Name) (Launch, error) {
	t.Helper()
	return mustResolve(t, mustParse(t, src), roots...).Launch()
}

func asJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestLaunchEmpty(t *testing.T) {
	l, err := launchOf(t, `{"profiles":{"a":{}}}`, "a")
	if err != nil || !l.IsEmpty() {
		t.Fatalf("Launch = %+v, %v; want empty", l, err)
	}
}

func TestLaunchChildOverridesParent(t *testing.T) {
	l, err := launchOf(t, `{"profiles":{
  "parent":{"settings":{"model":"opus","permissions":{"allow":["Bash(ls:*)"],"defaultMode":"default"}}},
  "child":{"extends":["parent"],"settings":{"permissions":{"allow":["Read"],"defaultMode":"acceptEdits"}}}
}}`, "child")
	if err != nil {
		t.Fatal(err)
	}
	want := `{"model":"opus","permissions":{"allow":["Bash(ls:*)","Read"],"defaultMode":"acceptEdits"}}`
	if got := asJSON(t, l.Settings); got != want {
		t.Errorf("settings = %s\nwant      %s", got, want)
	}
}

func TestLaunchSiblingsUnionAndDeepMerge(t *testing.T) {
	l, err := launchOf(t, `{"profiles":{
  "a":{"settings":{"permissions":{"allow":["Read","Edit"]},"hooks":{"PreToolUse":[{"matcher":"Bash"}]}}},
  "b":{"settings":{"permissions":{"allow":["Edit","Write"],"deny":["Bash(rm:*)"]},"hooks":{"PreToolUse":[{"matcher":"Bash"},{"matcher":"Edit"}]}}}
}}`, "a", "b")
	if err != nil {
		t.Fatal(err)
	}
	want := `{"hooks":{"PreToolUse":[{"matcher":"Bash"},{"matcher":"Edit"}]},"permissions":{"allow":["Read","Edit","Write"],"deny":["Bash(rm:*)"]}}`
	if got := asJSON(t, l.Settings); got != want {
		t.Errorf("settings = %s\nwant      %s", got, want)
	}
}

func TestLaunchSiblingConflict(t *testing.T) {
	src := `{"profiles":{
  "a":{"settings":{"permissions":{"defaultMode":"plan"}}}
  "b":{"settings":{"permissions":{"defaultMode":"auto"}}}
  "c":{"settings":{"permissions":{"defaultMode":"plan"}}}
}}`
	// HJSON allows newline-separated members, so the compact form above is valid.
	if _, err := launchOf(t, src, "a", "b"); !errors.Is(err, ErrSettingsConflict) {
		t.Fatalf("error = %v, want ErrSettingsConflict", err)
	}
	if _, err := launchOf(t, src, "a", "c"); err != nil {
		t.Fatalf("equal values should not conflict: %v", err)
	}
}

func TestLaunchMCPServers(t *testing.T) {
	l, err := launchOf(t, `{"profiles":{
  "base":{"mcp_servers":{"dash0":{"command":"dash0-mcp","env":{"TOKEN":"${DASH0_TOKEN}"}}}}
  "obs":{"extends":["base"],"strict_mcp":true,"mcp_servers":{"dash0":{"env":{"REGION":"eu"}},"sentry":{"url":"https://example.test/mcp"}}}
}}`, "obs")
	if err != nil {
		t.Fatal(err)
	}
	want := `{"dash0":{"command":"dash0-mcp","env":{"REGION":"eu","TOKEN":"${DASH0_TOKEN}"}},"sentry":{"url":"https://example.test/mcp"}}`
	if got := asJSON(t, l.MCPServers); got != want {
		t.Errorf("mcp = %s\nwant  %s", got, want)
	}
	if !l.StrictMCP {
		t.Error("StrictMCP not set")
	}
}

func TestLaunchDoesNotMutateConfig(t *testing.T) {
	cfg := mustParse(t, `{"profiles":{"a":{"settings":{"permissions":{"allow":["Read"]}}},"b":{"settings":{"permissions":{"allow":["Write"]}}}}}`)
	before := cfg.profiles["a"].settings
	snapshot := asJSON(t, before)
	if _, err := mustResolve(t, cfg, "a", "b").Launch(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(asJSON(t, cfg.profiles["a"].settings), snapshot) {
		t.Errorf("profile a changed from %s to %s", snapshot, asJSON(t, cfg.profiles["a"].settings))
	}
}
