package core

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"
)

// urlUserinfoRE matches the userinfo portion of any URL in arbitrary text.
// Git's stderr commonly echoes partial URLs ("https://user:token@host")
// without the path, so exact-string replacement of the configured remote
// isn't sufficient — we have to find any `://userinfo@host` pattern.
var urlUserinfoRE = regexp.MustCompile(`(https?|git\+ssh|ssh)://[^@\s/]+@`)

// UpdateWatcher polls the TRaSH-Guides upstream for new commits between
// scheduled Pull runs. Detection-only — never modifies the local clone, never
// touches Arr. Surfaces "TRaSH update available" badges on rules so users can
// trigger a manual Pull (or wait for the scheduled one) when they're ready.
//
// Phase 2a (MVP) implemented here:
//   - Empty-clone safety guard (Trash.CurrentCommit() == "" → skip)
//   - git ls-remote against the configured TRaSH branch
//   - Compare local HEAD vs upstream HEAD; update UpdateWatch persistence
//
// Phase 2b will add:
//   - Detailed commit-range walk + file-to-rule mapping
//   - ExcludedCFs filter (Decision 6 + addendum G)
//   - Per-rule notification firing for auto-sync-ON rules
//   - POST /api/watch/update/refresh with rate limiting
//
// Spec: dev/docs/layout-v2/feature-watch-and-drift.md (Mode 2)
type UpdateWatcher struct {
	app *App

	// refreshMu serialises concurrent Run calls so a manual refresh + scheduled
	// tick can't race on git ls-remote. Held only for the duration of one Run.
	refreshMu sync.Mutex

	// gitLsRemote is the upstream-HEAD lookup. Pluggable so tests can inject a
	// deterministic result without spawning git processes.
	gitLsRemote func(ctx context.Context, remoteURL, branch string) (string, error)
}

// NewUpdateWatcher constructs an UpdateWatcher with the production
// git-ls-remote implementation. Tests should construct directly and override
// gitLsRemote.
func NewUpdateWatcher(app *App) *UpdateWatcher {
	return &UpdateWatcher{
		app:         app,
		gitLsRemote: gitLsRemoteHead,
	}
}

// Run performs one upstream-HEAD check. Safe to call concurrently — internal
// mutex serialises actual work.
//
// Behaviour:
//   - If Trash.CurrentCommit() == "" (no local clone yet) → record paused state,
//     return without contacting remote. Single most-important safety guard
//     per the spec — without it, an empty-clone state would surface as
//     "every commit upstream is new" and spam notifications.
//   - If UpdateWatch is disabled → return without contacting remote.
//   - Otherwise → git ls-remote, update UpdateWatch with both heads + LastRun.
//
// Always returns nil on the "no work to do" cases (empty clone, disabled);
// returns an error only on actual remote failures so callers can decide
// whether to surface them.
func (uw *UpdateWatcher) Run(ctx context.Context) error {
	uw.refreshMu.Lock()
	defer uw.refreshMu.Unlock()

	cfg := uw.app.Config.Get()

	// Empty-clone safety guard — first thing. Without this, an empty local
	// HEAD would compare against any non-empty upstream HEAD and report
	// "everything is new", spamming notifications across every rule.
	localCommit := uw.app.Trash.CurrentCommit()
	if localCommit == "" {
		log.Printf("update-watcher: TRaSH clone not initialised — skipping check (next Pull will populate it)")
		return nil
	}

	// Disabled → no remote contact.
	if cfg.UpdateWatch == nil || !cfg.UpdateWatch.Enabled {
		return nil
	}

	remote := cfg.TrashRepo.URL
	branch := cfg.TrashRepo.Branch
	if remote == "" {
		return fmt.Errorf("update-watcher: TRaSH repo URL not configured")
	}
	if branch == "" {
		branch = "master"
	}

	upstreamHead, err := uw.gitLsRemote(ctx, remote, branch)
	if err != nil {
		// Redact remote URL from the error before persisting / returning —
		// users may have credentials embedded (https://user:token@host/...).
		safeErr := fmt.Errorf("git ls-remote on branch %q failed: %w", branch, err)
		uw.recordError(localCommit, safeErr)
		return safeErr
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if updErr := uw.app.Config.Update(func(c *Config) {
		if c.UpdateWatch == nil {
			return // disabled between snapshot + apply; just drop
		}
		c.UpdateWatch.LastRun = now
		c.UpdateWatch.LocalHead = localCommit
		c.UpdateWatch.UpstreamHead = upstreamHead
		// Clear PendingChanges on Phase 2a — Phase 2b will populate it with
		// per-rule mapping. Keeping it cleared avoids stale data leaking.
		c.UpdateWatch.PendingChanges = nil
	}); updErr != nil {
		return fmt.Errorf("update-watcher: persist result: %w", updErr)
	}

	if upstreamHead != localCommit {
		log.Printf("update-watcher: TRaSH upstream ahead — local=%s upstream=%s", shortHash(localCommit), shortHash(upstreamHead))
	}
	return nil
}

// recordError persists the failure so /api/watch/update can surface it,
// without contaminating UpstreamHead with stale data. The error must already
// be URL-redacted by the caller — runErr is logged verbatim.
func (uw *UpdateWatcher) recordError(localCommit string, runErr error) {
	now := time.Now().UTC().Format(time.RFC3339)
	if err := uw.app.Config.Update(func(c *Config) {
		if c.UpdateWatch == nil {
			return
		}
		c.UpdateWatch.LastRun = now
		c.UpdateWatch.LocalHead = localCommit
		// Leave UpstreamHead untouched — previous successful value is still
		// the best signal until the next successful run.
	}); err != nil {
		log.Printf("update-watcher: persist error-state failed: %v", err)
	}
	log.Printf("update-watcher: ls-remote failed: %v", runErr)
}

// gitLsRemoteHead is the production implementation. Resolves the HEAD commit
// of `branch` on `remoteURL` via `git ls-remote` without modifying any local
// state. Context applies a hard timeout so a stuck remote can't pin the
// watcher goroutine indefinitely.
//
// Hardening:
//   - `--` separator between options and the URL so a maliciously-crafted
//     `--upload-pack=...` URL is treated as a positional arg, not a flag.
//   - GIT_TERMINAL_PROMPT=0 + GIT_ASKPASS=/bin/true so a private repo with
//     missing/wrong credentials fails fast instead of blocking on a
//     stdin-credential prompt (would hang until the context timeout).
//   - Stderr captured separately and surfaced (URL-redacted) in errors so
//     flag-injection symptoms ("unknown option --upload-pack=...") become
//     visible.
//   - All error messages route through redactGitError so credentials
//     embedded in the URL (https://user:token@host/...) never reach logs
//     or the API response.
func gitLsRemoteHead(ctx context.Context, remoteURL, branch string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// `--` separator: protects against URLs that start with `-` being
	// interpreted as git options. Without this, `--upload-pack=evil` as the
	// URL would invoke arbitrary commands on the local machine.
	cmd := exec.CommandContext(ctx, "git", "ls-remote", "--exit-code", "--", remoteURL, "refs/heads/"+branch)
	cmd.Env = append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0",     // never prompt on stdin for missing credentials
		"GIT_ASKPASS=/bin/true",     // any credential helper invocation returns empty without prompting
		"GIT_SSH_COMMAND=ssh -oBatchMode=yes -oStrictHostKeyChecking=accept-new", // SSH remotes also fail-fast
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", redactGitError(remoteURL, branch, stderr.String(), err)
	}
	// Output: "<sha>\trefs/heads/<branch>\n"
	line := strings.TrimSpace(string(out))
	if line == "" {
		return "", fmt.Errorf("git ls-remote returned empty output for branch %q", branch)
	}
	parts := strings.Fields(line)
	if len(parts) < 1 || len(parts[0]) < 7 {
		return "", fmt.Errorf("git ls-remote returned unexpected format: %q", line)
	}
	return parts[0], nil
}

// redactGitError builds an error message that includes git's stderr (for
// diagnostic value — e.g. "unknown option" surfaces flag-injection attempts,
// "authentication failed" tells the user what to fix) but strips any
// occurrence of the remote URL so embedded credentials never leak.
//
// Pass remoteURL so we can also redact userinfo from any parsed URL form
// (https://user:token@host/path → https://host/path). The redacted URL is
// returned alongside the stderr tail so logs still show *which kind* of
// remote failed without revealing credentials.
func redactGitError(remoteURL, branch, stderr string, err error) error {
	tail := strings.TrimSpace(stderr)
	if len(tail) > 240 {
		tail = tail[:240] + "..."
	}
	// Strip credentials from any URL we might emit.
	safeURL := redactURL(remoteURL)
	// Strip userinfo from any URL appearing in the stderr tail — git often
	// echoes partial URLs ("https://user:token@host" without path) on auth
	// failures, so exact-string replacement of the configured remote isn't
	// enough. The regex catches any `scheme://user@host` pattern and rewrites
	// it to `scheme://host`.
	tail = urlUserinfoRE.ReplaceAllStringFunc(tail, func(match string) string {
		// match = "https://user:token@" — keep scheme, drop userinfo + @
		schemeEnd := strings.Index(match, "://")
		return match[:schemeEnd+3]
	})
	if tail != "" {
		return fmt.Errorf("git ls-remote (%s, branch %q): %s: %w", safeURL, branch, tail, err)
	}
	return fmt.Errorf("git ls-remote (%s, branch %q): %w", safeURL, branch, err)
}

// redactURL strips userinfo (user:password) from a URL. Returns the URL
// unchanged if it doesn't parse — better to leak nothing than to leak parts.
// SSH URLs (git@host:path) have no userinfo to strip and are returned as-is.
func redactURL(raw string) string {
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.User == nil {
		return raw
	}
	u.User = nil
	return u.String()
}

// shortHash returns the first 7 chars of a commit hash for logging — enough
// for human pattern-matching without flooding the log.
func shortHash(h string) string {
	if len(h) <= 7 {
		return h
	}
	return h[:7]
}
