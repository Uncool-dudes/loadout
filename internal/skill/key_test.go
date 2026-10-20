package skill

import "testing"

func TestParseKey(t *testing.T) {
	tests := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{in: "trailofbits/semgrep", want: "trailofbits/semgrep"},
		{in: "semgrep", wantErr: true},
		{in: "/semgrep", wantErr: true},
		{in: "trailofbits/", wantErr: true},
		{in: "a/b/c", wantErr: true},
		{in: "trailofbits/*", wantErr: true},
		{in: "trailofbits/semgrep]", wantErr: true},
		{in: "a b/c", wantErr: true},
		{in: "", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := ParseKey(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseKey(%q) error = %v, wantErr %v", tt.in, err, tt.wantErr)
			}
			if err == nil && got.String() != tt.want {
				t.Fatalf("ParseKey(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
