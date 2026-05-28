package core

import (
	"fmt"
	"strings"
	"time"
)

// RunDelayedApply is the "Wait before applying" engine. It applies each rule
// independently, ApplyDelayMinutes after THAT rule's changes were first
// detected — a per-rule debounce, not a fixed cadence.
//
// The per-rule anchor is the oldest PendingChange.DetectedAt on the rule.
// Those timestamps are persisted in clonarr.json, so the delay survives
// restarts for free: a rule detected 5 days ago with a 7-day delay still
// has 2 days left after a restart, and nothing fires at boot.
//
// Only does work when Mode == "delayed" and ApplyDelayMinutes > 0. Safe to
// call on a poll tick — it computes due-ness from persisted state each time,
// holds no timer, and no-ops when nothing is due.
//
// Returns the time of the earliest upcoming apply deadline across all rules
// (zero if none pending) so the scheduler can surface a countdown. Sync
// errors are logged via the per-rule path, not returned, so one bad instance
// doesn't block the rest.
func (app *App) RunDelayedApply() time.Time {
	cfg := app.Config.Get()
	if cfg.ProfileSync == nil ||
		cfg.ProfileSync.Mode != ProfileSyncModeDelayed ||
		cfg.ProfileSync.ApplyDelayMinutes <= 0 {
		return time.Time{}
	}
	delay := time.Duration(cfg.ProfileSync.ApplyDelayMinutes) * time.Minute
	now := time.Now()

	var due []AutoSyncRule
	var earliestUpcoming time.Time
	anyTrashSource := false

	for _, rule := range cfg.AutoSync.Rules {
		if !rule.Enabled || rule.OrphanedAt != "" || rule.ProfileSource == "imported" {
			continue
		}
		if len(rule.PendingChanges) == 0 {
			continue
		}
		oldest := oldestPendingDetectedAt(rule.PendingChanges)
		if oldest.IsZero() {
			continue // unparseable timestamps — skip rather than fire blindly
		}
		deadline := oldest.Add(delay)
		if !now.Before(deadline) {
			due = append(due, rule)
			// Track whether a pull is needed: any due rule with a
			// TRaSH-source pending change means local data is behind
			// upstream (detection is ls-remote-only and never pulled).
			for _, pc := range rule.PendingChanges {
				if pc.Source != "drift" {
					anyTrashSource = true
					break
				}
			}
		} else if earliestUpcoming.IsZero() || deadline.Before(earliestUpcoming) {
			earliestUpcoming = deadline
		}
	}

	if len(due) == 0 {
		// Nothing due yet. Log the waiting state once per distinct deadline
		// so the user can confirm the delay is active without it spamming
		// every 30s poll tick. lastDelayedApplyLog throttles to one line per
		// earliest-deadline change.
		if !earliestUpcoming.IsZero() && !earliestUpcoming.Equal(app.lastDelayedApplyLog) {
			app.lastDelayedApplyLog = earliestUpcoming
			app.DebugLog.Logf(LogAutoSync, "delayed-apply: changes pending, earliest applies at %s (%s delay)",
				earliestUpcoming.Format("2006-01-02 15:04"), humanizeMinutes(cfg.ProfileSync.ApplyDelayMinutes))
		}
		return earliestUpcoming
	}
	app.lastDelayedApplyLog = time.Time{} // reset throttle once something fires

	// Pull once before syncing if any due rule needs fresh TRaSH data.
	// Drift-only due rules don't need a pull — their target is built from
	// already-local data; the sync just re-pushes it to overwrite the drift.
	if anyTrashSource {
		if err := app.RunPullOnly(SourceDelayedApply); err != nil {
			app.DebugLog.Logf(LogError, "delayed-apply: pull failed, skipping this cycle: %v", err)
			return earliestUpcoming
		}
	}

	currentCommit := app.Trash.CurrentCommit()
	if currentCommit == "" {
		app.DebugLog.Logf(LogAutoSync, "delayed-apply: TRaSH data not loaded — skipping")
		return earliestUpcoming
	}

	tick := app.DebugLog.BeginOp(OpAutoSync, SourceDelayedApply,
		fmt.Sprintf("commit=%s due-rules=%d delay=%dm", shortHash(currentCommit), len(due), cfg.ProfileSync.ApplyDelayMinutes))
	changed, noChange, errorCount, changedSummary := app.runRulesPerInstance(due, currentCommit, tick)
	if len(changedSummary) > 0 {
		tick.Logf("changed: %s", strings.Join(changedSummary, " | "))
	}
	tick.End(fmt.Sprintf("ok | %d changed, %d no-op, %d errors", changed, noChange, errorCount))

	// Recompute earliest-upcoming AFTER the sync — due rules just had their
	// pendingChanges cleared, so the next deadline is among the rules that
	// weren't due this pass. Cheapest correct answer: re-read config.
	return app.earliestApplyDeadline()
}

// humanizeMinutes renders a minute count as the friendliest unit for logs +
// messages: "30 minutes", "6 hours", "7 days". Falls back to minutes for
// values that don't divide evenly.
func humanizeMinutes(m int) string {
	if m <= 0 {
		return "0 minutes"
	}
	if m%1440 == 0 {
		d := m / 1440
		if d == 1 {
			return "1 day"
		}
		return fmt.Sprintf("%d days", d)
	}
	if m%60 == 0 {
		h := m / 60
		if h == 1 {
			return "1 hour"
		}
		return fmt.Sprintf("%d hours", h)
	}
	if m == 1 {
		return "1 minute"
	}
	return fmt.Sprintf("%d minutes", m)
}

// oldestPendingDetectedAt returns the earliest DetectedAt among a rule's
// pending changes. Zero time when none parse.
func oldestPendingDetectedAt(pcs []PendingChange) time.Time {
	var oldest time.Time
	for _, pc := range pcs {
		if pc.DetectedAt == "" {
			continue
		}
		t, err := time.Parse(time.RFC3339, pc.DetectedAt)
		if err != nil {
			continue
		}
		if oldest.IsZero() || t.Before(oldest) {
			oldest = t
		}
	}
	return oldest
}

// earliestApplyDeadline scans all eligible rules and returns the soonest
// upcoming apply deadline (oldest pending + delay). Zero when delayed mode
// is off or nothing is pending. Used to drive the UI countdown.
func (app *App) earliestApplyDeadline() time.Time {
	cfg := app.Config.Get()
	if cfg.ProfileSync == nil ||
		cfg.ProfileSync.Mode != ProfileSyncModeDelayed ||
		cfg.ProfileSync.ApplyDelayMinutes <= 0 {
		return time.Time{}
	}
	delay := time.Duration(cfg.ProfileSync.ApplyDelayMinutes) * time.Minute
	var earliest time.Time
	for _, rule := range cfg.AutoSync.Rules {
		if !rule.Enabled || rule.OrphanedAt != "" || rule.ProfileSource == "imported" {
			continue
		}
		if len(rule.PendingChanges) == 0 {
			continue
		}
		oldest := oldestPendingDetectedAt(rule.PendingChanges)
		if oldest.IsZero() {
			continue
		}
		deadline := oldest.Add(delay)
		if earliest.IsZero() || deadline.Before(earliest) {
			earliest = deadline
		}
	}
	return earliest
}
