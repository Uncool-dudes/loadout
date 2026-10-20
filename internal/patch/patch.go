// Package patch rewrites a SKILL.md header so a skill's invocation matches a
// profile's rule, without touching anything else in the file.
package patch

import (
	"errors"
	"regexp"
	"strings"

	"github.com/Uncool-dudes/loadout/internal/skill"
)

var ErrNoFrontmatter = errors.New("SKILL.md has no YAML frontmatter")

// invocationKeys are removed from the header before the wanted values are
// appended, so the result never carries a duplicate key.
var invocationKeys = regexp.MustCompile(`^(disable-model-invocation|user-invocable)\s*:`)

// Invocation returns src with its header set to inv. Both means "as authored"
// and returns src unchanged. The edit is line based on purpose: re-encoding
// the header as YAML would reorder keys and drop comments the author wrote.
func Invocation(src []byte, inv skill.Invocation) ([]byte, error) {
	if inv == skill.Both {
		return src, nil
	}

	lines := strings.Split(string(src), "\n")
	cr := ""
	if strings.HasSuffix(lines[0], "\r") {
		cr = "\r"
	}
	if strings.TrimSuffix(lines[0], "\r") != "---" {
		return nil, ErrNoFrontmatter
	}
	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSuffix(lines[i], "\r") == "---" {
			end = i
			break
		}
	}
	if end < 0 {
		return nil, ErrNoFrontmatter
	}

	var header []string
	for _, line := range lines[1:end] {
		if !invocationKeys.MatchString(line) {
			header = append(header, line)
		}
	}
	// Explicit values in both directions, so an author's own setting on the
	// other flag cannot leak through.
	switch inv {
	case skill.User:
		header = append(header, "disable-model-invocation: true"+cr, "user-invocable: true"+cr)
	case skill.Model:
		header = append(header, "disable-model-invocation: false"+cr, "user-invocable: false"+cr)
	}

	out := append([]string{lines[0]}, header...)
	out = append(out, lines[end:]...)
	return []byte(strings.Join(out, "\n")), nil
}
