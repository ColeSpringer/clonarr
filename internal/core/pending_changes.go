package core

// PendingChange is one TRaSH-side modification that affects a specific sync
// rule, persisted per-rule so the UI can render "what's pending" timelines
// (the backlog view) and the runner can fingerprint the affected set for
// dedup. A future parallel "drift" source covers Arr-side changes.
//
// The accumulator is union-semantics: re-observing the same change across
// hourly ticks doesn't duplicate the entry, but a NEW commit touching the
// same trash_id does add a new entry (so the timeline shows the history).
type PendingChange struct {
	Source       string `json:"source"`               // "trash" (TRaSH upstream) | "drift" (Arr-side, future)
	DetectedAt   string `json:"detectedAt"`           // RFC3339
	CommitHash   string `json:"commitHash,omitempty"` // TRaSH commit that introduced this change
	ChangeType   string `json:"changeType"`           // "cf-modified" | "cf-added" | "cf-removed" | "cf-group-modified" | "profile-modified" | "qs-modified"
	AffectedID   string `json:"affectedId"`           // trash_id of the changed CF / cf-group / profile / qs
	AffectedName string `json:"affectedName,omitempty"` // human-readable; populated when looked up at detection-time
}

// MergePendingChanges union-merges new into existing, deduplicating on
// (AffectedID, ChangeType) — the LOGICAL identity of a pending change.
// CommitHash is excluded from the key on purpose: if the same change is
// still pending across multiple Check runs at different upstream HEADs,
// we want ONE entry that reflects the latest detection, not N entries.
// Incoming wins on conflict (its CommitHash + AffectedName replace the
// existing entry's), so the timeline shows the newest commit + name.
//
// Cap at maxPendingChangesPerRule to prevent unbounded growth if upstream
// goes wild (or detection misclassifies). Oldest entries drop first.
func MergePendingChanges(existing, incoming []PendingChange) []PendingChange {
	keyFor := func(pc PendingChange) string {
		return pc.AffectedID + "|" + pc.ChangeType
	}
	byKey := make(map[string]PendingChange, len(existing)+len(incoming))
	order := make([]string, 0, len(existing)+len(incoming))
	for _, pc := range existing {
		k := keyFor(pc)
		if _, ok := byKey[k]; !ok {
			order = append(order, k)
		}
		byKey[k] = pc
	}
	for _, pc := range incoming {
		k := keyFor(pc)
		if _, ok := byKey[k]; !ok {
			order = append(order, k)
		}
		byKey[k] = pc // incoming wins — refresh CommitHash, AffectedName, etc.
	}
	out := make([]PendingChange, 0, len(order))
	for _, k := range order {
		out = append(out, byKey[k])
	}
	if len(out) > maxPendingChangesPerRule {
		out = out[len(out)-maxPendingChangesPerRule:]
	}
	return out
}

// maxPendingChangesPerRule caps per-rule storage so a misbehaving detection
// run or a flood of upstream commits can't bloat clonarr.json indefinitely.
// 50 is plenty for the UI backlog view and lets older entries roll off
// naturally as the user pulls or dismisses.
const maxPendingChangesPerRule = 50
