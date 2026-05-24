package api

import (
	"net/http"

	"clonarr/internal/core"
)

// --- Watch & Drift (Phase 2a) — UpdateWatch endpoints ---

// handleGetUpdateWatch returns the current UpdateWatch state. When the
// subsystem has never run (UpdateWatch == nil in config), returns a default
// shape so the frontend can render a consistent layout.
func (s *Server) handleGetUpdateWatch(w http.ResponseWriter, r *http.Request) {
	cfg := s.Core.Config.Get()
	if cfg.UpdateWatch == nil {
		writeJSON(w, map[string]any{
			"enabled":      false,
			"lastRun":      "",
			"upstreamHead": "",
			"localHead":    "",
		})
		return
	}
	writeJSON(w, cfg.UpdateWatch)
}

// handlePutUpdateWatch toggles the Enabled flag. Other fields (LastRun, heads,
// pending changes) are managed by UpdateWatcher.Run — clients cannot set them
// directly to avoid drifting display state.
func (s *Server) handlePutUpdateWatch(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeJSON[struct {
		Enabled *bool `json:"enabled,omitempty"`
	}](w, r, 4096)
	if !ok {
		return
	}
	if req.Enabled == nil {
		writeError(w, 400, "enabled field required")
		return
	}
	if err := s.Core.Config.Update(func(cfg *core.Config) {
		if cfg.UpdateWatch == nil {
			cfg.UpdateWatch = &core.UpdateWatch{}
		}
		cfg.UpdateWatch.Enabled = *req.Enabled
	}); err != nil {
		writeError(w, 500, "failed to save update-watch setting")
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}
