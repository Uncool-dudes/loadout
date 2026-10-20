package skill

import (
	"fmt"
	"strings"
)

// Selector picks skills: one exact skill ("trailofbits/semgrep") or a whole
// source ("trailofbits/*"). Nothing richer is supported on purpose.
type Selector struct {
	source string
	name   string // empty selects the whole source
}

func ParseSelector(s string) (Selector, error) {
	source, name, ok := strings.Cut(s, "/")
	if !ok || !plainPart(source) {
		return Selector{}, fmt.Errorf("invalid selector %q: want <source>/<name> or <source>/*", s)
	}
	if name == "*" {
		return Selector{source: source}, nil
	}
	if !plainPart(name) {
		return Selector{}, fmt.Errorf("invalid selector %q: want <source>/<name> or <source>/*", s)
	}
	return Selector{source: source, name: name}, nil
}

func (s Selector) Matches(k Key) bool {
	return k.source == s.source && (s.name == "" || k.name == s.name)
}

func (s Selector) String() string {
	if s.name == "" {
		return s.source + "/*"
	}
	return s.source + "/" + s.name
}
