package core

import (
	"fmt"
	"strings"
)

// NotifyUpstreamUpdate fires the "TRaSH upstream ahead" notification —
// triggered when ProfileSyncRunner.runDetectionOnly() finds the remote HEAD
// has advanced beyond the local clone without performing a pull. Tells users
// who run Mode=notify or Mode=delayed that there are new TRaSH commits
// waiting for them, so they can click "Pull and sync now" when they're
// ready.
//
// Phase C MVP: one aggregated notification per detection-run that reports
// the commit-hash delta. Phase C commit 2 will add per-rule mapping so the
// notification can name which profiles are affected.
//
// Dispatches to every notification agent that has OnUpstreamAhead enabled
// (default false on existing agents — opt-in via Settings → Notifications
// once that toggle ships in a follow-up).
func (app *App) NotifyUpstreamUpdate(prevCommit, newCommit string) {
	cfg := app.Config.Get()

	// No agents opted in → cheap no-op. Skip the message build entirely.
	hasOptIn := false
	for _, agent := range cfg.AutoSync.NotificationAgents {
		if agent.Events.OnUpstreamAhead {
			hasOptIn = true
			break
		}
	}
	if !hasOptIn {
		return
	}

	title := "Clonarr: TRaSH updates available"
	parts := []string{
		"TRaSH-Guides has new commits upstream that clonarr hasn't pulled yet.",
		fmt.Sprintf("**Local:** `%s`", shortHash(prevCommit)),
		fmt.Sprintf("**Upstream:** `%s`", shortHash(newCommit)),
		"",
		"Open Clonarr and click **Pull** in the sidebar to apply, or wait for the next scheduled pull.",
	}
	payload := NotificationPayload{
		Title:    title,
		Message:  strings.Join(parts, "\n"),
		Color:    0x58a6ff, // accent-blue (matches "update available" UI badge in Phase 4 UI)
		Severity: NotificationSeverityInfo,
		// Route to the updates channel so users who configured a separate
		// Discord webhook for OnRepoUpdate get this event there too —
		// semantically these are both "TRaSH-Guides repo state changed"
		// notifications and belong in the same channel.
		Route: NotificationRouteUpdates,
	}
	for _, agent := range cfg.AutoSync.NotificationAgents {
		if !agent.Events.OnUpstreamAhead {
			continue
		}
		app.DispatchNotificationAgent(agent, payload)
	}
}
