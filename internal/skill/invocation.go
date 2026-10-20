package skill

import "fmt"

// Invocation says who may start a skill. The zero value is Both.
type Invocation int

const (
	// Both allows the slash command and auto-triggering.
	Both Invocation = iota
	// User is slash command only (disable-model-invocation: true).
	User
	// Model is auto-trigger only (user-invocable: false).
	Model
)

func ParseInvocation(s string) (Invocation, error) {
	switch s {
	case "both":
		return Both, nil
	case "user":
		return User, nil
	case "model":
		return Model, nil
	}
	return Both, fmt.Errorf("invalid invocation %q: want both, user or model", s)
}

func (i Invocation) String() string {
	switch i {
	case User:
		return "user"
	case Model:
		return "model"
	}
	return "both"
}

// Rules maps selectors to an Invocation.
type Rules map[Selector]Invocation

// For returns the rule for k, preferring an exact selector over a source
// wildcard.
func (r Rules) For(k Key) (Invocation, bool) {
	if inv, ok := r[Selector{source: k.source, name: k.name}]; ok {
		return inv, true
	}
	inv, ok := r[Selector{source: k.source}]
	return inv, ok
}
