package profile

import (
	"errors"
	"testing"

	"github.com/Uncool-dudes/loadout/internal/skill"
)

func mustResolve(t *testing.T, cfg *Config, names ...Name) *Active {
	t.Helper()
	a, err := cfg.Resolve(names...)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func TestSelects(t *testing.T) {
	cfg := mustParse(t, sample)
	tests := []struct {
		skill string
		want  bool
	}{
		{"mattpocock/code-review", true}, // inherited from base
		{"ollygarden/otel-collector", true},
		{"getsentry/sentry-fix-issues", true},
		{"ollygarden/otel-telemetrygen", false}, // excluded
		{"trailofbits/semgrep", false},
	}
	active := mustResolve(t, cfg, "observability")
	for _, tt := range tests {
		if got := active.Selects(key(t, tt.skill)); got != tt.want {
			t.Errorf("Selects(%s) = %v, want %v", tt.skill, got, tt.want)
		}
	}
}

func TestExcludeDoesNotLeakAcrossSiblings(t *testing.T) {
	cfg := mustParse(t, `{"profiles":{"a":{"skills":["s/x","s/y"],"exclude":["s/x"]},"b":{"skills":["s/x"]}}}`)
	active := mustResolve(t, cfg, "a", "b")
	if !active.Selects(key(t, "s/x")) {
		t.Error("s/x excluded by a should still be selected through b")
	}
	if !active.Selects(key(t, "s/y")) {
		t.Error("s/y should be selected")
	}
}

func TestInvocationPrecedence(t *testing.T) {
	cfg := mustParse(t, sample)
	tests := []struct {
		name  string
		roots []Name
		skill string
		want  skill.Invocation
	}{
		{"top-level wildcard default", []Name{"base"}, "trailofbits/semgrep", skill.User},
		{"top-level exact beats wildcard", []Name{"base"}, "trailofbits/sharp-edges", skill.Model},
		{"profile beats default", []Name{"security"}, "trailofbits/sharp-edges", skill.Both},
		{"no rule means both", []Name{"base"}, "mattpocock/code-review", skill.Both},
		{"profile rule", []Name{"observability"}, "ollygarden/otel-collector", skill.Model},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := mustResolve(t, cfg, tt.roots...).InvocationFor(key(t, tt.skill))
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("InvocationFor(%s) = %v, want %v", tt.skill, got, tt.want)
			}
		})
	}
}

func TestInvocationChildBeatsParent(t *testing.T) {
	cfg := mustParse(t, `{"profiles":{"parent":{"invocation":{"s/x":"user"}},"child":{"extends":["parent"],"invocation":{"s/x":"model"}}}}`)
	got, err := mustResolve(t, cfg, "child").InvocationFor(key(t, "s/x"))
	if err != nil || got != skill.Model {
		t.Fatalf("InvocationFor = %v, %v; want Model", got, err)
	}
}

func TestInvocationSiblingConflict(t *testing.T) {
	// Only profile-level rules count as a disagreement between roots; the
	// top-level default is a fallback and never conflicts.
	cfg := mustParse(t, `{"profiles":{"a":{"invocation":{"s/x":"user"}},"b":{"invocation":{"s/x":"model"}},"c":{"invocation":{"s/x":"user"}}}}`)
	_, err := mustResolve(t, cfg, "a", "b").InvocationFor(key(t, "s/x"))
	if !errors.Is(err, ErrInvocationConflict) {
		t.Fatalf("error = %v, want ErrInvocationConflict", err)
	}
	got, err := mustResolve(t, cfg, "a", "c").InvocationFor(key(t, "s/x"))
	if err != nil || got != skill.User {
		t.Fatalf("agreeing siblings = %v, %v; want User", got, err)
	}
}

func TestResolveUnknownProfile(t *testing.T) {
	cfg := mustParse(t, sample)
	if _, err := cfg.Resolve("ghost"); !errors.Is(err, ErrUnknownProfile) {
		t.Fatalf("error = %v, want ErrUnknownProfile", err)
	}
}

func TestUnmatched(t *testing.T) {
	cfg := mustParse(t, `{"profiles":{
  "base":{"skills":["a/one","a/typo"]},
  "child":{"extends":["base"],"skills":["b/*","c/*"]}
}}`)
	m, err := skill.ParseManifest([]byte(`{"version":1,"skills":[
  {"key":"a/one","path":"/p/1","descriptionBytes":1},
  {"key":"b/two","path":"/p/2","descriptionBytes":1}
]}`))
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, sel := range mustResolve(t, cfg, "child").Unmatched(m) {
		got = append(got, sel.String())
	}
	if len(got) != 2 || got[0] != "a/typo" || got[1] != "c/*" {
		t.Errorf("Unmatched = %v, want [a/typo c/*]", got)
	}
}
