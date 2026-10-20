package source

import (
	"errors"
	"strings"
	"testing"
)

const rev = "0123456789abcdef0123456789abcdef01234567"
const hash = "sha256-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="

func validSource() Source {
	return Source{Owner: "trailofbits", Repo: "skills", Rev: rev, Hash: hash, Path: "plugins"}
}

func TestParseMarshalRoundTrip(t *testing.T) {
	in := `{
  "caveman": {
    "owner": "JuliusBrussee",
    "repo": "caveman",
    "rev": "` + rev + `",
    "hash": "` + hash + `",
    "path": "skills"
  },
  "humanizer": {
    "owner": "blader",
    "repo": "humanizer",
    "rev": "` + rev + `",
    "hash": "` + hash + `",
    "include": [
      "SKILL.md"
    ]
  },
  "insecure": {
    "owner": "trailofbits",
    "repo": "skills",
    "rev": "` + rev + `",
    "hash": "` + hash + `",
    "library": false,
    "pinned": true
  }
}
`
	set, err := Parse([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	out, err := set.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != in {
		t.Errorf("round trip changed the file:\n%s\nwant:\n%s", out, in)
	}
	if src, _ := set.Get("insecure"); src.InLibrary() || !src.Pinned {
		t.Errorf("insecure = %+v", src)
	}
	if src, _ := set.Get("caveman"); !src.InLibrary() {
		t.Error("a source without the library key must be in the library")
	}
}

func TestParseRejects(t *testing.T) {
	good := `"owner":"o","repo":"r","rev":"` + rev + `","hash":"` + hash + `"`
	tests := map[string]string{
		"unknown field":    `{"a":{` + good + `,"extra":1}}`,
		"short rev":        `{"a":{"owner":"o","repo":"r","rev":"abc","hash":"` + hash + `"}}`,
		"non-SRI hash":     `{"a":{"owner":"o","repo":"r","rev":"` + rev + `","hash":"abc"}}`,
		"path and include": `{"a":{` + good + `,"path":"x","include":["y"]}}`,
		"parent traversal": `{"a":{` + good + `,"path":"../x"}}`,
		"absolute path":    `{"a":{` + good + `,"path":"/x"}}`,
		"shell metachar":   `{"a":{` + good + `,"path":"x;rm"}}`,
		"bad source name":  `{"a/b":{` + good + `}}`,
		"bad owner":        `{"a":{"owner":"o o","repo":"r","rev":"` + rev + `","hash":"` + hash + `"}}`,
		"not json":         `owner = 1`,
	}
	for name, src := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse([]byte(src)); err == nil {
				t.Fatal("Parse succeeded, want error")
			}
		})
	}
}

func TestParseEmptyFile(t *testing.T) {
	set, err := Parse(nil)
	if err != nil || len(set.Names()) != 0 {
		t.Fatalf("Parse(nil) = %v, %v", set.Names(), err)
	}
	out, _ := set.Marshal()
	if strings.TrimSpace(string(out)) != "{}" {
		t.Errorf("empty set marshals to %q", out)
	}
}

func TestAddReplaceRemove(t *testing.T) {
	var set Set
	if err := set.Add("tob", validSource()); err != nil {
		t.Fatal(err)
	}
	if err := set.Add("tob", validSource()); !errors.Is(err, ErrExists) {
		t.Errorf("duplicate Add error = %v, want ErrExists", err)
	}
	updated := validSource()
	updated.Rev = strings.Repeat("a", 40)
	if err := set.Replace("tob", updated); err != nil {
		t.Fatal(err)
	}
	if got, _ := set.Get("tob"); got.Rev != updated.Rev {
		t.Errorf("Replace kept rev %s", got.Rev)
	}
	if err := set.Replace("ghost", updated); !errors.Is(err, ErrNotFound) {
		t.Errorf("Replace unknown error = %v, want ErrNotFound", err)
	}
	if err := set.Remove("tob"); err != nil {
		t.Fatal(err)
	}
	if err := set.Remove("tob"); !errors.Is(err, ErrNotFound) {
		t.Errorf("second Remove error = %v, want ErrNotFound", err)
	}
	if err := set.Add("bad name", validSource()); err == nil {
		t.Error("Add accepted an invalid name")
	}
}

func TestPickRev(t *testing.T) {
	branch := strings.Repeat("b", 40)
	tag := strings.Repeat("c", 40)
	peeled := strings.Repeat("d", 40)
	out := branch + "\trefs/heads/v1\n" + tag + "\trefs/tags/v1\n" + peeled + "\trefs/tags/v1^{}\n"
	patterns := []string{"refs/heads/v1", "refs/tags/v1^{}", "refs/tags/v1"}

	if got, err := pickRev(out, patterns); err != nil || got != branch {
		t.Errorf("branch must win: %s, %v", got, err)
	}
	tagsOnly := tag + "\trefs/tags/v1\n" + peeled + "\trefs/tags/v1^{}\n"
	if got, err := pickRev(tagsOnly, patterns); err != nil || got != peeled {
		t.Errorf("annotated tag must resolve to its commit: %s, %v", got, err)
	}
	if _, err := pickRev("", patterns); err == nil {
		t.Error("pickRev accepted empty output")
	}
}
