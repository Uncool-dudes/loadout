# loadout

Per-project Claude Code setup from named profiles: which skills are linked,
and which settings and MCP servers `claude` starts with.

A skill that is not linked is not loaded, so nothing needs switching off.

## Install

```
nix profile install github:Uncool-dudes/loadout
```

or `go install github.com/Uncool-dudes/loadout/cmd/loadout@latest`. The flake
also provides `overlays.default` and a dev shell.

## How it works

- A **library** is a directory of skill sources, one subdirectory (or symlink)
  per source. Every directory containing a `SKILL.md` is a skill, keyed
  `<source>/<name>`, for example `trailofbits/semgrep`.
- `loadout.hjson` defines **profiles**: lists of skill selectors, plus optional
  settings and MCP servers.
- `sync` symlinks the selected skills into `<repo>/.claude/skills/`, or into
  `~/.claude/skills/` with `--global`.
- `exec` runs `sync`, merges the profiles' settings and MCP servers into
  files, and replaces itself with `claude --settings ... --mcp-config ...`.

The library is expected to be built by Nix, but any directory works.

## Commands

| Command | What it does |
| --- | --- |
| `sync [--profile p]... [--dry-run] [--global]` | Link the selected skills, remove links it made earlier that are no longer wanted. Any conflict aborts before changes. |
| `exec [--profile p]... [--no-sync] [--dry-run] [-- claude-args]` | Sync, then start `claude` with the merged settings and MCP servers. |
| `status [--profile p]... [--global] [-o json]` | Linked skills, effective invocation, description token cost, pending changes. |
| `list [--source s] [--profile p] [-o json]` | Every skill in the library and the profiles that select it. |
| `add owner/repo[@ref] [--name n] [--path p] [--include f]... [--no-library] [--pinned] [--dry-run]` | Pin a GitHub repository as a skill source in `sources.json`: resolve the ref, prefetch the hash, detect where the skills are. |
| `update [name...] [--force] [--dry-run]` | Move sources to the latest commit of their ref. Pinned sources are skipped unless named or `--force`. |
| `remove name` | Remove a source from `sources.json`. |
| `sources` | List the pinned sources. |
| `gc [--dry-run] [--older-than 720h]` | Remove patched copies and launch files nothing can use. |
| `completion zsh\|bash\|fish` | Shell completion for profiles and sources. |

Profiles are chosen by, in order: `--profile`, `LOADOUT_PROFILES`, the
`projects` entry for the repository.

## Paths

| Purpose | Environment variable | Default |
| --- | --- | --- |
| Library | `LOADOUT_LIBRARY` | `$XDG_DATA_HOME/loadout/library` |
| Config | `LOADOUT_CONFIG` | `$XDG_CONFIG_HOME/loadout/loadout.hjson` |
| Patched skills | `LOADOUT_CACHE` | `$XDG_CACHE_HOME/loadout/skills` |
| Launch files | `LOADOUT_LAUNCH` | `$XDG_CACHE_HOME/loadout/launch` |
| Sources | `LOADOUT_SOURCES` | `sources.json` next to the real path of the config file |

`--library`, `--config` and `--manifest` override the first two per run.
`--manifest` / `LOADOUT_MANIFEST` read a `manifest.json` instead of scanning.

## sources.json

Plain data, edited by `add`, `update` and `remove`, read by Nix to fetch and lay
out the library. One object per source, keyed by the name used in selectors:

```json
{
  "trailofbits": {
    "owner": "trailofbits",
    "repo": "skills",
    "rev": "<40-character commit>",
    "hash": "sha256-...",
    "path": "plugins"
  }
}
```

| Key | Meaning |
| --- | --- |
| `owner`, `repo` | GitHub repository. |
| `ref` | Branch or tag `update` follows. Default branch if absent. |
| `rev`, `hash` | Pinned commit and its Nix hash. |
| `path` | Directory inside the repository to scan for skills. |
| `include` | Files or directories to copy into the source instead of `path`, for example `["SKILL.md"]` for a repository whose only skill sits at its root. |
| `library` | `false` fetches the source without offering its skills (plugins wanted for hooks). |
| `pinned` | `true` keeps `update` from moving it. |

## loadout.hjson

HJSON. Quoteless strings run to the end of the line, so write lists one item
per line, never `[a, b]`. Names containing `] [ { } ,` are rejected for that
reason.

```hjson
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
    security: {
      extends: [
        base
      ]
      skills: [
        trailofbits/*
      ]
      exclude: [
        trailofbits/audit-context-building
      ]
      settings: {
        permissions: {
          allow: [
            "Bash(semgrep:*)"
          ]
        }
      }
      mcp_servers: {
        dash0: {
          command: dash0-mcp
          env: {
            DASH0_TOKEN: "${DASH0_TOKEN}"
          }
        }
      }
      strict_mcp: false
    }
  }
  projects: {
    github.com/owner/repo: [
      security
    ]
  }
}
```

| Key | Meaning |
| --- | --- |
| `global` | Profiles for `sync --global`. |
| `invocation` | `both`, `user` (slash command only) or `model` (auto-trigger only), per selector. |
| `profiles.<name>.extends` | Profiles to inherit from. Cycles are an error. |
| `profiles.<name>.skills` | Selectors: `source/name` or `source/*`. |
| `profiles.<name>.exclude` | Selectors removed after `extends` and `skills` are combined. |
| `profiles.<name>.invocation` | Overrides the top-level `invocation`. |
| `profiles.<name>.settings` | A `settings.json` fragment, passed with `--settings`. |
| `profiles.<name>.mcp_servers` | MCP server definitions, passed with `--mcp-config`. |
| `profiles.<name>.strict_mcp` | Pass `--strict-mcp-config`: only these servers load. |
| `projects` | `host/owner/repo` (exact or `host/owner/*`) to profile names. Linked worktrees match the main repository. |

Precedence for `invocation`: a profile beats the top level, a child beats the
profile it extends, an exact key beats a wildcard. Two active profiles that
disagree are an error.

Settings and MCP servers: a child overrides the profile it extends. Across
active profiles, arrays are unioned, objects merge key by key, and a key set
to different values is an error. Keep secrets as `${VAR}` references; Claude
Code expands them at launch, so generated files never contain a token.

## Behaviour to know

- Invocation rules are applied to patched copies of `SKILL.md` in the cache.
  The original skill is never modified.
- A skill name that also exists in `~/.claude/skills` is a conflict, because
  Claude Code prefers the personal skill over the project one.
- Real files, directories, and symlinks loadout did not create are never
  touched. Created links are recorded in `.claude/skills/.loadout.json`.
- Links are kept out of git with a block in the repository's
  `info/exclude`.
- Skill discovery stops at a linked worktree's root, so run `sync` in each
  worktree.
- Settings and MCP servers apply only to sessions started with `exec`.

## Layout

```
cmd/loadout        entry point
internal/cli       commands
internal/skill     keys, selectors, invocation, library scan, manifest
internal/profile   config parsing, profile resolution, settings merge
internal/plan      pure planning of link changes
internal/link      applying a plan to a skills directory
internal/patch     SKILL.md frontmatter rewrite
internal/cache     patched skill copies
internal/repo      git root, repository identity, exclude block
internal/ident     plain-name check
nix/package.nix    Nix package
```
