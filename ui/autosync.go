package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

// autoSyncAfterPull runs after a successful TRaSH repo pull.
// For each enabled rule, checks if the repo commit changed since last sync,
// builds a dry-run plan, and applies if there are actual changes.
func (app *App) autoSyncAfterPull() {
	cfg := app.config.Get()
	if len(cfg.AutoSync.Rules) == 0 {
		return
	}

	currentCommit := app.trash.CurrentCommit()
	if currentCommit == "" {
		return
	}

	for _, rule := range cfg.AutoSync.Rules {
		if !rule.Enabled {
			continue
		}
		if rule.LastSyncCommit == currentCommit {
			continue // no repo changes since last sync
		}

		app.runAutoSyncRule(rule, currentCommit)
	}
}

// runAutoSyncRule evaluates and applies a single auto-sync rule.
func (app *App) runAutoSyncRule(rule AutoSyncRule, currentCommit string) {
	inst, ok := app.config.GetInstance(rule.InstanceID)
	if !ok {
		log.Printf("Auto-sync: skipping rule %s — instance %s not found", rule.ID, rule.InstanceID)
		return
	}

	// Per-instance mutex — skip if manual sync is running
	mu := getSyncMutex(inst.ID)
	if !mu.TryLock() {
		log.Printf("Auto-sync: skipping rule %s — sync already in progress for %s", rule.ID, inst.Name)
		return
	}
	defer mu.Unlock()

	log.Printf("Auto-sync: evaluating rule %s (instance=%s, profile=%s)", rule.ID, inst.Name, rule.TrashProfileID)
	app.debugLog.Logf(LogAutoSync, "Rule %s: evaluating %q → %s", rule.ID, rule.TrashProfileID, inst.Name)

	ad := app.trash.GetAppData(inst.Type)
	if ad == nil {
		app.updateAutoSyncRuleError(rule.ID, "no TRaSH data for "+inst.Type)
		return
	}

	// Build sync request from rule
	req := SyncRequest{
		InstanceID:     rule.InstanceID,
		ProfileTrashID: rule.TrashProfileID,
		ArrProfileID:   rule.ArrProfileID,
		SelectedCFs:    rule.SelectedCFs,
		Behavior:       rule.Behavior,
	}
	if rule.ProfileSource == "imported" {
		req.ImportedProfileID = rule.ImportedProfileID
	}

	// Resolve imported profile if needed
	var imported *ImportedProfile
	if req.ImportedProfileID != "" {
		p, ok := app.profiles.Get(req.ImportedProfileID)
		if !ok {
			app.updateAutoSyncRuleError(rule.ID, "imported profile not found: "+req.ImportedProfileID)
			return
		}
		imported = &p
	}

	// Dry-run plan
	customCFs := app.customCFs.List(inst.Type)
	lastSyncedCFs := app.getLastSyncedCFs(req.InstanceID, req.ProfileTrashID, req.Behavior)
	behavior := ResolveSyncBehavior(req.Behavior)
	plan, err := BuildSyncPlan(ad, inst, req, imported, customCFs, lastSyncedCFs)
	if err != nil {
		errMsg := fmt.Sprintf("plan failed: %v", err)
		log.Printf("Auto-sync: rule %s — %s", rule.ID, errMsg)
		app.debugLog.Logf(LogError, "Auto-sync rule %s: plan failed: %s", rule.ID, errMsg)
		app.updateAutoSyncRuleError(rule.ID, errMsg)
		app.notifyAutoSync(rule, inst, nil, fmt.Errorf("%s", errMsg))
		return
	}

	if !plan.HasChanges() {
		// No actual changes — update commit hash, clear error
		log.Printf("Auto-sync: rule %s — no changes for %s", rule.ID, inst.Name)
		app.debugLog.Logf(LogAutoSync, "Rule %s: no changes for %s", rule.ID, inst.Name)
		app.updateAutoSyncRuleCommit(rule.ID, currentCommit)
		return
	}

	// Apply
	result, err := ExecuteSyncPlan(ad, inst, req, plan, imported, customCFs, behavior)
	if err != nil {
		errMsg := fmt.Sprintf("apply failed: %v", err)
		log.Printf("Auto-sync: rule %s — %s", rule.ID, errMsg)
		app.debugLog.Logf(LogError, "Auto-sync rule %s: apply failed: %s", rule.ID, errMsg)
		app.updateAutoSyncRuleError(rule.ID, errMsg)
		app.notifyAutoSync(rule, inst, nil, fmt.Errorf("%s", errMsg))
		return
	}

	log.Printf("Auto-sync: rule %s applied — %d CFs created, %d updated, %d scores on %s",
		rule.ID, result.CFsCreated, result.CFsUpdated, result.ScoresUpdated, inst.Name)
	app.debugLog.Logf(LogAutoSync, "Rule %s: applied — %d created, %d updated, %d scores on %s",
		rule.ID, result.CFsCreated, result.CFsUpdated, result.ScoresUpdated, inst.Name)

	app.updateAutoSyncRuleCommit(rule.ID, currentCommit)

	// Update sync history (same as manual sync)
	allCFIDs := make([]string, 0)
	for _, a := range plan.CFActions {
		allCFIDs = append(allCFIDs, a.TrashID)
	}
	entry := SyncHistoryEntry{
		InstanceID:     inst.ID,
		ProfileTrashID: req.ProfileTrashID,
		ProfileName:    plan.ProfileName,
		ArrProfileID:   req.ArrProfileID,
		ArrProfileName: plan.ArrProfileName,
		SyncedCFs:      allCFIDs,
		CFsCreated:     result.CFsCreated,
		CFsUpdated:     result.CFsUpdated,
		ScoresUpdated:  result.ScoresUpdated,
		LastSync:       time.Now().Format(time.RFC3339),
	}
	if result.ProfileCreated {
		entry.ArrProfileID = result.ArrProfileID
		entry.ArrProfileName = result.ArrProfileName
	}
	if err := app.config.UpsertSyncHistory(entry); err != nil {
		log.Printf("Auto-sync: failed to save sync history: %v", err)
	}

	app.notifyAutoSync(rule, inst, result, nil)
}

// updateAutoSyncRuleCommit updates the last sync commit and clears error for a rule.
func (app *App) updateAutoSyncRuleCommit(ruleID, commit string) {
	app.config.Update(func(cfg *Config) {
		for i := range cfg.AutoSync.Rules {
			if cfg.AutoSync.Rules[i].ID == ruleID {
				cfg.AutoSync.Rules[i].LastSyncCommit = commit
				cfg.AutoSync.Rules[i].LastSyncTime = time.Now().Format(time.RFC3339)
				cfg.AutoSync.Rules[i].LastSyncError = ""
				return
			}
		}
	})
}

// updateAutoSyncRuleError sets the last error for a rule (does NOT update commit).
func (app *App) updateAutoSyncRuleError(ruleID, errMsg string) {
	app.config.Update(func(cfg *Config) {
		for i := range cfg.AutoSync.Rules {
			if cfg.AutoSync.Rules[i].ID == ruleID {
				cfg.AutoSync.Rules[i].LastSyncError = errMsg
				cfg.AutoSync.Rules[i].LastSyncTime = time.Now().Format(time.RFC3339)
				return
			}
		}
	})
}

// notifyAutoSync sends Discord notification for auto-sync result.
func (app *App) notifyAutoSync(rule AutoSyncRule, inst Instance, result *SyncResult, syncErr error) {
	cfg := app.config.Get()
	if cfg.AutoSync.DiscordWebhook == "" {
		return
	}
	if syncErr != nil && !cfg.AutoSync.NotifyOnFailure {
		return
	}
	if syncErr == nil && !cfg.AutoSync.NotifyOnSuccess {
		return
	}

	var color int
	var title, description string

	if syncErr != nil {
		color = 0xf85149 // red
		title = "Auto-Sync Failed"
		description = fmt.Sprintf("**Instance:** %s\n**Profile:** %s\n**Error:** %s",
			inst.Name, rule.TrashProfileID, syncErr.Error())
	} else {
		color = 0x3fb950 // green
		title = "Auto-Sync Applied"
		description = fmt.Sprintf("**Instance:** %s\n**Profile:** %s\n**CFs:** %d created, %d updated\n**Scores:** %d updated",
			inst.Name, rule.TrashProfileID,
			result.CFsCreated, result.CFsUpdated, result.ScoresUpdated)
	}

	embed := map[string]any{
		"title":       title,
		"description": description,
		"color":       color,
		"footer":      map[string]string{"text": "Clonarr by ProphetSe7en"},
	}
	payload, err := json.Marshal(map[string]any{"embeds": []any{embed}})
	if err != nil {
		log.Printf("Auto-sync: failed to marshal Discord payload: %v", err)
		return
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(cfg.AutoSync.DiscordWebhook, "application/json", bytes.NewReader(payload))
	if err != nil {
		log.Printf("Auto-sync: Discord notification failed: %v", err)
		return
	}
	resp.Body.Close()
}

// notifyRepoUpdate sends a Discord notification when the TRaSH repo has new commits.
// Includes changelog entries from updates.txt for human-readable context.
func (app *App) notifyRepoUpdate(prevCommit, newCommit string) {
	cfg := app.config.Get()
	if !cfg.AutoSync.NotifyOnRepoUpdate {
		return
	}
	webhook := cfg.AutoSync.DiscordWebhook
	if webhook == "" {
		return
	}

	description := fmt.Sprintf("**Commit:** `%s` → `%s`", prevCommit, newCommit)

	// Build changelog from parsed updates.txt (already available after pull)
	status := app.trash.Status()
	if len(status.Changelog) > 0 {
		section := status.Changelog[0] // latest section
		description += fmt.Sprintf("\n\n**Changes** — %s", section.Date)
		for _, e := range section.Entries {
			icon := "🔧"
			if e.Type == "feat" {
				icon = "✨"
			} else if e.Type == "refactor" {
				icon = "♻️"
			}
			line := fmt.Sprintf("\n%s **%s:** %s", icon, e.Scope, e.Msg)
			if e.PR != "" {
				line += fmt.Sprintf(" ([#%s](https://github.com/TRaSH-Guides/Guides/pull/%s))", e.PR, e.PR)
			}
			description += line
		}
	}

	embed := map[string]any{
		"title":       "TRaSH Guides Updated",
		"description": description,
		"color":       0x58a6ff, // blue
		"footer":      map[string]string{"text": "Clonarr by ProphetSe7en"},
	}
	payload, err := json.Marshal(map[string]any{"embeds": []any{embed}})
	if err != nil {
		log.Printf("Repo update: failed to marshal Discord payload: %v", err)
		return
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(webhook, "application/json", bytes.NewReader(payload))
	if err != nil {
		log.Printf("Repo update: Discord notification failed: %v", err)
		return
	}
	resp.Body.Close()
	log.Printf("Repo update: Discord notification sent (%s → %s)", prevCommit, newCommit)
}
