package profile

import (
	"errors"
	"testing"

	"github.com/Uncool-dudes/loadout/internal/skill"
)

const sample = `
{
  global: [
    base
  ]
  invocation: {
    trailofbits/*: user
    trailofbits/sharp-edges: model
  }
  profiles: {
    base: {
      skills: [
        mattpocock/code-review
      ]
    }
    observability: {
      extends: [
        base
      ]
      skills: [
        ollygarden/*
        getsentry/sentry-fix-issues
      ]
      exclude: [
        ollygarden/otel-telemetrygen
      ]
      invocation: {
        ollygarden/otel-collector: model
      }
      settings: {
        permissions: {
          allow: [
            "mcp__dash0__*"
          ]
        }
      }
      mcp_servers: {
        dash0: {
          command: dash0-mcp
          env: {
            DASH0_TOKEN: ${DASH0_TOKEN}
          }
        }
      }
      strict_mcp: false
    }
    security: {
      skills: [
        trailofbits/*
      ]
      invocation: {
        trailofbits/sharp-edges: both
      }
    }
  }
  projects: {
    github.com/ratchio/gamma: [
      observability
    ]
    github.com/ratchio/*: [
      base
    ]
  }
}
`

func mustParse(t *testing.T, src string) *Config {
	t.Helper()
	cfg, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return cfg
}

func key(t *testing.T, s string) skill.Key {
	t.Helper()
	k, err := skill.ParseKey(s)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func TestParseSample(t *testing.T) {
	cfg := mustParse(t, sample)
	if got := cfg.Global(); len(got) != 1 || got[0] != "base" {
		t.Errorf("Global() = %v, want [base]", got)
	}
}

func TestParseRejects(t *testing.T) {
	tests := []struct {
		name    string
		src     string
		wantErr error
	}{
		{
			name: "unknown top-level key",
			src:  `{"gloabl":["base"],"profiles":{"base":{}}}`,
		},
		{
			name: "unknown profile key",
			src:  `{"profiles":{"base":{"exlcude":["a/b"]}}}`,
		},
		{
			// HJSON quoteless strings run to end of line, so this swallows
			// the closing bracket into the name.
			name: "inline list swallowed by quoteless string",
			src:  "{\n global: [base]\n profiles: { base: {} }\n}",
		},
		{
			name:    "unknown extends",
			src:     `{"profiles":{"a":{"extends":["ghost"]}}}`,
			wantErr: ErrUnknownProfile,
		},
		{
			name:    "unknown global",
			src:     `{"global":["ghost"],"profiles":{"a":{}}}`,
			wantErr: ErrUnknownProfile,
		},
		{
			name:    "extends cycle",
			src:     `{"profiles":{"a":{"extends":["b"]},"b":{"extends":["a"]}}}`,
			wantErr: ErrCycle,
		},
		{
			name: "bad invocation value",
			src:  `{"profiles":{"a":{"invocation":{"x/y":"sometimes"}}}}`,
		},
		{
			name: "bad selector",
			src:  `{"profiles":{"a":{"skills":["nosource"]}}}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse([]byte(tt.src))
			if err == nil {
				t.Fatal("Parse succeeded, want error")
			}
			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestProjectProfiles(t *testing.T) {
	cfg := mustParse(t, sample)
	tests := []struct {
		repo string
		want []Name
		ok   bool
	}{
		{"github.com/ratchio/gamma", []Name{"observability"}, true},
		{"github.com/ratchio/alpha", []Name{"base"}, true},
		{"github.com/samber/other", nil, false},
	}
	for _, tt := range tests {
		got, ok, err := cfg.ProjectProfiles(tt.repo)
		if err != nil {
			t.Fatal(err)
		}
		if ok != tt.ok || len(got) != len(tt.want) || (ok && got[0] != tt.want[0]) {
			t.Errorf("ProjectProfiles(%q) = %v, %v; want %v, %v", tt.repo, got, ok, tt.want, tt.ok)
		}
	}
}

func TestProjectProfilesAmbiguous(t *testing.T) {
	cfg := mustParse(t, `{"profiles":{"a":{},"b":{}},"projects":{"github.com/x/*":["a"],"github.com/*/y":["b"]}}`)
	_, _, err := cfg.ProjectProfiles("github.com/x/y")
	if !errors.Is(err, ErrAmbiguousProject) {
		t.Fatalf("error = %v, want ErrAmbiguousProject", err)
	}
}
