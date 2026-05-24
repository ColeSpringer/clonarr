package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestMigrateFlatNotifications verifies that a v2.0.x config file containing
// the legacy flat notification fields (discordWebhook, gotifyUrl, etc.) is
// promoted to the new NotificationAgents slice on Load(), with correct names,
// types, credentials, and event subscriptions preserved.
func TestMigrateFlatNotifications(t *testing.T) {
	// Build a minimal config JSON that mimics a pre-agent v2.0.x file.
	oldCfg := map[string]any{
		"autoSync": map[string]any{
			"enabled":            true,
			"notifyOnSuccess":    true,
			"notifyOnFailure":    true,
			"notifyOnRepoUpdate": false,
			// Discord
			"discordWebhook":        "https://discord.com/api/webhooks/111/aaa",
			"discordWebhookUpdates": "https://discord.com/api/webhooks/222/bbb",
			"discordEnabled":        true,
			// Gotify
			"gotifyUrl":              "https://gotify.example.com",
			"gotifyToken":            "tok123",
			"gotifyEnabled":          false,
			"gotifyPriorityCritical": true,
			"gotifyPriorityWarning":  true,
			"gotifyPriorityInfo":     false,
			"gotifyCriticalValue":    8,
			"gotifyWarningValue":     5,
			"gotifyInfoValue":        3,
			// Pushover
			"pushoverUserKey":  "ukey456",
			"pushoverAppToken": "atoken789",
			"pushoverEnabled":  false,
		},
	}

	raw, err := json.Marshal(oldCfg)
	if err != nil {
		t.Fatalf("marshal old config: %v", err)
	}

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "clonarr.json")
	if err := os.WriteFile(cfgPath, raw, 0600); err != nil {
		t.Fatalf("write temp config: %v", err)
	}

	cs := NewConfigStore(dir)
	if err := cs.Load(); err != nil {
		t.Fatalf("Load(): %v", err)
	}

	agents := cs.Get().AutoSync.NotificationAgents
	if len(agents) != 3 {
		t.Fatalf("want 3 agents after migration, got %d", len(agents))
	}

	// Build a map by type for order-independent assertions.
	byType := make(map[string]NotificationAgent, 3)
	for _, a := range agents {
		byType[a.Type] = a
	}

	// --- Discord ---
	d, ok := byType["discord"]
	if !ok {
		t.Fatal("no discord agent after migration")
	}
	if d.Name != "Discord" {
		t.Errorf("discord name = %q, want %q", d.Name, "Discord")
	}
	if !d.Enabled {
		t.Error("discord agent should be enabled (discordEnabled=true)")
	}
	if d.Config.DiscordWebhook != "https://discord.com/api/webhooks/111/aaa" {
		t.Errorf("discord webhook = %q", d.Config.DiscordWebhook)
	}
	if d.Config.DiscordWebhookUpdates != "https://discord.com/api/webhooks/222/bbb" {
		t.Errorf("discord updates webhook = %q", d.Config.DiscordWebhookUpdates)
	}
	if !d.Events.OnSyncSuccess {
		t.Error("discord: OnSyncSuccess should be true")
	}
	if !d.Events.OnSyncFailure {
		t.Error("discord: OnSyncFailure should be true")
	}
	if d.Events.OnRepoUpdate {
		t.Error("discord: OnRepoUpdate should be false")
	}

	// --- Gotify ---
	g, ok := byType["gotify"]
	if !ok {
		t.Fatal("no gotify agent after migration")
	}
	if g.Name != "Gotify" {
		t.Errorf("gotify name = %q, want %q", g.Name, "Gotify")
	}
	if g.Enabled {
		t.Error("gotify agent should be disabled (gotifyEnabled=false)")
	}
	if g.Config.GotifyURL != "https://gotify.example.com" {
		t.Errorf("gotify url = %q", g.Config.GotifyURL)
	}
	if g.Config.GotifyToken != "tok123" {
		t.Errorf("gotify token = %q", g.Config.GotifyToken)
	}
	if g.Config.GotifyCriticalValue == nil || *g.Config.GotifyCriticalValue != 8 {
		t.Errorf("gotify critical value wrong")
	}

	// --- Pushover ---
	p, ok := byType["pushover"]
	if !ok {
		t.Fatal("no pushover agent after migration")
	}
	if p.Name != "Pushover" {
		t.Errorf("pushover name = %q, want %q", p.Name, "Pushover")
	}
	if p.Enabled {
		t.Error("pushover agent should be disabled (pushoverEnabled=false)")
	}
	if p.Config.PushoverUserKey != "ukey456" {
		t.Errorf("pushover user key = %q", p.Config.PushoverUserKey)
	}
	if p.Config.PushoverAppToken != "atoken789" {
		t.Errorf("pushover app token = %q", p.Config.PushoverAppToken)
	}

	// --- Idempotency: second Load() must not duplicate agents ---
	if err := cs.Load(); err != nil {
		t.Fatalf("second Load(): %v", err)
	}
	agents2 := cs.Get().AutoSync.NotificationAgents
	if len(agents2) != 3 {
		t.Errorf("idempotency: want 3 agents on second load, got %d", len(agents2))
	}
}

// TestMigrateFlatNotificationsEmpty verifies that Load() on a config with no
// flat notification fields produces zero agents (no phantom entries).
func TestMigrateFlatNotificationsEmpty(t *testing.T) {
	emptyCfg := map[string]any{
		"autoSync": map[string]any{"enabled": false},
	}
	raw, _ := json.Marshal(emptyCfg)

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "clonarr.json"), raw, 0600)

	cs := NewConfigStore(dir)
	if err := cs.Load(); err != nil {
		t.Fatalf("Load(): %v", err)
	}
	if n := len(cs.Get().AutoSync.NotificationAgents); n != 0 {
		t.Errorf("want 0 agents for empty config, got %d", n)
	}
}

// TestDeleteSyncHistory_RemovesAllMatching verifies that DeleteSyncHistory
// removes EVERY entry with the matching (instanceID, arrProfileID) pair —
// not just the first one. A profile that's been synced multiple times has
// multiple history entries; the UI dedupes them to one row, so a single
// delete must clear all of them.
func TestDeleteSyncHistory_RemovesAllMatching(t *testing.T) {
	dir := t.TempDir()
	cs := NewConfigStore(dir)
	if err := cs.Update(func(cfg *Config) {
		cfg.SyncHistory = []SyncHistoryEntry{
			{InstanceID: "inst-A", ArrProfileID: 10, ProfileName: "first"},
			{InstanceID: "inst-A", ArrProfileID: 10, ProfileName: "second"},
			{InstanceID: "inst-A", ArrProfileID: 10, ProfileName: "third"},
			{InstanceID: "inst-A", ArrProfileID: 99, ProfileName: "different-profile"},
			{InstanceID: "inst-B", ArrProfileID: 10, ProfileName: "different-instance"},
		}
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if err := cs.DeleteSyncHistory("inst-A", 10); err != nil {
		t.Fatalf("DeleteSyncHistory: %v", err)
	}

	got := cs.Get().SyncHistory
	if len(got) != 2 {
		t.Errorf("want 2 entries left, got %d: %+v", len(got), got)
	}
	for _, sh := range got {
		if sh.InstanceID == "inst-A" && sh.ArrProfileID == 10 {
			t.Errorf("entry for (inst-A, 10) was not removed: %+v", sh)
		}
	}
}

// TestDeleteSyncHistory_NotFound verifies that calling delete on a
// non-existent (instanceID, arrProfileID) pair returns an error and leaves
// existing entries untouched.
func TestDeleteSyncHistory_NotFound(t *testing.T) {
	dir := t.TempDir()
	cs := NewConfigStore(dir)
	if err := cs.Update(func(cfg *Config) {
		cfg.SyncHistory = []SyncHistoryEntry{
			{InstanceID: "inst-A", ArrProfileID: 10},
		}
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if err := cs.DeleteSyncHistory("inst-A", 999); err == nil {
		t.Error("want error for missing entry, got nil")
	}
	if got := cs.Get().SyncHistory; len(got) != 1 {
		t.Errorf("untouched entries should remain, got %d", len(got))
	}
}

// TestAddNotificationAgentAllowsMultipleEnabledSameType verifies users can
// configure multiple active agents at once, including multiple agents of the
// same provider type.
func TestAddNotificationAgentAllowsMultipleEnabledSameType(t *testing.T) {
	dir := t.TempDir()
	cs := NewConfigStore(dir)

	a1 := NotificationAgent{
		Name:    "Discord Alerts",
		Type:    "discord",
		Enabled: true,
		Events: AgentEvents{
			OnSyncSuccess: true,
			OnSyncFailure: true,
		},
		Config: NotificationConfig{
			DiscordWebhook: "https://discord.com/api/webhooks/111/aaa",
		},
	}
	a2 := NotificationAgent{
		Name:    "Discord Ops",
		Type:    "discord",
		Enabled: true,
		Events: AgentEvents{
			OnSyncSuccess: true,
			OnSyncFailure: true,
		},
		Config: NotificationConfig{
			DiscordWebhook: "https://discord.com/api/webhooks/222/bbb",
		},
	}

	created1, err := cs.AddNotificationAgent(a1)
	if err != nil {
		t.Fatalf("AddNotificationAgent(a1): %v", err)
	}
	created2, err := cs.AddNotificationAgent(a2)
	if err != nil {
		t.Fatalf("AddNotificationAgent(a2): %v", err)
	}
	if created1.ID == "" || created2.ID == "" {
		t.Fatal("expected generated IDs for both agents")
	}
	if created1.ID == created2.ID {
		t.Fatal("expected unique IDs for each configured agent")
	}

	cfg := cs.Get()
	if len(cfg.AutoSync.NotificationAgents) != 2 {
		t.Fatalf("want 2 configured agents, got %d", len(cfg.AutoSync.NotificationAgents))
	}

	enabledCount := 0
	discordCount := 0
	for _, a := range cfg.AutoSync.NotificationAgents {
		if a.Enabled {
			enabledCount++
		}
		if a.Type == "discord" {
			discordCount++
		}
	}
	if enabledCount != 2 {
		t.Fatalf("want 2 enabled agents, got %d", enabledCount)
	}
	if discordCount != 2 {
		t.Fatalf("want 2 discord agents, got %d", discordCount)
	}

	cs2 := NewConfigStore(dir)
	if err := cs2.Load(); err != nil {
		t.Fatalf("Load() after save: %v", err)
	}
	reloaded := cs2.Get().AutoSync.NotificationAgents
	if len(reloaded) != 2 {
		t.Fatalf("want 2 agents after reload, got %d", len(reloaded))
	}
}

// --- Watch & Drift (Phase 1) — config-shape tests ---

func TestDriftWatchAndUpdateWatch_DefaultNil(t *testing.T) {
	dir := t.TempDir()
	freshCfg := map[string]any{
		"instances":    []any{},
		"pullInterval": "24h",
	}
	raw, _ := json.Marshal(freshCfg)
	if err := os.WriteFile(filepath.Join(dir, "clonarr.json"), raw, 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	cs := NewConfigStore(dir)
	if err := cs.Load(); err != nil {
		t.Fatalf("Load(): %v", err)
	}
	cfg := cs.Get()
	if cfg.DriftWatch != nil {
		t.Errorf("DriftWatch should be nil on fresh install, got %+v", cfg.DriftWatch)
	}
	if cfg.UpdateWatch != nil {
		t.Errorf("UpdateWatch should be nil on fresh install, got %+v", cfg.UpdateWatch)
	}
}

func TestDriftWatch_PersistsAndRoundTrips(t *testing.T) {
	dir := t.TempDir()
	cs := NewConfigStore(dir)
	want := &DriftWatch{
		Mode: "fix",
		Schedule: &PullSchedule{
			Mode:      "daily",
			Time:      "03:30",
			DayOfWeek: 0,
		},
		LastRun: "2026-05-24T03:30:00Z",
		NextRun: "2026-05-25T03:30:00Z",
		LastResult: &DriftRunResult{
			DriftsDetected: 3,
			DriftsFixed:    2,
			Errors:         []string{"radarr-4k: instance unreachable"},
		},
	}
	if err := cs.Update(func(cfg *Config) {
		cfg.DriftWatch = want
	}); err != nil {
		t.Fatalf("Update(): %v", err)
	}

	cs2 := NewConfigStore(dir)
	if err := cs2.Load(); err != nil {
		t.Fatalf("Load(): %v", err)
	}
	got := cs2.Get().DriftWatch
	if got == nil {
		t.Fatal("DriftWatch lost across reload")
	}
	if got.Mode != want.Mode || got.LastRun != want.LastRun || got.NextRun != want.NextRun {
		t.Errorf("DriftWatch roundtrip mismatch:\n got=%+v\nwant=%+v", got, want)
	}
	if got.Schedule == nil || got.Schedule.Time != want.Schedule.Time {
		t.Errorf("Schedule roundtrip mismatch: got=%+v want=%+v", got.Schedule, want.Schedule)
	}
	if got.LastResult == nil || got.LastResult.DriftsDetected != 3 || got.LastResult.DriftsFixed != 2 {
		t.Errorf("LastResult roundtrip lost: %+v", got.LastResult)
	}
	if len(got.LastResult.Errors) != 1 || got.LastResult.Errors[0] != want.LastResult.Errors[0] {
		t.Errorf("LastResult.Errors roundtrip lost: %+v", got.LastResult.Errors)
	}
}

func TestDriftWatch_GetReturnsDeepCopy(t *testing.T) {
	dir := t.TempDir()
	cs := NewConfigStore(dir)
	if err := cs.Update(func(cfg *Config) {
		cfg.DriftWatch = &DriftWatch{
			Mode: "detect",
			LastResult: &DriftRunResult{
				DriftsDetected: 1,
				Errors:         []string{"original"},
			},
		}
	}); err != nil {
		t.Fatalf("Update(): %v", err)
	}

	got := cs.Get().DriftWatch
	got.Mode = "fix"
	got.LastResult.DriftsDetected = 99
	got.LastResult.Errors[0] = "mutated"

	again := cs.Get().DriftWatch
	if again.Mode == "fix" {
		t.Error("Get() returned shallow copy — mutating Mode leaked to store")
	}
	if again.LastResult.DriftsDetected == 99 {
		t.Error("Get() returned shallow LastResult — mutating DriftsDetected leaked to store")
	}
	if again.LastResult.Errors[0] == "mutated" {
		t.Error("Get() returned shallow LastResult.Errors — mutating slice leaked to store")
	}
}

func TestUpdateWatch_PersistsAndRoundTrips(t *testing.T) {
	dir := t.TempDir()
	cs := NewConfigStore(dir)
	want := &UpdateWatch{
		Enabled:      true,
		LastRun:      "2026-05-24T12:00:00Z",
		UpstreamHead: "abc1234",
		LocalHead:    "def5678",
		PendingChanges: []ChangeSummary{
			{
				Type:          "cf",
				TrashID:       "trash-id-1",
				Name:          "DD+",
				AffectedRules: []string{"rule-A", "rule-B"},
			},
			{
				Type:            "qs",
				TrashID:         "qs-movie",
				Name:            "Movie QS",
				AffectedQSTypes: []string{"movie"},
			},
		},
	}
	if err := cs.Update(func(cfg *Config) {
		cfg.UpdateWatch = want
	}); err != nil {
		t.Fatalf("Update(): %v", err)
	}

	cs2 := NewConfigStore(dir)
	if err := cs2.Load(); err != nil {
		t.Fatalf("Load(): %v", err)
	}
	got := cs2.Get().UpdateWatch
	if got == nil {
		t.Fatal("UpdateWatch lost across reload")
	}
	if got.Enabled != want.Enabled || got.UpstreamHead != want.UpstreamHead {
		t.Errorf("UpdateWatch roundtrip mismatch:\n got=%+v\nwant=%+v", got, want)
	}
	if len(got.PendingChanges) != 2 {
		t.Fatalf("want 2 PendingChanges, got %d", len(got.PendingChanges))
	}
	if got.PendingChanges[0].Name != "DD+" || len(got.PendingChanges[0].AffectedRules) != 2 {
		t.Errorf("PendingChanges[0] roundtrip lost: %+v", got.PendingChanges[0])
	}
	if got.PendingChanges[1].Type != "qs" || got.PendingChanges[1].AffectedQSTypes[0] != "movie" {
		t.Errorf("PendingChanges[1] roundtrip lost: %+v", got.PendingChanges[1])
	}
}

func TestUpdateWatch_GetReturnsDeepCopy(t *testing.T) {
	dir := t.TempDir()
	cs := NewConfigStore(dir)
	if err := cs.Update(func(cfg *Config) {
		cfg.UpdateWatch = &UpdateWatch{
			Enabled: true,
			PendingChanges: []ChangeSummary{
				{Type: "cf", TrashID: "original", AffectedRules: []string{"rule-1"}},
			},
		}
	}); err != nil {
		t.Fatalf("Update(): %v", err)
	}

	got := cs.Get().UpdateWatch
	got.PendingChanges[0].TrashID = "mutated"
	got.PendingChanges[0].AffectedRules[0] = "mutated-rule"

	again := cs.Get().UpdateWatch
	if again.PendingChanges[0].TrashID == "mutated" {
		t.Error("Get() returned shallow PendingChanges — TrashID mutation leaked")
	}
	if again.PendingChanges[0].AffectedRules[0] == "mutated-rule" {
		t.Error("Get() returned shallow AffectedRules slice — mutation leaked")
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

// TestUpdateWatch_EmptyPendingChangesRoundTrip locks the omitempty
// behaviour: writing an empty (non-nil) slice should marshal to nothing
// (per omitempty), and reload should produce either nil OR empty —
// both are acceptable, just not corrupted.
func TestUpdateWatch_EmptyPendingChangesRoundTrip(t *testing.T) {
	dir := t.TempDir()
	cs := NewConfigStore(dir)
	if err := cs.Update(func(cfg *Config) {
		cfg.UpdateWatch = &UpdateWatch{
			Enabled:        true,
			PendingChanges: []ChangeSummary{},
		}
	}); err != nil {
		t.Fatalf("Update(): %v", err)
	}
	cs2 := NewConfigStore(dir)
	if err := cs2.Load(); err != nil {
		t.Fatalf("Load(): %v", err)
	}
	got := cs2.Get().UpdateWatch
	if got == nil {
		t.Fatal("UpdateWatch lost across reload with empty PendingChanges")
	}
	if !got.Enabled {
		t.Error("Enabled flag lost across reload")
	}
	if len(got.PendingChanges) != 0 {
		t.Errorf("want empty PendingChanges, got %d entries", len(got.PendingChanges))
	}
}
