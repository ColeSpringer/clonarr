package api

import (
	"net/http"
	"time"

	"clonarr/internal/core"
)

// handleWidgetSummary returns a stable, integration-friendly snapshot for
// external dashboards (homepage, glance, etc). Fields here are an explicit
// contract — never remove or rename without a major version bump. Add new
// fields freely. Auth honours the standard middleware, so X-Api-Key works.
//
// Why this exists instead of pointing integrators at /api/config + /api/auto-sync/rules + /api/trash/status:
//   - /api/config returns deep-mask'd instance secrets but also a large
//     payload (custom CFs, sync history, etc.) that integrators don't need.
//   - The "Auto-Sync active" indicator in the v3 UI is derived from rule
//     count + per-instance pause state, NOT cfg.AutoSync.Enabled — that
//     mismatch confused at least one third-party widget author (homepage
//     discussion #6618). This endpoint surfaces the derived state directly.
//   - "Next sync" requires picking between PullSchedule (TRaSH pull) and
//     SyncSchedule (independent force-sync). Resolving that on the client
//     means re-implementing nextPullTimeAt + nextSyncTimeAt. We compute
//     both here and let integrators show whichever they want.
func (s *Server) handleWidgetSummary(w http.ResponseWriter, r *http.Request) {
	cfg := s.Core.Config.Get()
	trashSt := s.Core.Trash.Status()

	type instanceSummary struct {
		ID             string `json:"id"`
		Name           string `json:"name"`
		Type           string `json:"type"`
		AutoSyncPaused bool   `json:"autoSyncPaused"`
	}
	type ruleSummary struct {
		InstanceID     string `json:"instanceId"`
		InstanceName   string `json:"instanceName"`
		InstanceType   string `json:"instanceType"`
		TrashProfileID string `json:"trashProfileId,omitempty"`
		ProfileName    string `json:"profileName"`
		ArrProfileID   int    `json:"arrProfileId"`
		ArrProfileName string `json:"arrProfileName,omitempty"`
		LastSyncTime   string `json:"lastSyncTime,omitempty"`
		LastSyncError  string `json:"lastSyncError,omitempty"`
		Orphaned       bool   `json:"orphaned"`
	}

	// Build name lookup keyed by (instanceID, arrProfileID) from the most-recent
	// matching history entry. SyncHistory is append-only; later entries win.
	type histKey struct {
		instanceID   string
		arrProfileID int
	}
	histNames := make(map[histKey][2]string, len(cfg.SyncHistory))
	for _, h := range cfg.SyncHistory {
		histNames[histKey{h.InstanceID, h.ArrProfileID}] = [2]string{h.ProfileName, h.ArrProfileName}
	}

	instances := make([]instanceSummary, 0, len(cfg.Instances))
	instanceByID := make(map[string]core.Instance, len(cfg.Instances))
	var radarrCount, sonarrCount, pausedCount int
	for _, inst := range cfg.Instances {
		instances = append(instances, instanceSummary{
			ID: inst.ID, Name: inst.Name, Type: inst.Type, AutoSyncPaused: inst.AutoSyncPaused,
		})
		instanceByID[inst.ID] = inst
		switch inst.Type {
		case "radarr":
			radarrCount++
		case "sonarr":
			sonarrCount++
		}
		if inst.AutoSyncPaused {
			pausedCount++
		}
	}

	rules := make([]ruleSummary, 0, len(cfg.AutoSync.Rules))
	var ruleActive, ruleOrphaned, ruleWithErrors int
	var mostRecentSync time.Time
	var mostRecentSyncRaw string
	var firstError string
	for _, r := range cfg.AutoSync.Rules {
		inst := instanceByID[r.InstanceID]
		profileName := r.TrashProfileID
		var arrName string
		if names, ok := histNames[histKey{r.InstanceID, r.ArrProfileID}]; ok {
			if names[0] != "" {
				profileName = names[0]
			}
			arrName = names[1]
		}
		rs := ruleSummary{
			InstanceID:     r.InstanceID,
			InstanceName:   inst.Name,
			InstanceType:   inst.Type,
			TrashProfileID: r.TrashProfileID,
			ProfileName:    profileName,
			ArrProfileID:   r.ArrProfileID,
			ArrProfileName: arrName,
			LastSyncTime:   r.LastSyncTime,
			LastSyncError:  r.LastSyncError,
			Orphaned:       r.OrphanedAt != "",
		}
		rules = append(rules, rs)
		if rs.Orphaned {
			ruleOrphaned++
			continue
		}
		if r.Enabled {
			ruleActive++
		}
		if r.LastSyncError != "" {
			ruleWithErrors++
			if firstError == "" {
				firstError = r.LastSyncError
			}
		}
		if r.LastSyncTime != "" {
			if t, err := time.Parse(time.RFC3339, r.LastSyncTime); err == nil && t.After(mostRecentSync) {
				mostRecentSync = t
				mostRecentSyncRaw = r.LastSyncTime
			}
		}
	}

	radarrRules := make([]ruleSummary, 0)
	sonarrRules := make([]ruleSummary, 0)
	for _, r := range rules {
		if r.Orphaned {
			continue
		}
		switch r.InstanceType {
		case "radarr":
			radarrRules = append(radarrRules, r)
		case "sonarr":
			sonarrRules = append(sonarrRules, r)
		}
	}

	var nextPull, nextSync string
	if t := s.nextPullForStatus(); !t.IsZero() {
		nextPull = t.Format(time.RFC3339)
	}
	if cfg.SyncSchedule != nil {
		if t := core.NextSyncTime(*cfg.SyncSchedule); !t.IsZero() {
			nextSync = t.Format(time.RFC3339)
		}
	}

	writeJSON(w, map[string]any{
		"version":  s.Core.Version,
		"serverNow": time.Now().Format(time.RFC3339),

		"trash": map[string]any{
			"commit":       trashSt.CommitHash,
			"commitDate":   trashSt.CommitDate,
			"lastPull":     trashSt.LastPull,
			"nextPull":     nextPull,
			"radarrCFs":    trashSt.RadarrCFs,
			"sonarrCFs":    trashSt.SonarrCFs,
			"radarrGroups": trashSt.RadarrGroups,
			"sonarrGroups": trashSt.SonarrGroups,
		},

		"autoSync": map[string]any{
			"enabled":  cfg.AutoSync.Enabled,
			"paused":   cfg.AutoSync.Paused,
			"nextSync": nextSync,
			"lastSync": mostRecentSyncRaw,
			"lastError": firstError,
		},

		"instances": map[string]any{
			"total":  len(cfg.Instances),
			"radarr": radarrCount,
			"sonarr": sonarrCount,
			"paused": pausedCount,
			"list":   instances,
		},

		"rules": map[string]any{
			"total":      len(cfg.AutoSync.Rules),
			"active":     ruleActive,
			"orphaned":   ruleOrphaned,
			"withErrors": ruleWithErrors,
			"list":        rules,
			"radarrList":  radarrRules,
			"sonarrList":  sonarrRules,
			"radarrTotal": len(radarrRules),
			"sonarrTotal": len(sonarrRules),
		},
	})
}
