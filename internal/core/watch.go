package core

import (
	"context"
	"fmt"
	"log"
	"os/exec"
	"strings"
	"sync"
	"time"
)

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
		uw.recordError(localCommit, err)
		return err
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
// without contaminating UpstreamHead with stale data.
func (uw *UpdateWatcher) recordError(localCommit string, runErr error) {
	now := time.Now().UTC().Format(time.RFC3339)
	_ = uw.app.Config.Update(func(c *Config) {
		if c.UpdateWatch == nil {
			return
		}
		c.UpdateWatch.LastRun = now
		c.UpdateWatch.LocalHead = localCommit
		// Leave UpstreamHead untouched — previous successful value is still
		// the best signal until the next successful run.
	})
	log.Printf("update-watcher: ls-remote failed: %v", runErr)
}

// gitLsRemoteHead is the production implementation. Resolves the HEAD commit
// of `branch` on `remoteURL` via `git ls-remote` without modifying any local
// state. Context applies a hard timeout so a stuck remote can't pin the
// watcher goroutine indefinitely.
func gitLsRemoteHead(ctx context.Context, remoteURL, branch string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// Use --exit-code so absent ref → non-zero exit (caller surfaces as error).
	cmd := exec.CommandContext(ctx, "git", "ls-remote", "--exit-code", remoteURL, "refs/heads/"+branch)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git ls-remote %s refs/heads/%s: %w", remoteURL, branch, err)
	}
	// Output: "<sha>\trefs/heads/<branch>\n"
	line := strings.TrimSpace(string(out))
	if line == "" {
		return "", fmt.Errorf("git ls-remote returned empty output for %s/%s", remoteURL, branch)
	}
	parts := strings.Fields(line)
	if len(parts) < 1 || len(parts[0]) < 7 {
		return "", fmt.Errorf("git ls-remote returned unexpected format: %q", line)
	}
	return parts[0], nil
}

// shortHash returns the first 7 chars of a commit hash for logging — enough
// for human pattern-matching without flooding the log.
func shortHash(h string) string {
	if len(h) <= 7 {
		return h
	}
	return h[:7]
}
