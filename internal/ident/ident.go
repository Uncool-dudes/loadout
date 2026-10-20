// Package ident validates the plain tokens used for source, skill and profile
// names.
package ident

import (
	"strings"
	"unicode"
)

// forbidden holds characters that HJSON's quoteless strings would swallow:
// `skills: [a, b]` parses as the single string "a, b]". Rejecting them turns
// that silent mis-parse into an error.
const forbidden = `[]{},"'`

func Valid(s string) bool {
	if s == "" {
		return false
	}
	return !strings.ContainsFunc(s, func(r rune) bool {
		return unicode.IsSpace(r) || strings.ContainsRune(forbidden, r)
	})
}
