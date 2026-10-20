package plan

import (
	"slices"
	"testing"

	"github.com/Uncool-dudes/loadout/internal/skill"
)

func desired(t *testing.T, key, target string) Desired {
	t.Helper()
	k, err := skill.ParseKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return Desired{Key: k, Target: target}
}

func ops(p Plan) []string {
	var out []string
	for _, a := range p.Actions {
		out = append(out, string(a.Op)+" "+a.Name)
	}
	return out
}

func TestCompute(t *testing.T) {
	tests := []struct {
		name         string
		in           Input
		wantOps      []string
		wantConflict []string
		wantManaged  []string
	}{
		{
			name: "fresh add",
			in: Input{
				Desired: []Desired{desired(t, "t/semgrep", "/s/a"), desired(t, "o/otel", "/s/b")},
			},
			wantOps:     []string{"add otel", "add semgrep"},
			wantManaged: []string{"otel", "semgrep"},
		},
		{
			name: "unchanged link is kept",
			in: Input{
				Desired:  []Desired{desired(t, "t/semgrep", "/s/a")},
				Existing: []Existing{{Name: "semgrep", Kind: Symlink, Target: "/s/a"}},
				Managed:  []string{"semgrep"},
			},
			wantOps:     []string{"keep semgrep"},
			wantManaged: []string{"semgrep"},
		},
		{
			name: "new store path replaces managed link",
			in: Input{
				Desired:  []Desired{desired(t, "t/semgrep", "/s/new")},
				Existing: []Existing{{Name: "semgrep", Kind: Symlink, Target: "/s/old"}},
				Managed:  []string{"semgrep"},
			},
			wantOps:     []string{"replace semgrep"},
			wantManaged: []string{"semgrep"},
		},
		{
			name: "managed link no longer wanted is removed",
			in: Input{
				Existing: []Existing{{Name: "semgrep", Kind: Symlink, Target: "/s/a"}},
				Managed:  []string{"semgrep"},
			},
			wantOps: []string{"remove semgrep"},
		},
		{
			name: "unmanaged link is left alone even when not wanted",
			in: Input{
				Existing: []Existing{{Name: "mine", Kind: Symlink, Target: "/elsewhere"}},
			},
		},
		{
			name: "real directory in the way",
			in: Input{
				Desired:  []Desired{desired(t, "t/semgrep", "/s/a")},
				Existing: []Existing{{Name: "semgrep", Kind: Directory}},
			},
			wantConflict: []string{"semgrep"},
		},
		{
			name: "unmanaged symlink in the way",
			in: Input{
				Desired:  []Desired{desired(t, "t/semgrep", "/s/a")},
				Existing: []Existing{{Name: "semgrep", Kind: Symlink, Target: "/other"}},
			},
			wantConflict: []string{"semgrep"},
		},
		{
			name: "two sources with the same skill name",
			in: Input{
				Desired: []Desired{desired(t, "a/semgrep", "/s/a"), desired(t, "b/semgrep", "/s/b")},
			},
			wantConflict: []string{"semgrep"},
		},
		{
			name: "shadowed by a personal skill",
			in: Input{
				Desired:  []Desired{desired(t, "t/semgrep", "/s/a"), desired(t, "o/otel", "/s/b")},
				Shadowed: []string{"semgrep"},
			},
			wantOps:      []string{"add otel"},
			wantConflict: []string{"semgrep"},
			wantManaged:  []string{"otel"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := Compute(tt.in)
			if got := ops(p); !slices.Equal(got, tt.wantOps) {
				t.Errorf("ops = %v, want %v", got, tt.wantOps)
			}
			var gotConflicts []string
			for _, c := range p.Conflicts {
				gotConflicts = append(gotConflicts, c.Name)
			}
			if !slices.Equal(gotConflicts, tt.wantConflict) {
				t.Errorf("conflicts = %v, want %v", gotConflicts, tt.wantConflict)
			}
			if !slices.Equal(p.Managed, tt.wantManaged) {
				t.Errorf("managed = %v, want %v", p.Managed, tt.wantManaged)
			}
		})
	}
}

func TestHasChanges(t *testing.T) {
	kept := Plan{Actions: []Action{{Op: Keep, Name: "a"}}}
	if kept.HasChanges() {
		t.Error("keep-only plan reports changes")
	}
	added := Plan{Actions: []Action{{Op: Keep, Name: "a"}, {Op: Add, Name: "b"}}}
	if !added.HasChanges() {
		t.Error("plan with an add reports no changes")
	}
}
