package core

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
)

// WatchState lives on AutoSyncRule and tracks notification-dedup state for
// the Profile Sync subsystem. Without this, the runner would re-fire a
// notification on every detection tick while the user lets a pending state
// sit. With it, notifications fire only when the underlying change-set
// actually grows or shrinks for that specific rule.
//
// Two fingerprints because TRaSH-upstream and Arr-drift have independent
// state lifecycles — TRaSH advances when upstream commits land, drift
// changes when someone edits Arr directly.
type WatchState struct {
	// LastUpstreamFingerprint is the SHA-fragment over the sorted list of
	// trash_ids that this rule was affected by in the most-recent detection
	// run. Phase C uses this for "is this a new set vs what we already
	// notified about?" check.
	LastUpstreamFingerprint string `json:"lastUpstreamFingerprint,omitempty"`
	// LastUpstreamNotifiedAt is the RFC3339 timestamp of the most-recent
	// upstream-ahead notification we fired for this rule. Surfaced in UI
	// so users can see "last alerted Xh ago".
	LastUpstreamNotifiedAt string `json:"lastUpstreamNotifiedAt,omitempty"`

	// LastDriftFingerprint + LastDriftNotifiedAt: same dedup pattern for
	// Phase D's Arr-drift detection. Field defined now so Phase D doesn't
	// have to migrate WatchState shapes; runtime population lands then.
	LastDriftFingerprint  string `json:"lastDriftFingerprint,omitempty"`
	LastDriftNotifiedAt   string `json:"lastDriftNotifiedAt,omitempty"`
}

// ComputeUpstreamFingerprint returns a stable 12-char SHA fragment over a
// set of trash_ids. Sort-stable so re-ordering of the input doesn't change
// the result. Empty set returns "" so an empty-fingerprint check is the same
// as "no affected items yet".
func ComputeUpstreamFingerprint(trashIDs []string) string {
	if len(trashIDs) == 0 {
		return ""
	}
	sorted := make([]string, len(trashIDs))
	copy(sorted, trashIDs)
	sort.Strings(sorted)
	h := sha256.New()
	for _, id := range sorted {
		h.Write([]byte(id))
		h.Write([]byte{0}) // separator so "abc"+"def" ≠ "ab"+"cdef"
	}
	return hex.EncodeToString(h.Sum(nil))[:12]
}
