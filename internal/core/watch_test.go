package core

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

// newWatcherTestApp builds an App backed by a temp ConfigStore + TrashStore.
// The TrashStore is initialised but its repo is empty — tests that need a
// populated commit must call seedTrashCommit on the returned store.
func newWatcherTestApp(t *testing.T) (*App, *TrashStore, string) {
	t.Helper()
	dir := t.TempDir()
	cs := NewConfigStore(dir)
	if err := cs.Update(func(cfg *Config) {
		cfg.TrashRepo = TrashRepo{
			URL:    "https://example.invalid/repo.git",
			Branch: "master",
		}
	}); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	ts := NewTrashStore(filepath.Join(dir, "trash"))
	app := &App{
		Config:     cs,
		Trash:      ts,
		HTTPClient: &http.Client{Timeout: 5 * time.Second},
	}
	return app, ts, dir
}

// seedTrashCommit sets a non-empty commit hash on the TrashStore so the
// empty-clone guard treats it as initialised. Side-steps a real git clone.
func seedTrashCommit(t *testing.T, ts *TrashStore, commit string) {
	t.Helper()
	ts.mu.Lock()
	ts.data.CommitHash = commit
	ts.mu.Unlock()
}

// TestUpdateWatcher_SkipsWhenTrashCloneEmpty is the most important safety
// test in Phase 2a. Without the guard, an empty local HEAD compared against
// any non-empty upstream would surface as "everything is new" and spam
// notifications across every rule (once Phase 2b wires firing).
func TestUpdateWatcher_SkipsWhenTrashCloneEmpty(t *testing.T) {
	app, _, _ := newWatcherTestApp(t)
	// CommitHash left empty — Trash clone "not initialised".

	uw := &UpdateWatcher{
		app: app,
		gitLsRemote: func(ctx context.Context, remoteURL, branch string) (string, error) {
			t.Fatal("ls-remote should NOT be called when local clone is empty")
			return "", nil
		},
	}
	if err := uw.Run(context.Background()); err != nil {
		t.Errorf("Run() with empty clone should return nil, got %v", err)
	}
	// UpdateWatch state must remain untouched (nil-default).
	if got := app.Config.Get().UpdateWatch; got != nil {
		t.Errorf("UpdateWatch should stay nil when guard fires, got %+v", got)
	}
}

// TestUpdateWatcher_NoRemoteCallWhenDisabled verifies the Enabled flag
// short-circuits before any remote contact. UpdateWatch must exist (so the
// flag is reachable) — only ls-remote is skipped.
func TestUpdateWatcher_NoRemoteCallWhenDisabled(t *testing.T) {
	app, ts, _ := newWatcherTestApp(t)
	seedTrashCommit(t, ts, "local-head-abcdef0")
	_ = app.Config.Update(func(c *Config) {
		c.UpdateWatch = &UpdateWatch{Enabled: false}
	})

	var called int32
	uw := &UpdateWatcher{
		app: app,
		gitLsRemote: func(ctx context.Context, remoteURL, branch string) (string, error) {
			atomic.AddInt32(&called, 1)
			return "should-not-happen", nil
		},
	}
	if err := uw.Run(context.Background()); err != nil {
		t.Errorf("Run() disabled should return nil, got %v", err)
	}
	if atomic.LoadInt32(&called) != 0 {
		t.Error("ls-remote called even though Enabled=false")
	}
}

// TestUpdateWatcher_PersistsHeadsOnSuccessfulCheck verifies the happy path:
// guard passes, watcher enabled, ls-remote returns a fresh upstream head →
// LastRun, LocalHead, UpstreamHead all populated and PendingChanges cleared.
func TestUpdateWatcher_PersistsHeadsOnSuccessfulCheck(t *testing.T) {
	app, ts, _ := newWatcherTestApp(t)
	seedTrashCommit(t, ts, "local-commit-1111111")
	_ = app.Config.Update(func(c *Config) {
		c.UpdateWatch = &UpdateWatch{
			Enabled: true,
			// Stale PendingChanges from a prior tick — Phase 2a clears these
			// since per-rule mapping doesn't ship until Phase 2b.
			PendingChanges: []ChangeSummary{{Type: "cf", TrashID: "stale", Name: "stale"}},
		}
	})

	uw := &UpdateWatcher{
		app: app,
		gitLsRemote: func(ctx context.Context, remoteURL, branch string) (string, error) {
			if remoteURL != "https://example.invalid/repo.git" {
				t.Errorf("unexpected remoteURL: %s", remoteURL)
			}
			if branch != "master" {
				t.Errorf("unexpected branch: %s", branch)
			}
			return "upstream-commit-2222222", nil
		},
	}
	if err := uw.Run(context.Background()); err != nil {
		t.Fatalf("Run(): %v", err)
	}

	got := app.Config.Get().UpdateWatch
	if got == nil {
		t.Fatal("UpdateWatch missing after Run")
	}
	if got.LocalHead != "local-commit-1111111" {
		t.Errorf("LocalHead = %q, want local-commit-1111111", got.LocalHead)
	}
	if got.UpstreamHead != "upstream-commit-2222222" {
		t.Errorf("UpstreamHead = %q, want upstream-commit-2222222", got.UpstreamHead)
	}
	if got.LastRun == "" {
		t.Error("LastRun not set")
	}
	if _, err := time.Parse(time.RFC3339, got.LastRun); err != nil {
		t.Errorf("LastRun not RFC3339: %q (%v)", got.LastRun, err)
	}
	if len(got.PendingChanges) != 0 {
		t.Errorf("PendingChanges should be cleared on Phase 2a, got %d", len(got.PendingChanges))
	}
}

// TestUpdateWatcher_LsRemoteErrorPreservesPriorUpstreamHead verifies a
// transient remote failure doesn't wipe the last-known-good UpstreamHead.
// LastRun still bumps so the UI can show "last attempt: 2m ago"; UpstreamHead
// stays at the prior value so the "X commits ahead" badge isn't lost.
func TestUpdateWatcher_LsRemoteErrorPreservesPriorUpstreamHead(t *testing.T) {
	app, ts, _ := newWatcherTestApp(t)
	seedTrashCommit(t, ts, "local-aaaaaaa")
	_ = app.Config.Update(func(c *Config) {
		c.UpdateWatch = &UpdateWatch{
			Enabled:      true,
			LocalHead:    "stale-local",
			UpstreamHead: "prior-good-upstream",
			LastRun:      "2026-01-01T00:00:00Z",
		}
	})

	uw := &UpdateWatcher{
		app: app,
		gitLsRemote: func(ctx context.Context, remoteURL, branch string) (string, error) {
			return "", fmt.Errorf("remote unreachable")
		},
	}
	err := uw.Run(context.Background())
	if err == nil {
		t.Error("Run() should propagate ls-remote error")
	}

	got := app.Config.Get().UpdateWatch
	if got == nil {
		t.Fatal("UpdateWatch lost")
	}
	if got.UpstreamHead != "prior-good-upstream" {
		t.Errorf("UpstreamHead should be preserved on error, got %q", got.UpstreamHead)
	}
	if got.LocalHead != "local-aaaaaaa" {
		t.Errorf("LocalHead should still update to current local commit, got %q", got.LocalHead)
	}
	if got.LastRun == "2026-01-01T00:00:00Z" {
		t.Error("LastRun should bump even on error (attempt-recording semantics)")
	}
}

// TestUpdateWatcher_MissingRepoURL surfaces config errors as Run errors
// rather than silently no-oping — operators need to see this.
func TestUpdateWatcher_MissingRepoURL(t *testing.T) {
	app, ts, _ := newWatcherTestApp(t)
	seedTrashCommit(t, ts, "local-head-xyz1234")
	_ = app.Config.Update(func(c *Config) {
		c.TrashRepo.URL = ""
		c.UpdateWatch = &UpdateWatch{Enabled: true}
	})

	uw := &UpdateWatcher{
		app: app,
		gitLsRemote: func(ctx context.Context, remoteURL, branch string) (string, error) {
			t.Fatal("ls-remote should not be called with empty URL")
			return "", nil
		},
	}
	if err := uw.Run(context.Background()); err == nil {
		t.Error("Run() should error when TRaSH repo URL is empty")
	}
}
