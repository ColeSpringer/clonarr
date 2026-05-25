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

// ProfileSyncRunner polls the TRaSH-Guides upstream for new commits between
// scheduled Pull runs. Detection-only — never modifies the local clone, never
// touches Arr. Surfaces "TRaSH update available" badges on rules so users can
// trigger a manual Pull (or wait for the scheduled one) when they're ready.
//
// Phase 2a (MVP) implemented here:
//   - Empty-clone safety guard (Trash.CurrentCommit() == "" → skip)
//   - git ls-remote against the configured TRaSH branch
//   - Compare local HEAD vs upstream HEAD; update ProfileSync persistence
//
// Phase 2b will add:
//   - Detailed commit-range walk + file-to-rule mapping
//   - ExcludedCFs filter (Decision 6 + addendum G)
//   - Per-rule notification firing for auto-sync-ON rules
//   - POST /api/watch/update/refresh with rate limiting
//
type ProfileSyncRunner struct {
	app *App

	// refreshMu serialises concurrent Run calls so a manual refresh + scheduled
	// tick can't race on git ls-remote. Held only for the duration of one Run.
	refreshMu sync.Mutex

	// gitLsRemote is the upstream-HEAD lookup. Pluggable so tests can inject a
	// deterministic result without spawning git processes.
	gitLsRemote func(ctx context.Context, remoteURL, branch string) (string, error)
}

// NewProfileSyncRunner constructs an ProfileSyncRunner with the production
// git-ls-remote implementation. Tests should construct directly and override
// gitLsRemote.
func NewProfileSyncRunner(app *App) *ProfileSyncRunner {
	return &ProfileSyncRunner{
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
//   - If ProfileSync.Sources.TrashUpstream is false → return without contacting remote.
//   - Otherwise → git ls-remote, update ProfileSync with both heads + LastRun.
//
// Always returns nil on the "no work to do" cases (empty clone, disabled);
// returns an error only on actual remote failures so callers can decide
// whether to surface them.
func (uw *ProfileSyncRunner) Run(ctx context.Context) error {
	uw.refreshMu.Lock()
	defer uw.refreshMu.Unlock()

	cfg := uw.app.Config.Get()
	if cfg.ProfileSync == nil {
		return nil // pre-migration state; never happens after first Load()
	}

	// Sources gate — at least one detection source must be on. Manual Pull
	// (via api endpoint) bypasses this check and always does a full pull;
	// the scheduled run only acts on configured sources.
	if !cfg.ProfileSync.Sources.TrashUpstream && !cfg.ProfileSync.Sources.ArrDrift {
		return nil
	}

	// Mode dispatch. Unknown / empty Mode falls through to the
	// detection-only path so a hand-edited bad config can't accidentally
	// trigger Arr writes.
	switch cfg.ProfileSync.Mode {
	case ProfileSyncModeAuto:
		// Pull-and-sync (= today's Pull behaviour). Empty-clone is a
		// valid initial state here — Trash.CloneOrPull will populate it.
		return uw.runPullAndSync(ctx)
	default:
		// Notify-only / Delayed modes use the ls-remote-only path for
		// detection. Notification firing + per-rule mapping land in Phase C.
		return uw.runDetectionOnly(ctx)
	}
}

// runDetectionOnly performs the ls-remote-only check. Empty-clone guard
// applies — comparing against an empty local HEAD would surface every
// upstream commit as "new" once Phase C wires notification firing.
func (uw *ProfileSyncRunner) runDetectionOnly(ctx context.Context) error {
	cfg := uw.app.Config.Get()

	// Empty-clone guard — DO NOT REMOVE. Comparing an empty local HEAD
	// against any non-empty upstream HEAD would surface every TRaSH
	// commit as "new" and fire NotifyUpstreamUpdate spuriously on the
	// very first detection tick for fresh containers. The runner's
	// notification dedup (priorUpstream != upstreamHead) doesn't help
	// here because the prior value is also empty on first run.
	localCommit := uw.app.Trash.CurrentCommit()
	if localCommit == "" {
		log.Printf("profile-sync: TRaSH clone not initialised — skipping detection (next Pull will populate it)")
		return nil
	}
	if !cfg.ProfileSync.Sources.TrashUpstream {
		return nil // ArrDrift-only path lands in Phase D; nothing to do here yet
	}

	remote := cfg.TrashRepo.URL
	branch := cfg.TrashRepo.Branch
	if remote == "" {
		return fmt.Errorf("profile-sync: TRaSH repo URL not configured")
	}
	if branch == "" {
		branch = "master"
	}

	upstreamHead, err := uw.gitLsRemote(ctx, remote, branch)
	if err != nil {
		safeErr := fmt.Errorf("git ls-remote on branch %q failed: %w", branch, err)
		uw.recordError(localCommit, safeErr)
		return safeErr
	}

	// Capture the prior UpstreamHead INSIDE the Config.Update closure so
	// it reflects the actual state at write-time, not a potentially-stale
	// snapshot from cfg.Get() at the top of Run. Without this, a manual
	// Pull interleaving between our snapshot and our persist would let us
	// re-fire a notification even though Pull just synced everything.
	// (Full per-rule WatchState dedup with SHA fingerprints lands in
	// Phase C commit 2; this is the MVP-level dedup against the most
	// recent persisted UpstreamHead.)
	var priorUpstream string
	now := time.Now().UTC().Format(time.RFC3339)
	if updErr := uw.app.Config.Update(func(c *Config) {
		if c.ProfileSync == nil {
			return
		}
		// Snapshot under the same lock that's about to do the write —
		// no interleaving possible inside this closure.
		priorUpstream = c.ProfileSync.UpstreamHead
		c.ProfileSync.LastRun = now
		c.ProfileSync.LocalHead = localCommit
		c.ProfileSync.UpstreamHead = upstreamHead
	}); updErr != nil {
		return fmt.Errorf("profile-sync: persist result: %w", updErr)
	}

	// Length-normalised compare. New code stores the full 40-char hash on
	// both sides, but existing installs may have a 7-char shortened local
	// hash persisted in last-pull-diff.json from an earlier --short bug;
	// truncating the longer side to the shorter's length lets the very
	// first detection tick after upgrade compare equal instead of firing
	// a false "upstream ahead" notification. The next loadAndSwap rewrites
	// CommitHash to the full form and this fallback stops mattering.
	cmpLocal, cmpUpstream := localCommit, upstreamHead
	if len(cmpLocal) < len(cmpUpstream) {
		cmpUpstream = cmpUpstream[:len(cmpLocal)]
	} else if len(cmpUpstream) < len(cmpLocal) {
		cmpLocal = cmpLocal[:len(cmpUpstream)]
	}
	if cmpUpstream != cmpLocal {
		log.Printf("profile-sync: TRaSH upstream ahead — local=%s upstream=%s", shortHash(localCommit), shortHash(upstreamHead))
		// Phase C commit 2 — fetch commit-range to a dedicated side-ref
		// + walk the file changes + per-rule PendingChanges + per-rule
		// fingerprint dedup. Best-effort: failures in this enrichment
		// path fall back to the MVP-level aggregate notification so
		// users still hear about upstream activity.
		uw.detectPerRuleChanges(ctx, cfg, remote, branch, localCommit, upstreamHead)

		// Aggregate-level notification (the MVP path) still fires only
		// when this is a NEW upstream commit we haven't notified about.
		// Per-rule notifications inside detectPerRuleChanges have their
		// own per-rule fingerprint dedup.
		if priorUpstream != upstreamHead {
			uw.app.NotifyUpstreamUpdate(localCommit, upstreamHead)
		}
	}
	return nil
}

// detectPerRuleChanges fetches the commit-range to a side-ref, walks the
// file changes, classifies them into trash_ids, and updates per-rule
// PendingChanges + WatchState for every sync rule whose profile is
// affected. Best-effort — a failure here is logged but doesn't propagate
// (the aggregate notification still fires in the caller).
func (uw *ProfileSyncRunner) detectPerRuleChanges(ctx context.Context, cfg Config, remote, branch, localCommit, upstreamHead string) {
	if err := uw.app.Trash.FetchUpstreamRefspec(ctx, remote, branch); err != nil {
		log.Printf("profile-sync: fetch upstream side-ref failed: %v", err)
		return
	}
	changedFiles, err := uw.app.Trash.ChangedFilesSinceLocal(ctx, branch)
	if err != nil {
		log.Printf("profile-sync: walk commit range failed: %v", err)
		return
	}
	if len(changedFiles) == 0 {
		return // upstream != local but no file diffs — unusual but harmless
	}

	// Bucket changed trash_ids by app-type. Profile-eligible-CF lookup
	// per rule needs to know which app the rule's profile belongs to so
	// we only consider changes in the matching app's directory.
	changedByApp := make(map[string][]ClassifiedFile)
	for _, path := range changedFiles {
		cf := ClassifyTrashFilePath(path)
		if cf.Kind == FileChangeOther || cf.Kind == FileChangeQualitySize {
			continue // QS handled separately in Phase D; non-data files ignored
		}
		changedByApp[cf.AppType] = append(changedByApp[cf.AppType], cf)
	}
	if len(changedByApp) == 0 {
		return
	}

	// For each rule, compute the affected trash_id set + per-rule
	// fingerprint dedup. Updates land via a single Config.Update so
	// multiple rule changes batch into one disk write.
	snap := uw.app.Trash.Snapshot()
	if snap == nil {
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	perRuleUpdates := uw.buildPerRuleUpdates(cfg, snap, changedByApp, upstreamHead, now)
	if len(perRuleUpdates) == 0 {
		return
	}
	if err := uw.app.Config.Update(func(c *Config) {
		for i := range c.AutoSync.Rules {
			r := &c.AutoSync.Rules[i]
			upd, ok := perRuleUpdates[r.ID]
			if !ok {
				continue
			}
			if r.WatchState == nil {
				r.WatchState = &WatchState{}
			}
			r.WatchState.LastUpstreamFingerprint = upd.fingerprint
			if upd.shouldNotify {
				r.WatchState.LastUpstreamNotifiedAt = now
			}
			r.PendingChanges = MergePendingChanges(r.PendingChanges, upd.pending)
		}
	}); err != nil {
		log.Printf("profile-sync: persist per-rule pending failed: %v", err)
	}
	log.Printf("profile-sync: per-rule mapping updated %d rule(s)", len(perRuleUpdates))
}

// perRuleUpdate is the planned mutation for one rule, computed outside the
// Config.Update closure so the closure stays trivially short.
type perRuleUpdate struct {
	fingerprint  string
	shouldNotify bool
	pending      []PendingChange
}

func (uw *ProfileSyncRunner) buildPerRuleUpdates(cfg Config, snap *TrashData, changedByApp map[string][]ClassifiedFile, upstreamHead, now string) map[string]perRuleUpdate {
	out := make(map[string]perRuleUpdate)
	for _, rule := range cfg.AutoSync.Rules {
		if rule.OrphanedAt != "" || !rule.Enabled {
			continue
		}
		var inst Instance
		var found bool
		for _, i := range cfg.Instances {
			if i.ID == rule.InstanceID {
				inst, found = i, true
				break
			}
		}
		if !found {
			continue
		}
		changedForApp := changedByApp[inst.Type]
		if len(changedForApp) == 0 {
			continue
		}
		var ad *AppData
		switch inst.Type {
		case "radarr":
			ad = &snap.Radarr
		case "sonarr":
			ad = &snap.Sonarr
		default:
			continue
		}
		var profile *TrashQualityProfile
		for _, p := range ad.Profiles {
			if p != nil && p.TrashID == rule.TrashProfileID {
				profile = p
				break
			}
		}
		if profile == nil {
			continue
		}

		changedTrashIDs := make([]string, 0, len(changedForApp))
		for _, cf := range changedForApp {
			if cf.Kind == FileChangeCF {
				changedTrashIDs = append(changedTrashIDs, cf.TrashID)
			}
		}
		affected := RuleAffectedTrashIDs(&rule, profile, ad, changedTrashIDs)
		if len(affected) == 0 {
			continue
		}

		fp := ComputeUpstreamFingerprint(affected)
		var prior string
		if rule.WatchState != nil {
			prior = rule.WatchState.LastUpstreamFingerprint
		}
		shouldNotify := fp != prior

		pending := make([]PendingChange, 0, len(affected))
		for _, tid := range affected {
			pending = append(pending, PendingChange{
				Source:     "trash",
				DetectedAt: now,
				CommitHash: upstreamHead,
				ChangeType: "cf-modified",
				AffectedID: tid,
			})
		}
		out[rule.ID] = perRuleUpdate{
			fingerprint:  fp,
			shouldNotify: shouldNotify,
			pending:      pending,
		}
	}
	return out
}

// runPullAndSync dispatches to the canonical App.RunPullAndSync flow with
// the scheduled-pull source tag. All telemetry (DebugLog op-trace,
// SetPullError, ProfileSync state persistence, DiffPull detail lines,
// AfterPullCallback, AutoSyncAfterPull) is unified there — manual Pull
// (handleTrashPull) goes through the same method.
func (uw *ProfileSyncRunner) runPullAndSync(_ context.Context) error {
	log.Printf("profile-sync: scheduled pull-and-sync starting (mode=auto)")
	return uw.app.RunPullAndSync(SourceAutoPullInterval)
}

// recordError persists the failure so /api/watch/update can surface it,
// without contaminating UpstreamHead with stale data. The error must already
// be URL-redacted by the caller — runErr is logged verbatim.
func (uw *ProfileSyncRunner) recordError(localCommit string, runErr error) {
	now := time.Now().UTC().Format(time.RFC3339)
	if err := uw.app.Config.Update(func(c *Config) {
		if c.ProfileSync == nil {
			return
		}
		c.ProfileSync.LastRun = now
		c.ProfileSync.LocalHead = localCommit
		// Leave UpstreamHead untouched — previous successful value is still
		// the best signal until the next successful run.
	}); err != nil {
		log.Printf("profile-sync: persist error-state failed: %v", err)
	}
	log.Printf("profile-sync: ls-remote failed: %v", runErr)
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
