package source

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// Fetcher is the network side of adding a source. The commands depend on this
// interface, so their tests never reach GitHub.
type Fetcher interface {
	// Resolve turns a branch, tag or empty ref (the default branch) into a
	// commit hash.
	Resolve(ctx context.Context, owner, repo, ref string) (string, error)
	// Prefetch downloads the revision into the Nix store and returns the hash
	// fetchFromGitHub expects, plus where it landed.
	Prefetch(ctx context.Context, owner, repo, rev string) (hash, storePath string, err error)
}

// Git is the real Fetcher: git ls-remote and nix flake prefetch.
type Git struct{}

func (Git) Resolve(ctx context.Context, owner, repo, ref string) (string, error) {
	if revPattern.MatchString(ref) {
		return ref, nil
	}
	patterns := []string{"HEAD"}
	if ref != "" {
		patterns = []string{"refs/heads/" + ref, "refs/tags/" + ref + "^{}", "refs/tags/" + ref}
	}
	args := append([]string{"ls-remote", fmt.Sprintf("https://github.com/%s/%s", owner, repo)}, patterns...)
	out, err := run(ctx, "git", args...)
	if err != nil {
		return "", err
	}
	return pickRev(out, patterns)
}

func (Git) Prefetch(ctx context.Context, owner, repo, rev string) (string, string, error) {
	out, err := run(ctx, "nix", "flake", "prefetch", "--json", fmt.Sprintf("github:%s/%s/%s", owner, repo, rev))
	if err != nil {
		return "", "", err
	}
	var res struct {
		Hash      string `json:"hash"`
		StorePath string `json:"storePath"`
	}
	if err := json.Unmarshal([]byte(out), &res); err != nil || res.Hash == "" || res.StorePath == "" {
		return "", "", fmt.Errorf("unexpected nix flake prefetch output: %.200s", out)
	}
	return res.Hash, res.StorePath, nil
}

// pickRev chooses the commit for the first pattern that matched, so a branch
// beats a tag of the same name and an annotated tag resolves to its commit.
func pickRev(lsRemote string, patterns []string) (string, error) {
	found := map[string]string{}
	for _, line := range strings.Split(lsRemote, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && revPattern.MatchString(fields[0]) {
			found[fields[1]] = fields[0]
		}
	}
	for _, p := range patterns {
		if rev, ok := found[p]; ok {
			return rev, nil
		}
	}
	return "", errors.New("ref not found on the remote")
}

func run(ctx context.Context, name string, args ...string) (string, error) {
	out, err := exec.CommandContext(ctx, name, args...).Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return "", fmt.Errorf("%s %s: %s", name, strings.Join(args[:min(len(args), 2)], " "), strings.TrimSpace(string(exitErr.Stderr)))
		}
		return "", fmt.Errorf("%s: %w", name, err)
	}
	return string(out), nil
}
