package skill

import "testing"

func TestParseSelector(t *testing.T) {
	tests := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{in: "trailofbits/semgrep", want: "trailofbits/semgrep"},
		{in: "trailofbits/*", want: "trailofbits/*"},
		{in: "*/semgrep", wantErr: true},
		{in: "trailofbits/sem*", wantErr: true},
		{in: "trailofbits", wantErr: true},
		{in: "a/b/c", wantErr: true},
		{in: "trailofbits/semgrep]", wantErr: true},
		{in: "a, b/c", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := ParseSelector(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseSelector(%q) error = %v, wantErr %v", tt.in, err, tt.wantErr)
			}
			if err == nil && got.String() != tt.want {
				t.Fatalf("ParseSelector(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestSelectorMatches(t *testing.T) {
	key, _ := ParseKey("trailofbits/semgrep")
	tests := []struct {
		selector string
		want     bool
	}{
		{"trailofbits/semgrep", true},
		{"trailofbits/*", true},
		{"trailofbits/codeql", false},
		{"ollygarden/*", false},
	}
	for _, tt := range tests {
		sel, err := ParseSelector(tt.selector)
		if err != nil {
			t.Fatal(err)
		}
		if got := sel.Matches(key); got != tt.want {
			t.Errorf("%s.Matches(%s) = %v, want %v", sel, key, got, tt.want)
		}
	}
}

func TestRulesForPrefersExactOverWildcard(t *testing.T) {
	wild, _ := ParseSelector("trailofbits/*")
	exact, _ := ParseSelector("trailofbits/sharp-edges")
	rules := Rules{wild: User, exact: Model}

	sharp, _ := ParseKey("trailofbits/sharp-edges")
	other, _ := ParseKey("trailofbits/semgrep")
	unrelated, _ := ParseKey("ollygarden/otel-collector")

	if got, ok := rules.For(sharp); !ok || got != Model {
		t.Errorf("sharp-edges = %v, %v; want Model", got, ok)
	}
	if got, ok := rules.For(other); !ok || got != User {
		t.Errorf("semgrep = %v, %v; want User", got, ok)
	}
	if _, ok := rules.For(unrelated); ok {
		t.Error("unrelated skill matched a rule")
	}
}
