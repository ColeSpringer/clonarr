package core

// PendingChange is one TRaSH-side modification that affects a specific sync
// rule, persisted per-rule so the UI can render "what's pending" timelines
// (Phase 4 backlog view) and the runner can fingerprint the affected set
// for dedup. Phase D will add a parallel "drift" source for Arr-side
// changes.
//
// The accumulator is union-semantics: re-observing the same change across
// hourly ticks doesn't duplicate the entry, but a NEW commit touching the
// same trash_id does add a new entry (so the timeline shows the history).
type PendingChange struct {
	Source       string `json:"source"`               // "trash" (Phase C) | "drift" (Phase D)
	DetectedAt   string `json:"detectedAt"`           // RFC3339
	CommitHash   string `json:"commitHash,omitempty"` // TRaSH commit that introduced this change
	ChangeType   string `json:"changeType"`           // "cf-modified" | "cf-added" | "cf-removed" | "cf-group-modified" | "profile-modified" | "qs-modified"
	AffectedID   string `json:"affectedId"`           // trash_id of the changed CF / cf-group / profile / qs
	AffectedName string `json:"affectedName,omitempty"` // human-readable; populated when looked up at detection-time
}

// MergePendingChanges union-merges new into existing, deduplicating on
// (CommitHash, AffectedID, ChangeType) so the same change isn't recorded
// twice on subsequent ticks. Returns the merged list; caller is responsible
// for assigning back to the rule.
//
// Cap at maxPendingChangesPerRule to prevent unbounded growth if upstream
// goes wild (or detection misclassifies). Oldest entries drop first.
func MergePendingChanges(existing, incoming []PendingChange) []PendingChange {
	seen := make(map[string]bool, len(existing))
	keyFor := func(pc PendingChange) string {
		return pc.CommitHash + "|" + pc.AffectedID + "|" + pc.ChangeType
	}
	out := make([]PendingChange, 0, len(existing)+len(incoming))
	for _, pc := range existing {
		seen[keyFor(pc)] = true
		out = append(out, pc)
	}
	for _, pc := range incoming {
		if !seen[keyFor(pc)] {
			seen[keyFor(pc)] = true
			out = append(out, pc)
		}
	}
	if len(out) > maxPendingChangesPerRule {
		out = out[len(out)-maxPendingChangesPerRule:]
	}
	return out
}

// maxPendingChangesPerRule caps per-rule storage so a misbehaving detection
// run or a flood of upstream commits can't bloat clonarr.json indefinitely.
// 50 is plenty for the UI backlog view (Phase 4) and lets older entries
// roll off naturally as the user pulls or dismisses.
const maxPendingChangesPerRule = 50
