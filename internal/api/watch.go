package api

import (
	"net/http"

	"clonarr/internal/core"
)

// --- Profile Sync — config + telemetry endpoints ---

// handleGetProfileSync returns the current ProfileSync state (settings +
// runner telemetry). When the subsystem has never been initialised by
// migration (ProfileSync == nil), returns a sensible default shape so
// frontend can render consistently.
func (s *Server) handleGetProfileSync(w http.ResponseWriter, r *http.Request) {
	cfg := s.Core.Config.Get()
	if cfg.ProfileSync == nil {
		// Return all fields with zero values so frontend doesn't need
		// optional-chaining on every read. Matches the shape Phase B will
		// populate via migration.
		writeJSON(w, core.ProfileSync{})
		return
	}
	writeJSON(w, cfg.ProfileSync)
}

// handlePutProfileSync updates user-controlled settings: Sources, Mode,
// Interval/Specific (Schedule), ApplyInterval/ApplySpecific (Apply schedule).
// Runner-managed fields (LastRun, UpstreamHead, LocalHead, LastResult) are
// read-only via API to prevent drifting display state.
func (s *Server) handlePutProfileSync(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeJSON[struct {
		Interval      *string                     `json:"interval,omitempty"`
		Specific      *core.PullSchedule          `json:"specific,omitempty"`
		Sources       *core.ProfileSyncSources    `json:"sources,omitempty"`
		Mode          *string                     `json:"mode,omitempty"`
		ApplyInterval *string                     `json:"applyInterval,omitempty"`
		ApplySpecific *core.PullSchedule          `json:"applySpecific,omitempty"`
	}](w, r, 8192)
	if !ok {
		return
	}
	// Boundary validation — reject invalid values now rather than letting
	// the runner silently no-op on garbage Mode strings or trip on bad
	// schedule durations.
	if req.Mode != nil && *req.Mode != "" && !core.IsValidProfileSyncMode(*req.Mode) {
		writeError(w, 400, "invalid mode (must be empty, 'auto', 'notify', or 'delayed')")
		return
	}
	if err := s.Core.Config.Update(func(cfg *core.Config) {
		if cfg.ProfileSync == nil {
			cfg.ProfileSync = &core.ProfileSync{}
		}
		if req.Interval != nil {
			cfg.ProfileSync.Interval = *req.Interval
		}
		if req.Specific != nil {
			cfg.ProfileSync.Specific = req.Specific
		}
		if req.Sources != nil {
			cfg.ProfileSync.Sources = *req.Sources
		}
		if req.Mode != nil {
			cfg.ProfileSync.Mode = *req.Mode
		}
		if req.ApplyInterval != nil {
			cfg.ProfileSync.ApplyInterval = *req.ApplyInterval
		}
		if req.ApplySpecific != nil {
			cfg.ProfileSync.ApplySpecific = req.ApplySpecific
		}
	}); err != nil {
		writeError(w, 500, "failed to save profile-sync settings")
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}
