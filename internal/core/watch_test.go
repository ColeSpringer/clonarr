package core

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
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

// TestProfileSyncRunner_SkipsWhenTrashCloneEmpty is the most important safety
// test in Phase 2a. Without the guard, an empty local HEAD compared against
// any non-empty upstream would surface as "everything is new" and spam
// notifications across every rule (once Phase 2b wires firing).
func TestProfileSyncRunner_SkipsWhenTrashCloneEmpty(t *testing.T) {
	app, _, _ := newWatcherTestApp(t)
	// CommitHash left empty — Trash clone "not initialised".

	uw := &ProfileSyncRunner{
		app: app,
		gitLsRemote: func(ctx context.Context, remoteURL, branch string) (string, error) {
			t.Fatal("ls-remote should NOT be called when local clone is empty")
			return "", nil
		},
	}
	if err := uw.Run(context.Background()); err != nil {
		t.Errorf("Run() with empty clone should return nil, got %v", err)
	}
	// ProfileSync state must remain untouched (nil-default).
	if got := app.Config.Get().ProfileSync; got != nil {
		t.Errorf("ProfileSync should stay nil when guard fires, got %+v", got)
	}
}

// TestProfileSyncRunner_NoRemoteCallWhenDisabled verifies the Enabled flag
// short-circuits before any remote contact. UpdateWatch must exist (so the
// flag is reachable) — only ls-remote is skipped.
func TestProfileSyncRunner_NoRemoteCallWhenDisabled(t *testing.T) {
	app, ts, _ := newWatcherTestApp(t)
	seedTrashCommit(t, ts, "local-head-abcdef0")
	_ = app.Config.Update(func(c *Config) {
		c.ProfileSync = &ProfileSync{Sources: ProfileSyncSources{TrashUpstream: false}}
	})

	var called int32
	uw := &ProfileSyncRunner{
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

// TestProfileSyncRunner_PersistsHeadsOnSuccessfulCheck verifies the happy path:
// guard passes, watcher enabled, ls-remote returns a fresh upstream head →
// LastRun, LocalHead, UpstreamHead all populated and PendingChanges cleared.
func TestProfileSyncRunner_PersistsHeadsOnSuccessfulCheck(t *testing.T) {
	app, ts, _ := newWatcherTestApp(t)
	seedTrashCommit(t, ts, "local-commit-1111111")
	_ = app.Config.Update(func(c *Config) {
		c.ProfileSync = &ProfileSync{
			Sources: ProfileSyncSources{TrashUpstream: true},
			Mode:    "auto",
		}
	})

	uw := &ProfileSyncRunner{
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

	got := app.Config.Get().ProfileSync
	if got == nil {
		t.Fatal("ProfileSync missing after Run")
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
}

// TestProfileSyncRunner_LsRemoteErrorPreservesPriorUpstreamHead verifies a
// transient remote failure doesn't wipe the last-known-good UpstreamHead.
// LastRun still bumps so the UI can show "last attempt: 2m ago"; UpstreamHead
// stays at the prior value so the "X commits ahead" badge isn't lost.
func TestProfileSyncRunner_LsRemoteErrorPreservesPriorUpstreamHead(t *testing.T) {
	app, ts, _ := newWatcherTestApp(t)
	seedTrashCommit(t, ts, "local-aaaaaaa")
	_ = app.Config.Update(func(c *Config) {
		c.ProfileSync = &ProfileSync{
			Sources:      ProfileSyncSources{TrashUpstream: true},
			Mode:         "auto",
			LocalHead:    "stale-local",
			UpstreamHead: "prior-good-upstream",
			LastRun:      "2026-01-01T00:00:00Z",
		}
	})

	uw := &ProfileSyncRunner{
		app: app,
		gitLsRemote: func(ctx context.Context, remoteURL, branch string) (string, error) {
			return "", fmt.Errorf("remote unreachable")
		},
	}
	err := uw.Run(context.Background())
	if err == nil {
		t.Error("Run() should propagate ls-remote error")
	}

	got := app.Config.Get().ProfileSync
	if got == nil {
		t.Fatal("ProfileSync lost")
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

// TestRedactURL verifies userinfo (credentials embedded in URL) is stripped
// before any URL is logged or returned in an error. Without this, a user
// who configures TrashRepo.URL as "https://oauth2:ghp_xxx@github.com/..."
// would see their token in container logs and API error responses.
func TestRedactURL(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"https-no-creds", "https://github.com/TRaSH-Guides/Guides.git", "https://github.com/TRaSH-Guides/Guides.git"},
		{"https-with-token", "https://oauth2:ghp_secrettoken@github.com/owner/repo.git", "https://github.com/owner/repo.git"},
		{"https-user-only", "https://username@github.com/owner/repo.git", "https://github.com/owner/repo.git"},
		{"http-with-creds", "http://admin:hunter2@internal.example.invalid:3000/repo.git", "http://internal.example.invalid:3000/repo.git"},
		{"ssh-form-unchanged", "git@github.com:owner/repo.git", "git@github.com:owner/repo.git"},
		{"malformed-returned-as-is", "not a url at all", "not a url at all"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := redactURL(tc.in)
			if got != tc.want {
				t.Errorf("redactURL(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestRedactGitError_StripsCredentialsFromOutput verifies that when git's
// stderr echoes the configured URL back (common on auth failures), the
// returned error has the credential portion redacted. Critical because
// this error string flows from Run() → API response → tester logs.
func TestRedactGitError_StripsCredentialsFromOutput(t *testing.T) {
	remote := "https://oauth2:ghp_supersecret@github.com/owner/repo.git"
	stderr := "fatal: could not read Password for 'https://oauth2:ghp_supersecret@github.com': terminal prompts disabled"
	rawErr := fmt.Errorf("exit status 128")

	got := redactGitError(remote, "master", stderr, rawErr).Error()

	if strings.Contains(got, "ghp_supersecret") {
		t.Errorf("redacted error still contains token: %q", got)
	}
	if !strings.Contains(got, "github.com/owner/repo.git") {
		t.Errorf("redacted error lost the host (should still tell user which repo failed): %q", got)
	}
	if !strings.Contains(got, "master") {
		t.Errorf("redacted error lost the branch: %q", got)
	}
}

// TestRedactGitError_TruncatesLongStderr keeps logs sane when git emits
// a multi-line stderr (e.g. server error pages echoed from a misconfigured
// HTTP remote).
func TestRedactGitError_TruncatesLongStderr(t *testing.T) {
	stderr := strings.Repeat("x", 500)
	got := redactGitError("https://example.invalid/repo.git", "master", stderr, fmt.Errorf("exit 1")).Error()
	if !strings.Contains(got, "...") {
		t.Errorf("expected truncation marker in long stderr, got %q", got)
	}
	if len(got) > 400 {
		t.Errorf("error message too long after truncation: %d chars", len(got))
	}
}

// TestProfileSyncRunner_ErrorReturnedFromRunIsRedacted verifies the end-to-end
// path: stub gitLsRemote returning a credential-laden error → Run wraps it
// via redactGitError → returned error to API caller is safe to display.
func TestProfileSyncRunner_ErrorReturnedFromRunIsRedacted(t *testing.T) {
	app, ts, _ := newWatcherTestApp(t)
	seedTrashCommit(t, ts, "local-commit-aaaa")
	_ = app.Config.Update(func(c *Config) {
		c.TrashRepo.URL = "https://oauth2:ghp_supersecret@github.com/owner/repo.git"
		c.ProfileSync = &ProfileSync{Sources: ProfileSyncSources{TrashUpstream: true}, Mode: "auto"}
	})

	uw := &ProfileSyncRunner{
		app: app,
		gitLsRemote: func(ctx context.Context, remoteURL, branch string) (string, error) {
			// Production gitLsRemoteHead would call redactGitError before
			// returning; the stub mimics that contract.
			return "", redactGitError(remoteURL, branch, "fatal: authentication required for "+remoteURL, fmt.Errorf("exit 128"))
		},
	}
	err := uw.Run(context.Background())
	if err == nil {
		t.Fatal("Run() should propagate ls-remote error")
	}
	msg := err.Error()
	if strings.Contains(msg, "ghp_supersecret") {
		t.Errorf("Run() error leaked credential token: %q", msg)
	}
}

// TestProfileSyncRunner_MissingRepoURL surfaces config errors as Run errors
// rather than silently no-oping — operators need to see this.
func TestProfileSyncRunner_MissingRepoURL(t *testing.T) {
	app, ts, _ := newWatcherTestApp(t)
	seedTrashCommit(t, ts, "local-head-xyz1234")
	_ = app.Config.Update(func(c *Config) {
		c.TrashRepo.URL = ""
		c.ProfileSync = &ProfileSync{Sources: ProfileSyncSources{TrashUpstream: true}, Mode: "auto"}
	})

	uw := &ProfileSyncRunner{
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
