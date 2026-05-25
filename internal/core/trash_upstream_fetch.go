package core

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// upstreamWatchRefPrefix is the side-ref namespace Profile Sync uses to
// stash upstream commits without touching the user's master branch. Keeps
// `git pull` and Reset TRaSH Data working normally — they only care about
// refs/heads/*, never refs/upstream-watch/*.
const upstreamWatchRefPrefix = "refs/upstream-watch/"

// FetchUpstreamRefspec runs `git fetch` from the configured remote into a
// dedicated side-ref so the detection-only path can walk the commit range
// without mutating any branch tracked by other code paths (CloneOrPull,
// Reset, manual `git pull`).
//
// Fetched commits land in `.git/objects/`; the side-ref names the upstream
// tip. The user's working tree and refs/heads/<branch> are untouched.
//
// Hardened against credential-leak + flag-injection the same way
// gitLsRemoteHead is (see watch.go) — credentials from a custom remote
// URL never reach error messages or container logs.
func (ts *TrashStore) FetchUpstreamRefspec(ctx context.Context, remoteURL, branch string) error {
	ts.repoMu.RLock()
	dataDir := ts.dataDir
	ts.repoMu.RUnlock()
	if dataDir == "" {
		return fmt.Errorf("trash store data dir not set")
	}

	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	// refspec: pull `branch` into our side-ref so we don't disturb refs/heads/<branch>
	refspec := fmt.Sprintf("+%s:%s%s", branch, upstreamWatchRefPrefix, branch)
	cmd := exec.CommandContext(ctx, "git", "-C", dataDir, "fetch",
		"--no-write-fetch-head", // don't update FETCH_HEAD; we use the side-ref
		"--no-tags",             // tags aren't relevant; skip to save objects
		"--", remoteURL, refspec,
	)
	cmd.Env = append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0",
		"GIT_ASKPASS=/bin/true",
		"GIT_SSH_COMMAND=ssh -oBatchMode=yes -oStrictHostKeyChecking=accept-new",
	)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return redactGitError(remoteURL, branch, stderr.String(), err)
	}
	return nil
}

// ChangedFilesSinceLocal returns the list of file paths changed in commits
// reachable from the upstream side-ref but not from the local branch HEAD.
// One entry per file regardless of how many commits touched it (the
// detection layer doesn't care about per-commit attribution — it cares
// about "what's the union of changes I missed since my last pull").
//
// Caller must have called FetchUpstreamRefspec first to populate the
// side-ref. Returns empty slice when the side-ref doesn't exist (treat
// as "no changes detected" rather than erroring).
func (ts *TrashStore) ChangedFilesSinceLocal(ctx context.Context, branch string) ([]string, error) {
	ts.repoMu.RLock()
	dataDir := ts.dataDir
	ts.repoMu.RUnlock()
	if dataDir == "" {
		return nil, fmt.Errorf("trash store data dir not set")
	}

	sideRef := upstreamWatchRefPrefix + branch
	localRef := "refs/heads/" + branch

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// `git log local..sideRef --name-only --pretty=format:` returns each
	// changed file once per commit. Pipe through sort -u-style dedup in
	// Go since we don't need shell.
	cmd := exec.CommandContext(ctx, "git", "-C", dataDir, "log",
		"--name-only", "--pretty=format:",
		"--", // separator; nothing after this means "all paths"
	)
	// Insert the rev-range before the `--` separator. Building the args
	// slice explicitly avoids quoting issues.
	cmd.Args = []string{"git", "-C", dataDir, "log",
		localRef + ".." + sideRef,
		"--name-only", "--pretty=format:",
	}

	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		// Common case: side-ref doesn't exist yet (FetchUpstreamRefspec
		// hasn't been called or failed). Treat as "no changes" rather
		// than surfacing a misleading error to the watcher loop.
		if strings.Contains(stderr.String(), "unknown revision") ||
			strings.Contains(stderr.String(), "bad revision") {
			return nil, nil
		}
		return nil, fmt.Errorf("git log %s..%s: %w (stderr: %s)", localRef, sideRef, err, strings.TrimSpace(stderr.String()))
	}

	seen := make(map[string]bool)
	out_list := make([]string, 0)
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || seen[line] {
			continue
		}
		seen[line] = true
		out_list = append(out_list, line)
	}
	return out_list, nil
}
