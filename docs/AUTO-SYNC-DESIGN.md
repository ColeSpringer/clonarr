# Auto-Sync Design

Automatic synchronization when TRaSH Guides repo data changes.

## Overview

After each TRaSH repo pull (scheduled or manual), detect changes and automatically apply sync plans to configured instances. Users configure auto-sync per profile+instance pair with optional Discord notifications.

## Data Model

```go
// AutoSyncRule defines one auto-sync binding
type AutoSyncRule struct {
    ID            string `json:"id"`            // UUID
    Enabled       bool   `json:"enabled"`
    InstanceID    string `json:"instanceId"`    // target Arr instance
    ProfileSource string `json:"profileSource"` // "trash" or "imported"
    TrashProfileID string `json:"trashProfileId,omitempty"` // TRaSH profile trash_id
    ImportedProfileID string `json:"importedProfileId,omitempty"` // imported profile ID
    ScoreSet      string `json:"scoreSet,omitempty"`      // e.g. "default", "sqp-1"
    SyncMode      string `json:"syncMode"`      // "create-update" or "update-only"
    LastSyncCommit string `json:"lastSyncCommit,omitempty"` // git commit hash of last successful sync
    LastSyncTime   string `json:"lastSyncTime,omitempty"`   // ISO 8601
    LastSyncError  string `json:"lastSyncError,omitempty"`
}
```

Config extension in `clonarr.json`:
```json
{
  "autoSync": {
    "enabled": false,
    "notifyOnSuccess": true,
    "notifyOnFailure": true,
    "discordWebhook": "",
    "rules": []
  }
}
```

## Change Detection

After each TRaSH repo pull:

1. Get current HEAD commit hash (`git rev-parse HEAD`)
2. For each enabled auto-sync rule:
   a. Compare `lastSyncCommit` with current HEAD
   b. If different (or never synced), run dry-run plan
   c. If plan has changes → execute apply
   d. Update `lastSyncCommit` and `lastSyncTime` on success
3. Short-circuit: if no rule has a stale commit, skip entirely

This is commit-level detection — any repo change triggers evaluation. The dry-run plan itself determines if the specific profile has actual changes (no changes = no apply).

## Backend

### New file: `autosync.go`

```go
// autoSyncAfterPull runs after successful TRaSH repo pull
func (app *App) autoSyncAfterPull() {
    cfg := app.config.Get()
    if !cfg.AutoSync.Enabled || len(cfg.AutoSync.Rules) == 0 {
        return
    }

    currentCommit := app.trash.CurrentCommit() // git rev-parse HEAD

    for _, rule := range cfg.AutoSync.Rules {
        if !rule.Enabled {
            continue
        }
        if rule.LastSyncCommit == currentCommit {
            continue // no repo changes since last sync
        }

        log.Printf("Auto-sync: evaluating rule %s (instance=%s)", rule.ID, rule.InstanceID)

        plan, err := app.runAutoSyncPlan(rule)
        if err != nil {
            app.updateRuleError(rule.ID, err.Error())
            app.notifyAutoSync(rule, nil, err)
            continue
        }

        if !plan.HasChanges() {
            // No actual changes for this profile — update commit, skip apply
            app.updateRuleCommit(rule.ID, currentCommit)
            continue
        }

        result, err := app.applyAutoSyncPlan(rule, plan)
        if err != nil {
            app.updateRuleError(rule.ID, err.Error())
            app.notifyAutoSync(rule, nil, err)
            continue
        }

        app.updateRuleCommit(rule.ID, currentCommit)
        app.notifyAutoSync(rule, result, nil)
        log.Printf("Auto-sync: applied rule %s — %d CFs, %d profiles",
            rule.ID, len(result.CFsCreated)+len(result.CFsUpdated), len(result.ProfilesUpdated))
    }
}
```

### Integration points

Call `app.autoSyncAfterPull()` from:
- `main.go` scheduled pull goroutine (after successful `CloneOrPull`)
- `handleTrashPull` handler (after manual pull)

Already partially implemented: `app.autoSyncQualitySizes()` follows the same pattern.

### API endpoints (5 new)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/auto-sync/rules` | List all rules |
| POST | `/api/auto-sync/rules` | Create rule |
| PUT | `/api/auto-sync/rules/{id}` | Update rule |
| DELETE | `/api/auto-sync/rules/{id}` | Delete rule |
| PUT | `/api/auto-sync/settings` | Update global auto-sync settings |

### Discord notifications

```go
func (app *App) notifyAutoSync(rule AutoSyncRule, result *SyncResult, err error) {
    cfg := app.config.Get()
    if cfg.AutoSync.DiscordWebhook == "" {
        return
    }
    if err != nil && !cfg.AutoSync.NotifyOnFailure {
        return
    }
    if err == nil && !cfg.AutoSync.NotifyOnSuccess {
        return
    }
    // POST embed to webhook — green for success, red for failure
    // Fields: instance name, profile name, CFs created/updated, profiles updated
}
```

## Frontend

### Sync modal toggle

When opening the sync modal for a TRaSH profile, add an "Auto-sync" toggle:

```
[x] Auto-sync this profile
    When TRaSH repo updates, automatically sync changes to this instance.
```

Toggling ON creates an AutoSyncRule. Toggling OFF deletes it. This is the simplest UX — no separate management page needed for basic use.

### Settings tab

Under Settings, add an "Auto-Sync" section:

```
Auto-Sync
  [x] Enabled
  Discord Webhook: [________________________]
  [x] Notify on success
  [x] Notify on failure

  Active Rules:
  ┌──────────────┬───────────────┬──────────────┬────────────┐
  │ Profile      │ Instance      │ Last Sync    │ Status     │
  ├──────────────┼───────────────┼──────────────┼────────────┤
  │ SQP-1        │ Radarr HD     │ 2h ago       │ OK         │
  │ WEB-2160p    │ Sonarr 4K     │ 2h ago       │ OK         │
  └──────────────┴───────────────┴──────────────┴────────────┘

  Each row: [Edit] [Delete] buttons
```

### Imported profile auto-sync

Imported profiles with a `trashProfileId` can also be auto-synced — they reference TRaSH data that may change. Custom profiles (no trashProfileId) cannot be auto-synced since they have no upstream source.

## Edge Cases

1. **Instance unreachable during auto-sync** — Log error, set `lastSyncError`, notify if configured. Do NOT update `lastSyncCommit` so it retries next pull.

2. **Concurrent manual + auto sync** — Per-instance sync mutex (`getSyncMutex`) already prevents this. Auto-sync uses `TryLock` — if locked, skip with log message (manual sync takes priority).

3. **Rule references deleted instance** — Skip rule, log warning. UI should clean up orphaned rules when instance is deleted.

4. **Rule references deleted imported profile** — Same as above. Skip and log.

5. **Multiple rules for same instance** — Allowed. Each rule syncs a different profile. Executed sequentially (same mutex).

6. **Rapid pulls (manual spam)** — Commit hash check short-circuits if already synced. No redundant work.

7. **First-time sync (no lastSyncCommit)** — Always runs. Equivalent to initial manual sync.

## Implementation Phases

All phases implemented in v0.6.0:

1. **Phase 1: Data model + config** — `AutoSyncConfig` + `AutoSyncRule` structs, deep-copy in `config.Get()`
2. **Phase 2: Change detection** — `CurrentCommit()` on trashStore, `HasChanges()` on SyncPlan
3. **Phase 3: Auto-sync engine** — `autosync.go` with dry-run → apply pipeline, Discord notifications
4. **Phase 4: API endpoints** — 6 routes: GET/PUT settings, GET/POST/PUT/DELETE rules
5. **Phase 5: Sync modal toggle** — Checkbox creates/deletes rule per profile+instance
6. **Phase 6: Settings UI** — Rule list with enable/disable/delete, webhook, notification preferences
7. **Phase 7: Discord notifications** — Embedded in Phase 3: green/red embeds with footer branding

### Safety features
- Duplicate rule prevention (409 on same instance+profile)
- Per-instance sync mutex with TryLock (manual sync takes priority)
- 10s timeout on Discord webhook HTTP client
- Sync state preserved on rule update, error cleared on re-enable
- Instance deletion cascades to orphaned auto-sync rules
