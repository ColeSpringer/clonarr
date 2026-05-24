package api

import (
	"clonarr/internal/core"
	"clonarr/internal/utils"
	"errors"
	"net/http"
	"sort"
	"time"
)

// --- TRaSH ---

func (s *Server) handleTrashStatus(w http.ResponseWriter, r *http.Request) {
	st := s.Core.Trash.Status()
	now := time.Now()
	st.ServerNow = now.Format(time.RFC3339)
	// Keep countdown math on the server side so browser timezone settings do not
	// drift from the container's TZ.
	if next := s.nextPullForStatus(); !next.IsZero() {
		st.NextPull = next.Format(time.RFC3339)
		st.NextPullClock = next.Format("15:04")
	}
	writeJSON(w, st)
}

func (s *Server) nextPullForStatus() time.Time {
	cfg := s.Core.Config.Get()
	if cfg.PullInterval == "0" {
		return time.Time{}
	}
	// Single source of truth: the scheduler keeps NextPullAt updated for both
	// interval and "specific" modes via SetNextPullAt. Recomputing here would
	// produce different values 1ms before vs after a fire instant (today's
	// 03:00 vs tomorrow's 03:00) — countdown jitter near fire time. Trust
	// the scheduler's value; fall back to a fresh compute only if the
	// scheduler hasn't initialised yet (boot race).
	if next := s.Core.GetNextPullAt(); !next.IsZero() {
		return next
	}
	if cfg.PullInterval == "specific" {
		return nextSpecificPull(cfg)
	}
	return time.Time{}
}

func nextPullAfterConfigSave(cfg core.Config) time.Time {
	if cfg.PullInterval == "specific" {
		return nextSpecificPull(cfg)
	}
	interval := core.ParsePullInterval(cfg.PullInterval)
	if interval <= 0 {
		return time.Time{}
	}
	return time.Now().Add(interval)
}

func nextSpecificPull(cfg core.Config) time.Time {
	if cfg.PullSchedule == nil {
		return time.Time{}
	}
	return core.NextPullTime(*cfg.PullSchedule)
}

func (s *Server) handleTrashPull(w http.ResponseWriter, r *http.Request) {
	utils.SafeGo("manual-trash-pull", func() {
		// All Pull-and-sync logic (DebugLog tracing, SetPullError on
		// failure, ProfileSync state persistence, DiffPull detail lines,
		// AfterPullCallback, AutoSyncAfterPull) lives in App.RunPullAndSync
		// so manual and scheduled paths can't drift apart. The error is
		// already logged inside RunPullAndSync — we don't need to surface
		// it again at this call site.
		_ = s.Core.RunPullAndSync(core.SourceManualPull)
	})
	w.WriteHeader(http.StatusAccepted)
	writeJSON(w, map[string]string{"status": "pulling"})
}

func (s *Server) handleTrashReset(w http.ResponseWriter, r *http.Request) {
	unlockSyncs, busy := s.Core.TryLockConfiguredSyncs()
	if busy {
		writeError(w, http.StatusConflict, "Sync already in progress; try again after it finishes")
		return
	}
	defer unlockSyncs()

	if err := s.Core.Trash.Reset(); err != nil {
		if errors.Is(err, core.ErrTrashBusy) {
			writeError(w, http.StatusConflict, "TRaSH pull/reset already in progress")
			return
		}
		writeError(w, http.StatusInternalServerError, "Failed to reset TRaSH data: "+err.Error())
		return
	}
	writeJSON(w, map[string]string{"status": "reset"})
}

// shortCommit returns the first 7 characters of a git commit hash for
// inclusion in human-readable log messages. Returns the full string if
// it's already short.
func shortCommit(hash string) string {
	if len(hash) <= 7 {
		return hash
	}
	return hash[:7]
}

func (s *Server) handleTrashCFs(w http.ResponseWriter, r *http.Request) {
	appType := r.PathValue("app")
	if appType != "radarr" && appType != "sonarr" {
		writeError(w, 400, "app must be 'radarr' or 'sonarr'")
		return
	}

	ad := s.Core.Trash.GetAppData(appType)
	if ad == nil {
		writeJSON(w, []any{})
		return
	}

	cfs := make([]*core.TrashCF, 0, len(ad.CustomFormats))
	for _, cf := range ad.CustomFormats {
		cfs = append(cfs, cf)
	}
	writeJSON(w, cfs)
}

// handleTrashScoreContexts returns the distinct trash_scores context keys
// actually used in TRaSH-Guides CFs for the given s.Core. Keeps the Custom Format
// editor's context dropdown in sync with upstream without hardcoding.
func (s *Server) handleTrashScoreContexts(w http.ResponseWriter, r *http.Request) {
	appType := r.PathValue("app")
	if appType != "radarr" && appType != "sonarr" {
		writeError(w, 400, "app must be 'radarr' or 'sonarr'")
		return
	}

	ad := s.Core.Trash.GetAppData(appType)
	if ad == nil {
		writeJSON(w, []string{"default"})
		return
	}

	seen := map[string]struct{}{"default": {}}
	for _, cf := range ad.CustomFormats {
		for k := range cf.TrashScores {
			seen[k] = struct{}{}
		}
	}

	keys := make([]string, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	// Stable ordering: "default" first, then alphabetical.
	sort.Slice(keys, func(i, j int) bool {
		if keys[i] == "default" {
			return true
		}
		if keys[j] == "default" {
			return false
		}
		return keys[i] < keys[j]
	})
	writeJSON(w, keys)
}

func (s *Server) handleTrashCFGroups(w http.ResponseWriter, r *http.Request) {
	appType := r.PathValue("app")
	if appType != "radarr" && appType != "sonarr" {
		writeError(w, 400, "app must be 'radarr' or 'sonarr'")
		return
	}

	ad := s.Core.Trash.GetAppData(appType)
	if ad == nil {
		writeJSON(w, []any{})
		return
	}

	groups := ad.CFGroups
	if groups == nil {
		groups = []*core.TrashCFGroup{}
	}
	writeJSON(w, groups)
}

func (s *Server) handleTrashConflicts(w http.ResponseWriter, r *http.Request) {
	appType := r.PathValue("app")
	if appType != "radarr" && appType != "sonarr" {
		writeError(w, 400, "app must be 'radarr' or 'sonarr'")
		return
	}
	ad := s.Core.Trash.GetAppData(appType)
	if ad == nil || ad.Conflicts == nil {
		writeJSON(w, core.ConflictsData{CustomFormats: [][]core.ConflictEntry{}})
		return
	}
	writeJSON(w, ad.Conflicts)
}

func (s *Server) handleTrashProfiles(w http.ResponseWriter, r *http.Request) {
	appType := r.PathValue("app")
	if appType != "radarr" && appType != "sonarr" {
		writeError(w, 400, "app must be 'radarr' or 'sonarr'")
		return
	}

	ad := s.Core.Trash.GetAppData(appType)
	if ad == nil {
		writeJSON(w, []any{})
		return
	}

	type ProfileListItem struct {
		TrashID          string `json:"trashId"`
		Name             string `json:"name"`
		TrashScoreSet    string `json:"trashScoreSet,omitempty"`
		TrashDescription string `json:"trashDescription,omitempty"`
		TrashURL         string `json:"trashUrl,omitempty"`
		Group            int    `json:"group"`
		GroupName        string `json:"groupName"`
		CFCount          int    `json:"cfCount"`
	}

	groupNames := make(map[string]string) // trash_id → group name
	for _, pg := range ad.ProfileGroups {
		for _, tid := range pg.Profiles {
			groupNames[tid] = pg.Name
		}
	}

	var items []ProfileListItem
	for _, p := range ad.Profiles {
		gn := groupNames[p.TrashID]
		if gn == "" {
			gn = "Other"
		}
		items = append(items, ProfileListItem{
			TrashID:          p.TrashID,
			Name:             p.Name,
			TrashScoreSet:    p.TrashScoreSet,
			TrashDescription: p.TrashDescription,
			TrashURL:         p.TrashURL,
			Group:            p.Group,
			GroupName:        gn,
			CFCount:          len(p.FormatItems),
		})
	}
	writeJSON(w, items)
}

// handleTrashProfileDescriptions returns the auto-derived rich descriptions
// for every TRaSH profile in the app. Built by combining profile JSON +
// cf-group includes + per-app setup-quality-profiles.md sections. The result
// drives the new v3 TRaSH Profiles browse view (compact card layout).
func (s *Server) handleTrashProfileDescriptions(w http.ResponseWriter, r *http.Request) {
	appType := r.PathValue("app")
	if appType != "radarr" && appType != "sonarr" {
		writeError(w, 400, "app must be 'radarr' or 'sonarr'")
		return
	}
	descriptions, err := s.Core.Trash.DescribeProfiles(appType)
	if err != nil {
		writeError(w, 500, "describe profiles: "+err.Error())
		return
	}
	if descriptions == nil {
		writeJSON(w, []any{})
		return
	}
	writeJSON(w, descriptions)
}
