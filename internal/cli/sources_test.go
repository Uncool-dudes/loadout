package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Uncool-dudes/loadout/internal/source"
)

const (
	revA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	revB = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	hash = "sha256-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="
)

// fakeFetcher resolves every ref to rev and "downloads" into store.
type fakeFetcher struct {
	rev      string
	store    string
	resolved []string
	err      error
}

func (f *fakeFetcher) Resolve(_ context.Context, owner, repo, ref string) (string, error) {
	f.resolved = append(f.resolved, owner+"/"+repo+"@"+ref)
	return f.rev, f.err
}

func (f *fakeFetcher) Prefetch(_ context.Context, _, _, rev string) (string, string, error) {
	if rev == revB {
		return "sha256-BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB=", f.store, f.err
	}
	return hash, f.store, f.err
}

// sourcesEnv is a fake repository of skills plus a sources.json location.
type sourcesEnv struct {
	file    string
	fetcher *fakeFetcher
}

func newSourcesEnv(t *testing.T, skills ...string) sourcesEnv {
	t.Helper()
	store := t.TempDir()
	for _, s := range skills {
		dir := filepath.Join(store, s)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		body := "---\nname: " + filepath.Base(s) + "\ndescription: d\n---\n"
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	e := sourcesEnv{file: filepath.Join(t.TempDir(), "sources.json"), fetcher: &fakeFetcher{rev: revA, store: store}}
	old := newFetcher
	newFetcher = func() source.Fetcher { return e.fetcher }
	t.Cleanup(func() { newFetcher = old })
	return e
}

func (e sourcesEnv) run(t *testing.T, args ...string) (string, error) {
	t.Helper()
	return run(t, append(args, "--sources", e.file)...)
}

func (e sourcesEnv) read(t *testing.T) source.Set {
	t.Helper()
	data, err := os.ReadFile(e.file)
	if err != nil {
		t.Fatal(err)
	}
	set, err := source.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	return set
}

func TestAddDetectsLayoutAndWrites(t *testing.T) {
	e := newSourcesEnv(t, "plugins/a/skills/x", "plugins/b/skills/y")
	out, err := e.run(t, "add", "acme/skills@main")
	if err != nil {
		t.Fatal(err)
	}
	src, ok := e.read(t).Get("acme")
	if !ok {
		t.Fatal("source acme was not written")
	}
	if src.Owner != "acme" || src.Repo != "skills" || src.Ref != "main" || src.Rev != revA || src.Path != "plugins" || src.Hash != hash {
		t.Errorf("source = %+v", src)
	}
	for _, want := range []string{"2 skill(s) under plugins/", "added acme", "select skills as acme/<skill>"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if got := e.fetcher.resolved[0]; got != "acme/skills@main" {
		t.Errorf("resolved %q", got)
	}
}

func TestAddOptions(t *testing.T) {
	t.Run("custom name and github prefix", func(t *testing.T) {
		e := newSourcesEnv(t, "skills/foo")
		if _, err := e.run(t, "add", "github:acme/tools", "--name", "tools"); err != nil {
			t.Fatal(err)
		}
		if _, ok := e.read(t).Get("tools"); !ok {
			t.Error("source tools missing")
		}
	})
	t.Run("explicit path is checked", func(t *testing.T) {
		e := newSourcesEnv(t, "skills/foo")
		if _, err := e.run(t, "add", "acme/tools", "--path", "nope"); err == nil || !strings.Contains(err.Error(), "does not exist") {
			t.Fatalf("error = %v", err)
		}
		if _, err := e.run(t, "add", "acme/tools", "--path", "skills"); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("no library skips detection", func(t *testing.T) {
		e := newSourcesEnv(t) // no skills at all
		if _, err := e.run(t, "add", "acme/plugin", "--no-library", "--pinned"); err != nil {
			t.Fatal(err)
		}
		src, _ := e.read(t).Get("acme")
		if src.InLibrary() || !src.Pinned {
			t.Errorf("source = %+v", src)
		}
	})
	t.Run("dry run writes nothing", func(t *testing.T) {
		e := newSourcesEnv(t, "skills/foo")
		out, err := e.run(t, "add", "acme/tools", "--dry-run")
		if err != nil || !strings.Contains(out, "would add acme") {
			t.Fatalf("out = %q, err = %v", out, err)
		}
		if _, err := os.Stat(e.file); !os.IsNotExist(err) {
			t.Errorf("dry run wrote sources.json: %v", err)
		}
	})
}

func TestAddErrors(t *testing.T) {
	t.Run("name taken", func(t *testing.T) {
		e := newSourcesEnv(t, "skills/foo")
		if _, err := e.run(t, "add", "acme/one"); err != nil {
			t.Fatal(err)
		}
		_, err := e.run(t, "add", "acme/two")
		if err == nil || !strings.Contains(err.Error(), "already exists") {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("bad spec", func(t *testing.T) {
		e := newSourcesEnv(t)
		if _, err := e.run(t, "add", "justaname"); err == nil {
			t.Fatal("want an error")
		}
	})
	t.Run("no skills found", func(t *testing.T) {
		e := newSourcesEnv(t)
		_, err := e.run(t, "add", "acme/empty")
		if err == nil || !strings.Contains(err.Error(), "--no-library") {
			t.Fatalf("error = %v, want a hint about --no-library", err)
		}
		if _, statErr := os.Stat(e.file); !os.IsNotExist(statErr) {
			t.Errorf("a failed add wrote sources.json: %v", statErr)
		}
	})
	t.Run("network failure", func(t *testing.T) {
		e := newSourcesEnv(t, "skills/foo")
		e.fetcher.err = errors.New("offline")
		if _, err := e.run(t, "add", "acme/tools"); err == nil || !strings.Contains(err.Error(), "offline") {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestUpdate(t *testing.T) {
	e := newSourcesEnv(t, "skills/foo")
	for _, args := range [][]string{
		{"add", "acme/moves"},
		{"add", "acme/fixed", "--name", "fixed", "--pinned"},
		{"add", "acme/same", "--name", "same"},
	} {
		if _, err := e.run(t, args...); err != nil {
			t.Fatal(err)
		}
	}

	// Everything moved upstream except "same", which we pin back below.
	e.fetcher.rev = revB
	out, err := e.run(t, "update")
	if err != nil {
		t.Fatal(err)
	}
	set := e.read(t)
	if src, _ := set.Get("acme"); src.Rev != revB || !strings.HasPrefix(src.Hash, "sha256-BBBB") {
		t.Errorf("acme = %+v, want the new rev and hash", src)
	}
	if src, _ := set.Get("fixed"); src.Rev != revA {
		t.Errorf("a pinned source moved: %+v", src)
	}
	if !strings.Contains(out, "fixed") || !strings.Contains(out, "pinned") {
		t.Errorf("output does not mention the pinned source:\n%s", out)
	}

	// Naming a pinned source moves it anyway.
	if _, err := e.run(t, "update", "fixed"); err != nil {
		t.Fatal(err)
	}
	if src, _ := e.read(t).Get("fixed"); src.Rev != revB {
		t.Errorf("explicit update left rev %s", src.Rev)
	}

	// Nothing new upstream: unchanged, file untouched.
	before, _ := os.ReadFile(e.file)
	out, err = e.run(t, "update")
	if err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(e.file)
	if string(before) != string(after) || !strings.Contains(out, "unchanged") {
		t.Errorf("a no-op update changed the file or output:\n%s", out)
	}

	if _, err := e.run(t, "update", "ghost"); err == nil {
		t.Error("update accepted an unknown name")
	}
}

func TestUpdateDryRun(t *testing.T) {
	e := newSourcesEnv(t, "skills/foo")
	if _, err := e.run(t, "add", "acme/tools"); err != nil {
		t.Fatal(err)
	}
	e.fetcher.rev = revB
	if _, err := e.run(t, "update", "--dry-run"); err != nil {
		t.Fatal(err)
	}
	if src, _ := e.read(t).Get("acme"); src.Rev != revA {
		t.Errorf("dry run moved the rev to %s", src.Rev)
	}
}

func TestRemoveAndList(t *testing.T) {
	e := newSourcesEnv(t, "skills/foo")
	if _, err := e.run(t, "add", "acme/tools", "--pinned"); err != nil {
		t.Fatal(err)
	}
	out, err := e.run(t, "sources")
	if err != nil || !strings.Contains(out, "acme/tools") || !strings.Contains(out, "pinned") {
		t.Fatalf("sources = %q, %v", out, err)
	}
	if _, err := e.run(t, "remove", "acme"); err != nil {
		t.Fatal(err)
	}
	if names := e.read(t).Names(); len(names) != 0 {
		t.Errorf("sources left: %v", names)
	}
	if _, err := e.run(t, "remove", "acme"); err == nil {
		t.Error("removing twice succeeded")
	}
}

func TestSourcesPathFollowsConfigLink(t *testing.T) {
	repo := t.TempDir()
	real := filepath.Join(repo, "loadout.hjson")
	if err := os.WriteFile(real, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "loadout.hjson")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	e := newSourcesEnv(t, "skills/foo")
	t.Setenv("LOADOUT_SOURCES", "")
	if _, err := run(t, "add", "acme/tools", "--config", link); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(repo, "sources.json")); err != nil {
		t.Errorf("sources.json not written next to the real config: %v", err)
	}
	_ = e
}
