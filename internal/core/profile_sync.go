package core

import (
	"fmt"
	"log"
)

// ProfileSync is the unified Profile Sync subsystem — supersedes the
// short-lived UpdateWatch and folds in today's Pull-and-sync behaviour.
// Single subsystem that detects changes (TRaSH upstream and/or Arr-side
// drift) on a user-chosen schedule and acts on them per Mode.
//
// Scenario coverage:
//   Mode=auto + Sources.TrashUpstream → today's Pull-and-sync (default)
//   Mode=notify + Sources.TrashUpstream → notify only, never apply
//   Mode=delayed + ApplySchedule → notify now, apply at separate cadence
//   Sources.ArrDrift → adds Arr-side direct-edit detection (Phase D)
//
// Migration: populated on first load from existing PullInterval + PullSchedule
// with Mode=auto, Sources=TRaSH only. Zero functional change for existing
// users — same schedule, same behaviour. New capabilities are opt-in via UI.
type ProfileSync struct {
	// Schedule reuses the existing pull-schedule shape. PullInterval-style
	// duration ("5m", "1h", "24h"; minimum 1m via ParsePullInterval clamp) OR
	// wall-clock specification via the embedded PullSchedule pointer when
	// Interval == "specific".
	//
	// One-of:
	//   Interval == "0"        → Manual only
	//   Interval == "specific" → use Specific (wall-clock daily/weekly/monthly)
	//   Interval == <duration> → recurring interval
	Interval string        `json:"interval"`           // matches today's PullInterval semantics
	Specific *PullSchedule `json:"specific,omitempty"` // matches today's PullSchedule; populated only when Interval == "specific"

	// Sources controls what kind of changes the runner looks for. At least one
	// must be true for scheduled detection to do anything (manual button still
	// works regardless).
	Sources ProfileSyncSources `json:"sources"`

	// Mode controls what happens when detection finds changes:
	//   "auto"    — apply immediately for auto-sync ON rules; notify auto-sync OFF rules
	//   "notify"  — never apply automatically; notify all rules
	//   "delayed" — notify immediately; apply queued for ApplySchedule
	Mode string `json:"mode"`

	// ApplyInterval / ApplySpecific are ONLY consulted when Mode == "delayed".
	// In any other Mode the runner ignores them, so empty/nil is fine.
	// Same value-space as Interval/Specific above.
	ApplyInterval string        `json:"applyInterval,omitempty"`
	ApplySpecific *PullSchedule `json:"applySpecific,omitempty"`

	// Runner telemetry — written by ProfileSyncRunner after each run.
	LastRun      string                `json:"lastRun,omitempty"`      // RFC3339 timestamp
	NextRun      string                `json:"nextRun,omitempty"`      // RFC3339 timestamp; set by scheduler
	UpstreamHead string                `json:"upstreamHead,omitempty"` // commit hash from last successful ls-remote
	LocalHead    string                `json:"localHead,omitempty"`    // local HEAD at time of last run
	LastResult   *ProfileSyncRunResult `json:"lastResult,omitempty"`
}

// ProfileSyncSources captures which detection axes the runner walks each tick.
type ProfileSyncSources struct {
	TrashUpstream bool `json:"trashUpstream"` // git ls-remote + fetch-to-side-ref + diff walk
	ArrDrift      bool `json:"arrDrift"`      // per-rule Arr live state vs ComputeArrTarget diff (Phase D)
}

// ProfileSyncRunResult is the per-run telemetry surfaced via the API so the UI
// can render "last run: 4 minutes ago — 3 changes detected" without recomputing.
type ProfileSyncRunResult struct {
	TriggeredBy        string   `json:"triggeredBy"`         // ProfileSyncTriggerScheduled | ProfileSyncTriggerManual
	RanAt              string   `json:"ranAt"`               // RFC3339
	RulesChecked       int      `json:"rulesChecked"`        // populated in Phase B (currently always 0)
	PendingDetected    int      `json:"pendingDetected"`     // populated in Phase C (per-rule PendingChange accumulator)
	NotificationsFired int      `json:"notificationsFired"`  // populated in Phase C (post-dedup count)
	Errors             []string `json:"errors,omitempty"`    // per-rule errors (instance unreachable, etc.)
}

// ProfileSync mode constants. Mode determines what happens when detection
// finds changes. Defined as constants so the runner + API + tests share one
// vocabulary (typo at one site = compile error, not runtime mystery).
const (
	ProfileSyncModeAuto    = "auto"    // apply inline for auto-sync-ON rules
	ProfileSyncModeNotify  = "notify"  // never apply; notify all rules
	ProfileSyncModeDelayed = "delayed" // notify now; apply on ApplySchedule
)

// IsValidProfileSyncMode reports whether the given string is one of the
// known modes. Empty string is intentionally INVALID — fresh installs without
// migration have Mode="" and the runner treats that as off.
func IsValidProfileSyncMode(m string) bool {
	return m == ProfileSyncModeAuto || m == ProfileSyncModeNotify || m == ProfileSyncModeDelayed
}

// ProfileSyncRunResult.TriggeredBy constants.
const (
	ProfileSyncTriggerScheduled = "scheduled"
	ProfileSyncTriggerManual    = "manual"
)

// migrateProfileSync populates cs.config.ProfileSync from the legacy
// PullInterval + PullSchedule pair when ProfileSync is nil. Returns true if
// a migration was performed (caller persists). False on re-runs or when
// the user has already configured ProfileSync via the API.
//
// Caller (ConfigStore.Load) holds cs.mu — no internal locking here.
func (cs *ConfigStore) migrateProfileSync() bool {
	if cs.config.ProfileSync != nil {
		return false // already migrated or user-configured — leave alone
	}
	ps := &ProfileSync{
		Mode: ProfileSyncModeAuto,
		Sources: ProfileSyncSources{
			TrashUpstream: true,  // matches today's Pull-then-sync flow
			ArrDrift:      false, // new capability — opt-in via UI
		},
	}
	// Inherit today's pull cadence directly. PullInterval semantics carry
	// over 1:1 (empty → "24h" default, "0" → manual, "specific" → use Specific).
	if cs.config.PullInterval != "" {
		ps.Interval = cs.config.PullInterval
	} else {
		ps.Interval = "24h"
	}
	if cs.config.PullSchedule != nil {
		schedCopy := *cs.config.PullSchedule
		ps.Specific = &schedCopy
	}
	cs.config.ProfileSync = ps

	intervalDesc := ps.Interval
	switch ps.Interval {
	case "", "0":
		intervalDesc = "manual"
	case "specific":
		if ps.Specific != nil {
			intervalDesc = fmt.Sprintf("specific (%s)", ps.Specific.Mode)
		}
	}
	log.Printf("Migrated to ProfileSync (mode=%s, sources=TRaSH-only, interval=%s)", ps.Mode, intervalDesc)
	return true
}
