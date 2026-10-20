// Package skill holds the domain vocabulary shared by the rest of the tool:
// what a skill is called, how profiles select it, and how it may be invoked.
package skill

import (
	"fmt"
	"strings"

	"github.com/Uncool-dudes/loadout/internal/ident"
)

// Key identifies a skill as "<source>/<name>", for example "trailofbits/semgrep".
// The source prefix is kept so two sources shipping the same skill name never
// collide until link time, where the clash is reported explicitly.
type Key struct {
	source string
	name   string
}

func ParseKey(s string) (Key, error) {
	source, name, ok := strings.Cut(s, "/")
	if !ok || !plainPart(source) || !plainPart(name) {
		return Key{}, fmt.Errorf("invalid skill key %q: want <source>/<name>", s)
	}
	return Key{source: source, name: name}, nil
}

func (k Key) Source() string { return k.source }

// Name is the directory name used under .claude/skills/.
func (k Key) Name() string { return k.name }

func (k Key) String() string { return k.source + "/" + k.name }

// plainPart reports whether s is one path segment with no wildcard.
func plainPart(s string) bool {
	return ident.Valid(s) && !strings.ContainsAny(s, "/*")
}
