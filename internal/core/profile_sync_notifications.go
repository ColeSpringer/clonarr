package core

import (
	"fmt"
	"strings"
	"time"
)

// UpstreamChangeSummary carries aggregate detection results so the
// "upstream ahead" notification can name which CFs / profiles changed
// and how many of the user's rules are affected. Nil / zero-count means
// the caller didn't have per-rule data (degraded mode — falls back to
// commit-hash-only message).
type UpstreamChangeSummary struct {
	AffectedRuleCount int
	AffectedCFs       []AffectedItem // Custom Formats whose file changed
	AffectedProfiles  []AffectedItem // Quality Profiles whose file changed
	AffectedRules     []AffectedItem // the user's sync rules (by Arr/TRaSH profile name) that are affected
	// ManualUpdateRules is the subset of affected rules that have auto-sync
	// OFF and a NEW change-set since the last detection. In auto mode these
	// are skipped by the apply pass, so the runner notifies the user to apply
	// them by hand. Populated only with rules whose per-rule shouldNotify is
	// true, so repeated detection ticks with no new change stay silent.
	ManualUpdateRules []AffectedItem
}

// AffectedItem is one CF or profile that changed upstream AND is in scope
// for at least one of the user's rules. Used inside UpstreamChangeSummary.
type AffectedItem struct {
	Name    string // human-readable
	AppType string // "radarr" | "sonarr"
}

// NotifyUpstreamUpdate fires the "TRaSH upstream ahead" notification —
// triggered when ProfileSyncRunner.runDetectionOnly() finds the remote
// HEAD has advanced beyond the local clone without performing a pull.
//
// When summary is non-nil and AffectedRuleCount > 0, the message names the
// affected CFs and rule count. When summary is nil or empty, falls back to
// a commit-hash-only message (degraded mode if the per-rule mapping path
// failed for some reason).
//
// Dispatches to every notification agent that has OnUpstreamAhead enabled.
func (app *App) NotifyUpstreamUpdate(prevCommit, newCommit string, summary *UpstreamChangeSummary) {
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

	var title, message string
	if summary != nil && summary.AffectedRuleCount > 0 {
		title = fmt.Sprintf("Clonarr: TRaSH updates affect %d of your sync rule%s", summary.AffectedRuleCount, plural(summary.AffectedRuleCount))
		var parts []string
		cfCount := len(summary.AffectedCFs)
		profCount := len(summary.AffectedProfiles)
		summaryLine := buildAffectedSummaryLine(cfCount, profCount)
		parts = append(parts, summaryLine)
		const maxShown = 8
		renderList := func(label string, items []AffectedItem) {
			if len(items) == 0 {
				return
			}
			parts = append(parts, "")
			parts = append(parts, "**"+label+":**")
			shown := items
			extra := 0
			if len(shown) > maxShown {
				extra = len(shown) - maxShown
				shown = shown[:maxShown]
			}
			for _, it := range shown {
				app := it.AppType
				if app == "radarr" {
					app = "Radarr"
				} else if app == "sonarr" {
					app = "Sonarr"
				}
				parts = append(parts, fmt.Sprintf("• %s (%s)", it.Name, app))
			}
			if extra > 0 {
				parts = append(parts, fmt.Sprintf("...and %d more", extra))
			}
		}
		renderList("Custom Formats", summary.AffectedCFs)
		renderList("Quality Profiles", summary.AffectedProfiles)
		renderList("Your profiles affected", summary.AffectedRules)
		parts = append(parts, "")
		// Closing line is mode-aware. In "Wait before applying" mode the user
		// doesn't click Pull — clonarr applies each rule automatically after
		// the configured delay, so tell them roughly when that happens.
		// Notify mode keeps the manual "click Pull" call-to-action.
		if cfg.ProfileSync != nil && cfg.ProfileSync.Mode == ProfileSyncModeDelayed && cfg.ProfileSync.ApplyDelayMinutes > 0 {
			applyAt := time.Now().Add(time.Duration(cfg.ProfileSync.ApplyDelayMinutes) * time.Minute)
			parts = append(parts, fmt.Sprintf("These will be applied automatically around **%s** (%s after detection). No action needed. Open Clonarr first if you want to review or apply sooner.",
				applyAt.Format("15:04"), humanizeMinutes(cfg.ProfileSync.ApplyDelayMinutes)))
		} else {
			parts = append(parts, fmt.Sprintf("Open Clonarr and click **Pull** to apply (`%s` → `%s`).", shortHash(prevCommit), shortHash(newCommit)))
		}
		message = strings.Join(parts, "\n")
	} else {
		// Degraded fallback — per-rule mapping unavailable or zero matches.
		title = "Clonarr: TRaSH upstream has new commits"
		message = strings.Join([]string{
			fmt.Sprintf("TRaSH-Guides advanced from `%s` to `%s`.", shortHash(prevCommit), shortHash(newCommit)),
			"",
			"None of the changes appear to affect your synced profiles. The next Pull will catch the clone up.",
		}, "\n")
	}

	payload := NotificationPayload{
		Title:    title,
		Message:  message,
		Color:    0x58a6ff, // accent-blue (matches "update available" UI badge)
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

// NotifyManualUpdateNeeded fires in auto mode for sync rules that have
// auto-sync turned OFF but whose TRaSH source just changed upstream. The
// auto-apply pass skips those rules, so the user has to apply them by hand.
//
// Dedup is the caller's job: rules only land in the list when their per-rule
// shouldNotify is true (change-set is new since last detection), and in auto
// mode the pull advances the local clone to upstream so the next tick finds
// no diff. Together that means one notification per new change, not one per
// scheduler tick.
//
// Reuses the OnUpstreamAhead event ("TRaSH updates available") so it shares
// the user's existing updates toggle and updates channel. No agents opted in
// or empty list => cheap no-op.
func (app *App) NotifyManualUpdateNeeded(rules []AffectedItem) {
	if len(rules) == 0 {
		return
	}
	cfg := app.Config.Get()

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

	n := len(rules)
	title := fmt.Sprintf("Clonarr: %d profile%s with auto-sync off need a manual update", n, plural(n))

	var parts []string
	parts = append(parts, fmt.Sprintf("TRaSH-Guides changed %d of your profile%s that have auto-sync turned **off**. Clonarr will not apply these automatically.", n, plural(n)))
	parts = append(parts, "")
	const maxShown = 8
	shown := rules
	extra := 0
	if len(shown) > maxShown {
		extra = len(shown) - maxShown
		shown = shown[:maxShown]
	}
	for _, it := range shown {
		appLabel := it.AppType
		if appLabel == "radarr" {
			appLabel = "Radarr"
		} else if appLabel == "sonarr" {
			appLabel = "Sonarr"
		}
		parts = append(parts, fmt.Sprintf("• %s (%s)", it.Name, appLabel))
	}
	if extra > 0 {
		parts = append(parts, fmt.Sprintf("...and %d more", extra))
	}
	parts = append(parts, "")
	parts = append(parts, "Open Clonarr, go to Sync Rules, and run a manual update on these profiles to apply the changes.")

	payload := NotificationPayload{
		Title:    title,
		Message:  strings.Join(parts, "\n"),
		Color:    0x58a6ff, // accent-blue (matches the Updates badge)
		Severity: NotificationSeverityInfo,
		Route:    NotificationRouteUpdates,
	}
	for _, agent := range cfg.AutoSync.NotificationAgents {
		if !agent.Events.OnUpstreamAhead {
			continue
		}
		app.DispatchNotificationAgent(agent, payload)
	}
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// affectedRuleNames renders the summary's affected sync-rule profile names
// as a comma-joined list, capped at max with a "+N more" tail. Used in log
// lines where a flat string is cleaner than a bullet list.
func affectedRuleNames(summary *UpstreamChangeSummary, max int) string {
	if summary == nil || len(summary.AffectedRules) == 0 {
		return "(unnamed)"
	}
	names := make([]string, 0, len(summary.AffectedRules))
	for _, r := range summary.AffectedRules {
		names = append(names, r.Name)
	}
	if len(names) <= max {
		return strings.Join(names, ", ")
	}
	return strings.Join(names[:max], ", ") + fmt.Sprintf(", +%d more", len(names)-max)
}

// buildAffectedSummaryLine renders one of three sentence shapes depending
// on whether the upstream changes touched CFs, profiles, or both.
func buildAffectedSummaryLine(cfCount, profCount int) string {
	switch {
	case cfCount > 0 && profCount > 0:
		return fmt.Sprintf("TRaSH-Guides updated **%d Custom Format%s** and **%d Quality Profile%s** that affect your rules.",
			cfCount, plural(cfCount), profCount, plural(profCount))
	case cfCount > 0:
		return fmt.Sprintf("TRaSH-Guides updated **%d Custom Format%s** that your profiles use.",
			cfCount, plural(cfCount))
	case profCount > 0:
		return fmt.Sprintf("TRaSH-Guides updated **%d Quality Profile%s** that your rules sync.",
			profCount, plural(profCount))
	default:
		return "TRaSH-Guides has upstream updates affecting your rules."
	}
}
