package profile

import (
	"errors"
	"fmt"
	"maps"
	"reflect"
	"slices"
)

var ErrSettingsConflict = errors.New("conflicting settings")

// Launch is what `claude` is started with beyond skills: a settings fragment
// for --settings and MCP servers for --mcp-config. Both stay generic JSON
// trees, so keys Claude Code adds later pass through untouched.
type Launch struct {
	Settings   map[string]any
	MCPServers map[string]any
	// StrictMCP means only MCPServers load, ignoring every other MCP config.
	StrictMCP bool
}

func (l Launch) IsEmpty() bool {
	return len(l.Settings) == 0 && len(l.MCPServers) == 0 && !l.StrictMCP
}

// Launch merges the settings and MCP servers of the active profiles.
//
// Within one profile chain a child overrides the profile it extends. Across
// active profiles, arrays are unioned, objects are merged key by key, and two
// profiles that give the same key different values are an error: picking one
// would silently change behaviour, as with invocation rules.
func (a *Active) Launch() (Launch, error) {
	out := Launch{Settings: map[string]any{}, MCPServers: map[string]any{}}
	for _, root := range a.roots {
		settings, servers := map[string]any{}, map[string]any{}
		chain := a.cfg.chain(root)
		// Farthest ancestor first, so nearer profiles override it.
		for i := len(chain) - 1; i >= 0; i-- {
			if err := mergeInto(settings, chain[i].settings, false, "settings"); err != nil {
				return Launch{}, err
			}
			if err := mergeInto(servers, chain[i].mcpServers, false, "mcp_servers"); err != nil {
				return Launch{}, err
			}
			out.StrictMCP = out.StrictMCP || chain[i].strictMCP
		}
		if err := mergeInto(out.Settings, settings, true, fmt.Sprintf("settings (profile %s)", root)); err != nil {
			return Launch{}, err
		}
		if err := mergeInto(out.MCPServers, servers, true, fmt.Sprintf("mcp_servers (profile %s)", root)); err != nil {
			return Launch{}, err
		}
	}
	return out, nil
}

// mergeInto folds src into dst. Objects merge recursively and arrays union
// without duplicates. For anything else that differs, strict reports a
// conflict and non-strict lets src win.
func mergeInto(dst, src map[string]any, strict bool, where string) error {
	for _, key := range slices.Sorted(maps.Keys(src)) {
		value := src[key]
		existing, found := dst[key]
		if !found {
			dst[key] = clone(value)
			continue
		}

		switch ev := existing.(type) {
		case map[string]any:
			if sv, ok := value.(map[string]any); ok {
				if err := mergeInto(ev, sv, strict, where+"."+key); err != nil {
					return err
				}
				continue
			}
		case []any:
			if sv, ok := value.([]any); ok {
				for _, item := range sv {
					if !slices.ContainsFunc(ev, func(e any) bool { return reflect.DeepEqual(e, item) }) {
						ev = append(ev, clone(item))
					}
				}
				dst[key] = ev
				continue
			}
		}

		if reflect.DeepEqual(existing, value) {
			continue
		}
		if strict {
			return fmt.Errorf("%w: %s.%s is %v in one profile and %v in another", ErrSettingsConflict, where, key, existing, value)
		}
		dst[key] = clone(value)
	}
	return nil
}

// clone copies a decoded JSON tree, so merging never writes into the config.
func clone(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, e := range t {
			out[k] = clone(e)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, e := range t {
			out[i] = clone(e)
		}
		return out
	}
	return v
}
