package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestProfileSync_PersistsAndRoundTrips(t *testing.T) {
	dir := t.TempDir()
	cs := NewConfigStore(dir)
	want := &ProfileSync{
		Interval: "specific",
		Specific: &PullSchedule{
			Mode:      "weekly",
			Time:      "03:00",
			DayOfWeek: 3, // Wednesday
		},
		Sources: ProfileSyncSources{TrashUpstream: true, ArrDrift: false},
		Mode:    "auto",
		LastRun:      "2026-05-24T12:00:00Z",
		UpstreamHead: "abc1234",
		LocalHead:    "def5678",
		LastResult: &ProfileSyncRunResult{
			TriggeredBy:        "scheduled",
			RanAt:              "2026-05-24T12:00:00Z",
			RulesChecked:       4,
			PendingDetected:    2,
			NotificationsFired: 1,
		},
	}
	if err := cs.Update(func(cfg *Config) {
		cfg.ProfileSync = want
	}); err != nil {
		t.Fatalf("Update(): %v", err)
	}

	cs2 := NewConfigStore(dir)
	if err := cs2.Load(); err != nil {
		t.Fatalf("Load(): %v", err)
	}
	got := cs2.Get().ProfileSync
	if got == nil {
		t.Fatal("ProfileSync lost across reload")
	}
	if got.Interval != want.Interval || got.Mode != want.Mode || got.UpstreamHead != want.UpstreamHead {
		t.Errorf("ProfileSync roundtrip mismatch:\n got=%+v\nwant=%+v", got, want)
	}
	if !got.Sources.TrashUpstream || got.Sources.ArrDrift {
		t.Errorf("Sources roundtrip mismatch: got=%+v want=%+v", got.Sources, want.Sources)
	}
	if got.Specific == nil || got.Specific.Time != want.Specific.Time || got.Specific.DayOfWeek != want.Specific.DayOfWeek {
		t.Errorf("Specific roundtrip mismatch: got=%+v want=%+v", got.Specific, want.Specific)
	}
	if got.LastResult == nil || got.LastResult.RulesChecked != 4 || got.LastResult.PendingDetected != 2 {
		t.Errorf("LastResult roundtrip lost: %+v", got.LastResult)
	}
}


func TestProfileSync_GetReturnsDeepCopy(t *testing.T) {
	dir := t.TempDir()
	cs := NewConfigStore(dir)
	if err := cs.Update(func(cfg *Config) {
		cfg.ProfileSync = &ProfileSync{
			Interval: "6h",
			Mode:     "auto",
			Sources:  ProfileSyncSources{TrashUpstream: true},
			Specific: &PullSchedule{Mode: "daily", Time: "03:00"},
			LastResult: &ProfileSyncRunResult{
				TriggeredBy: "scheduled",
				Errors:      []string{"original-error"},
			},
		}
	}); err != nil {
		t.Fatalf("Update(): %v", err)
	}

	got := cs.Get().ProfileSync
	// Pointer + slice mutations — these would leak with a shallow copy.
	got.Specific.Time = "23:59"
	got.LastResult.Errors[0] = "mutated"

	again := cs.Get().ProfileSync
	if again.Specific.Time == "23:59" {
		t.Error("Get() returned shallow Specific — mutating Time leaked to store")
	}
	if again.LastResult.Errors[0] == "mutated" {
		t.Error("Get() returned shallow LastResult.Errors — mutating slice leaked to store")
	}
}

// TestDriftWatch_EmptyButNonNilErrorsDoesNotAlias verifies a slice with
// cap > 0 and len == 0 — appending to the returned copy must not leak to
// the store via the shared backing array. The len(...) > 0 guard in Get()
// skips the make+copy for empty slices, so we test that the resulting
// caller-side append doesn't grow into the store's backing array.
func TestDriftWatch_EmptyButNonNilErrorsDoesNotAlias(t *testing.T) {
	dir := t.TempDir()
	cs := NewConfigStore(dir)
	if err := cs.Update(func(cfg *Config) {
		cfg.DriftWatch = &DriftWatch{
			Mode: "detect",
			LastResult: &DriftRunResult{
				DriftsDetected: 0,
				Errors:         make([]string, 0, 4), // empty, but cap > 0
			},
		}
	}); err != nil {
		t.Fatalf("Update(): %v", err)
	}

	got := cs.Get().DriftWatch
	got.LastResult.Errors = append(got.LastResult.Errors, "appended-by-caller")

	again := cs.Get().DriftWatch
	if len(again.LastResult.Errors) != 0 {
		t.Errorf("caller's append leaked into store: store Errors=%+v", again.LastResult.Errors)
	}
}

// TestDriftWatch_MalformedModeLoadsAsIs locks in the current contract:
// Phase 1 does not validate Mode on Load, so a JSON-edited config with
// "mode": "garbage" loads successfully and surfaces the raw value. Phase 5
// (settings UI) will add validation on the Update path; this test catches
// any accidental tightening here.
func TestDriftWatch_MalformedModeLoadsAsIs(t *testing.T) {
	dir := t.TempDir()
	cfgJSON := `{"instances":[],"pullInterval":"24h","driftWatch":{"mode":"garbage"}}`
	if err := os.WriteFile(filepath.Join(dir, "clonarr.json"), []byte(cfgJSON), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	cs := NewConfigStore(dir)
	if err := cs.Load(); err != nil {
		t.Fatalf("Load() should not reject malformed mode: %v", err)
	}
	if got := cs.Get().DriftWatch; got == nil || got.Mode != "garbage" {
		t.Errorf("want DriftWatch{Mode: garbage}, got %+v", got)
	}
}

// (TestUpdateWatch_EmptyPendingChangesRoundTrip removed — PendingChanges
// moved from subsystem-level UpdateWatch to per-rule storage in Phase C.)

// TestProfileSync_ApplyDelayRoundtrip verifies the delayed-apply config
// value (ApplyDelayMinutes) persists + reads back unchanged. Delayed mode
// is a per-rule debounce anchored to each rule's pendingChange DetectedAt;
// the only stored knob is this global minute count.
func TestProfileSync_ApplyDelayRoundtrip(t *testing.T) {
	dir := t.TempDir()
	cs := NewConfigStore(dir)
	if err := cs.Update(func(cfg *Config) {
		cfg.ProfileSync = &ProfileSync{
			Mode:              ProfileSyncModeDelayed,
			ApplyDelayMinutes: 1440, // 24h
		}
	}); err != nil {
		t.Fatalf("Update(): %v", err)
	}

	got := cs.Get().ProfileSync
	if got.ApplyDelayMinutes != 1440 {
		t.Errorf("ApplyDelayMinutes roundtrip mismatch: got=%d want=1440", got.ApplyDelayMinutes)
	}
	if got.Mode != ProfileSyncModeDelayed {
		t.Errorf("Mode roundtrip mismatch: got=%q want=%q", got.Mode, ProfileSyncModeDelayed)
	}
}

// --- Profile Sync migration tests ---

// TestProfileSyncMigration_FreshInstall verifies the migration creates a
// sensible default ProfileSync from defaults (no PullSchedule, no PullInterval
// → falls back to "24h" + Mode=auto + TRaSH-only). Mirrors today's Pull
// behaviour.

// TestProfileSyncMigration_FreshInstall verifies the migration creates a
// sensible default ProfileSync from defaults (no PullSchedule, no PullInterval
// → falls back to "24h" + Mode=auto + TRaSH-only). Mirrors today's Pull
// behaviour.
func TestProfileSyncMigration_FreshInstall(t *testing.T) {
	dir := t.TempDir()
	// Minimal config — no ProfileSync, no PullInterval (so default kicks in)
	freshCfg := map[string]any{
		"instances": []any{},
	}
	raw, _ := json.Marshal(freshCfg)
	if err := os.WriteFile(filepath.Join(dir, "clonarr.json"), raw, 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	cs := NewConfigStore(dir)
	if err := cs.Load(); err != nil {
		t.Fatalf("Load(): %v", err)
	}
	ps := cs.Get().ProfileSync
	if ps == nil {
		t.Fatal("migration should populate ProfileSync, got nil")
	}
	if ps.Mode != ProfileSyncModeAuto {
		t.Errorf("Mode = %q, want %q", ps.Mode, ProfileSyncModeAuto)
	}
	if !ps.Sources.TrashUpstream {
		t.Error("Sources.TrashUpstream should be true (matches today's Pull behaviour)")
	}
	if ps.Sources.ArrDrift {
		t.Error("Sources.ArrDrift should be false (new opt-in capability)")
	}
	if ps.Interval != "24h" {
		t.Errorf("Interval = %q, want %q (empty PullInterval → 24h default)", ps.Interval, "24h")
	}
}

// TestProfileSyncMigration_InheritsPullInterval verifies a user who had
// `pullInterval: "6h"` set gets that same cadence on ProfileSync — no
// silent change in their scheduled-pull frequency.

// TestProfileSyncMigration_InheritsPullInterval verifies a user who had
// `pullInterval: "6h"` set gets that same cadence on ProfileSync — no
// silent change in their scheduled-pull frequency.
func TestProfileSyncMigration_InheritsPullInterval(t *testing.T) {
	dir := t.TempDir()
	cfgWith6h := map[string]any{
		"instances":    []any{},
		"pullInterval": "6h",
	}
	raw, _ := json.Marshal(cfgWith6h)
	if err := os.WriteFile(filepath.Join(dir, "clonarr.json"), raw, 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	cs := NewConfigStore(dir)
	if err := cs.Load(); err != nil {
		t.Fatalf("Load(): %v", err)
	}
	ps := cs.Get().ProfileSync
	if ps == nil || ps.Interval != "6h" {
		t.Errorf("Interval should inherit existing pullInterval=6h, got %+v", ps)
	}
}

// TestProfileSyncMigration_InheritsSpecificSchedule verifies wall-clock
// schedules (PullInterval=specific + PullSchedule daily 03:00) migrate
// into ProfileSync.Specific with the same value.

// TestProfileSyncMigration_InheritsSpecificSchedule verifies wall-clock
// schedules (PullInterval=specific + PullSchedule daily 03:00) migrate
// into ProfileSync.Specific with the same value.
func TestProfileSyncMigration_InheritsSpecificSchedule(t *testing.T) {
	dir := t.TempDir()
	cfgWithSpecific := map[string]any{
		"instances":    []any{},
		"pullInterval": "specific",
		"pullSchedule": map[string]any{
			"mode":      "daily",
			"time":      "03:00",
			"dayOfWeek": 0,
		},
	}
	raw, _ := json.Marshal(cfgWithSpecific)
	if err := os.WriteFile(filepath.Join(dir, "clonarr.json"), raw, 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	cs := NewConfigStore(dir)
	if err := cs.Load(); err != nil {
		t.Fatalf("Load(): %v", err)
	}
	ps := cs.Get().ProfileSync
	if ps == nil {
		t.Fatal("migration should populate ProfileSync, got nil")
	}
	if ps.Interval != "specific" {
		t.Errorf("Interval = %q, want %q", ps.Interval, "specific")
	}
	if ps.Specific == nil || ps.Specific.Mode != "daily" || ps.Specific.Time != "03:00" {
		t.Errorf("Specific should inherit PullSchedule, got %+v", ps.Specific)
	}
}

// TestProfileSyncMigration_PreservesUserConfig verifies migration is a
// no-op when ProfileSync is already user-configured (e.g. set via API after
// a previous migration). User Mode=notify must not be reset to Mode=auto.

// TestProfileSyncMigration_PreservesUserConfig verifies migration is a
// no-op when ProfileSync is already user-configured (e.g. set via API after
// a previous migration). User Mode=notify must not be reset to Mode=auto.
func TestProfileSyncMigration_PreservesUserConfig(t *testing.T) {
	dir := t.TempDir()
	cfgPreConfigured := map[string]any{
		"instances": []any{},
		"profileSync": map[string]any{
			"interval": "1h",
			"mode":     "notify",
			"sources":  map[string]any{"trashUpstream": true, "arrDrift": false},
		},
	}
	raw, _ := json.Marshal(cfgPreConfigured)
	if err := os.WriteFile(filepath.Join(dir, "clonarr.json"), raw, 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	cs := NewConfigStore(dir)
	if err := cs.Load(); err != nil {
		t.Fatalf("Load(): %v", err)
	}
	ps := cs.Get().ProfileSync
	if ps == nil || ps.Mode != "notify" || ps.Interval != "1h" {
		t.Errorf("user-configured ProfileSync overwritten by migration: got %+v", ps)
	}
}

// TestProfileSyncMigration_NoOpOnReload locks the idempotency contract:
// once migration has persisted, restarting clonarr (second Load) does not
// re-run migration or alter persisted state.

// TestProfileSyncMigration_NoOpOnReload locks the idempotency contract:
// once migration has persisted, restarting clonarr (second Load) does not
// re-run migration or alter persisted state.
func TestProfileSyncMigration_NoOpOnReload(t *testing.T) {
	dir := t.TempDir()
	freshCfg := map[string]any{"instances": []any{}, "pullInterval": "6h"}
	raw, _ := json.Marshal(freshCfg)
	if err := os.WriteFile(filepath.Join(dir, "clonarr.json"), raw, 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// First Load — migration runs.
	cs1 := NewConfigStore(dir)
	if err := cs1.Load(); err != nil {
		t.Fatalf("Load() 1st: %v", err)
	}
	ps1 := cs1.Get().ProfileSync
	if ps1 == nil || ps1.Interval != "6h" {
		t.Fatalf("first Load should migrate, got %+v", ps1)
	}

	// Second Load — must be no-op. The persisted profileSync is read back as-is.
	cs2 := NewConfigStore(dir)
	if err := cs2.Load(); err != nil {
		t.Fatalf("Load() 2nd: %v", err)
	}
	ps2 := cs2.Get().ProfileSync
	if ps2 == nil || ps2.Interval != "6h" || ps2.Mode != ProfileSyncModeAuto {
		t.Errorf("ProfileSync mutated on second Load: %+v", ps2)
	}
}

// TestProfileSyncMigration_DayOfMonthClampInteractsCorrectly verifies that
// the existing PullSchedule.DayOfMonth clamp at config.go:540-545 runs
// BEFORE migrateProfileSync — so the migrated Specific inherits the clamped
// value (28), not the original 0. Catches future Load() refactors that
// might reorder these two passes.

// TestProfileSyncMigration_DayOfMonthClampInteractsCorrectly verifies that
// the existing PullSchedule.DayOfMonth clamp at config.go:540-545 runs
// BEFORE migrateProfileSync — so the migrated Specific inherits the clamped
// value (28), not the original 0. Catches future Load() refactors that
// might reorder these two passes.
func TestProfileSyncMigration_DayOfMonthClampInteractsCorrectly(t *testing.T) {
	dir := t.TempDir()
	cfgBadDom := map[string]any{
		"instances":    []any{},
		"pullInterval": "specific",
		"pullSchedule": map[string]any{
			"mode":       "monthly",
			"time":       "03:00",
			"dayOfMonth": 0, // invalid — clamp pass runs first
		},
	}
	raw, _ := json.Marshal(cfgBadDom)
	if err := os.WriteFile(filepath.Join(dir, "clonarr.json"), raw, 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	cs := NewConfigStore(dir)
	if err := cs.Load(); err != nil {
		t.Fatalf("Load(): %v", err)
	}
	ps := cs.Get().ProfileSync
	if ps == nil || ps.Specific == nil {
		t.Fatal("migration should populate ProfileSync.Specific")
	}
	if ps.Specific.DayOfMonth != 28 {
		t.Errorf("DayOfMonth should be clamped to 28 BEFORE migration runs, got %d", ps.Specific.DayOfMonth)
	}
}

// TestProfileSyncMigration_SpecificWithoutSchedule covers the weird
// hand-edit case: pullInterval="specific" but pullSchedule=null. Migration
// should still populate ProfileSync (Interval="specific", Specific=nil),
// and the scheduler will treat it as Manual-mode equivalent (nextDelay
// returns !armed). Lock the contract so future refactors don't promote
// this edge case to a Mode=auto fire-immediately bug.

// TestProfileSyncMigration_SpecificWithoutSchedule covers the weird
// hand-edit case: pullInterval="specific" but pullSchedule=null. Migration
// should still populate ProfileSync (Interval="specific", Specific=nil),
// and the scheduler will treat it as Manual-mode equivalent (nextDelay
// returns !armed). Lock the contract so future refactors don't promote
// this edge case to a Mode=auto fire-immediately bug.
func TestProfileSyncMigration_SpecificWithoutSchedule(t *testing.T) {
	dir := t.TempDir()
	cfgSpecificNoSchedule := map[string]any{
		"instances":    []any{},
		"pullInterval": "specific",
		// pullSchedule omitted entirely → null in Go after unmarshal
	}
	raw, _ := json.Marshal(cfgSpecificNoSchedule)
	if err := os.WriteFile(filepath.Join(dir, "clonarr.json"), raw, 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	cs := NewConfigStore(dir)
	if err := cs.Load(); err != nil {
		t.Fatalf("Load(): %v", err)
	}
	ps := cs.Get().ProfileSync
	if ps == nil {
		t.Fatal("ProfileSync should still be populated even when PullSchedule is nil")
	}
	if ps.Interval != "specific" {
		t.Errorf("Interval = %q, want %q", ps.Interval, "specific")
	}
	if ps.Specific != nil {
		t.Errorf("Specific should remain nil when source PullSchedule is nil, got %+v", ps.Specific)
	}
}

// TestIsValidProfileSyncMode locks the allowed Mode values. Adding a new
// mode requires updating both the constant block and this test, so the
// validation can't silently drift from the spec.

// TestIsValidProfileSyncMode locks the allowed Mode values. Adding a new
// mode requires updating both the constant block and this test, so the
// validation can't silently drift from the spec.
func TestIsValidProfileSyncMode(t *testing.T) {
	valid := []string{ProfileSyncModeAuto, ProfileSyncModeNotify, ProfileSyncModeDelayed}
	invalid := []string{"", "off", "AUTO", "Auto", "hot_dog", " auto "}
	for _, m := range valid {
		if !IsValidProfileSyncMode(m) {
			t.Errorf("expected %q to be valid", m)
		}
	}
	for _, m := range invalid {
		if IsValidProfileSyncMode(m) {
			t.Errorf("expected %q to be invalid", m)
		}
	}
}

