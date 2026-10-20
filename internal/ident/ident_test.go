package ident

import "testing"

func TestValid(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"semgrep", true},
		{"sharp-edges", true},
		{"github.com", true},
		{"", false},
		{"a b", false},
		{"base]", false},
		{"a,b", false},
		{"{x}", false},
		{`"quoted"`, false},
	}
	for _, tt := range tests {
		if got := Valid(tt.in); got != tt.want {
			t.Errorf("Valid(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}
