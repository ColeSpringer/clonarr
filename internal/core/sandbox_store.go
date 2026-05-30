package core

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// SandboxState is the on-disk shape for a scoring-sandbox session.
// Persisted per app type so it survives browser clears, browser swaps,
// container migrations, and host-level /config backups.
//
// Only the *stable* user data is stored: release titles and the named
// score sets that bundle them. Everything else (parsed quality, matched
// CFs, per-profile score) is derived on demand from Arr's /parse
// endpoint and changes as soon as the user picks a different profile,
// so persisting it would be both wasteful and misleading. The file
// stays small enough to share by email or paste back into bulk-import:
// 1000 titles is ~80 KB, well below any practical sharing limit.
//
// Results is the legacy field name for the pre-2026-05-31 fat shape
// (full parsed records per title). Kept readable so existing server
// files migrate transparently on first load — GetState extracts the
// title strings from any Results array it finds and presents them as
// Titles. The next write produces the new compact shape and Results
// disappears (`omitempty` keeps it out of the JSON when zero-valued).
type SandboxState struct {
	Titles    []string          `json:"titles"`
	ScoreSets []json.RawMessage `json:"scoreSets"`

	// Results is the legacy fat-shape field. Always read on input; never
	// written on output (the field is cleared after migration so the
	// JSON encoder skips it via omitempty).
	Results []json.RawMessage `json:"results,omitempty"`
}

// SandboxStore persists per-app-type sandbox state to flat JSON files
// under <configDir>/sandbox/. Writes are atomic (temp file + rename) so
// a torn write from a container kill / power loss can never leave a
// half-serialized file. Reads return an empty (zero-value) state when
// the file doesn't exist yet, which the API layer translates to 200 OK
// with empty arrays so the frontend can detect the "no server data
// yet" case for one-time localStorage migration.
type SandboxStore struct {
	dir string
	mu  sync.Mutex
}

func NewSandboxStore(configDir string) *SandboxStore {
	return &SandboxStore{dir: filepath.Join(configDir, "sandbox")}
}

// validAppType rejects path-traversal attempts in the URL parameter
// before we use it as a filename component. Only the two real app
// types are accepted.
func validAppType(appType string) bool {
	return appType == "radarr" || appType == "sonarr"
}

// GetState reads the persisted sandbox state for the given app type.
// Returns a zero-value SandboxState (not an error) when the file is
// missing or empty — that maps cleanly to the "fresh install / no
// data yet" case the migration path checks for.
func (s *SandboxStore) GetState(appType string) (SandboxState, error) {
	if !validAppType(appType) {
		return SandboxState{}, fmt.Errorf("invalid app type %q", appType)
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	path := filepath.Join(s.dir, appType+".json")
	data, err := os.ReadFile(path) //nolint:gosec // appType is allowlisted above; path is constructed, not user-supplied
	if err != nil {
		if os.IsNotExist(err) {
			return SandboxState{}, nil
		}
		return SandboxState{}, fmt.Errorf("read sandbox state: %w", err)
	}
	if len(data) == 0 {
		return SandboxState{}, nil
	}
	var out SandboxState
	if err := json.Unmarshal(data, &out); err != nil {
		// A corrupt JSON file would otherwise lose all sandbox data on
		// next read. Surface the error so the API can return 500 and the
		// frontend can fall back to its localStorage cache instead of
		// silently presenting an empty list as if the server had been
		// reset.
		return SandboxState{}, fmt.Errorf("parse sandbox state: %w", err)
	}
	// Legacy-shape auto-migration: a file written by the pre-2026-05-31
	// build has a populated Results array and no Titles. Extract each
	// result's title field and present the state as if the file had
	// always been the new compact shape. The next save writes the new
	// shape and the legacy data falls away.
	if len(out.Titles) == 0 && len(out.Results) > 0 {
		extracted := make([]string, 0, len(out.Results))
		for _, raw := range out.Results {
			var r struct {
				Title string `json:"title"`
			}
			if err := json.Unmarshal(raw, &r); err == nil && r.Title != "" {
				extracted = append(extracted, r.Title)
			}
		}
		out.Titles = extracted
		out.Results = nil
	}
	return out, nil
}

// SaveState writes the new state atomically. Creates the sandbox
// directory on first use. The temp-file pattern guarantees a partial
// write can never replace the previous good file: the rename either
// fully succeeds or the old file stays intact.
func (s *SandboxStore) SaveState(appType string, state SandboxState) error {
	if !validAppType(appType) {
		return fmt.Errorf("invalid app type %q", appType)
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return fmt.Errorf("create sandbox dir: %w", err)
	}

	// Indent the on-disk file so a power user opening it directly via
	// a text editor / SMB share / `cat` sees one field per line and can
	// scan a 500-title set without horizontal scrolling. The ~10-15%
	// size overhead vs. compact JSON is fine for the practical ceiling
	// (a few MB) and round-trips identically.
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode sandbox state: %w", err)
	}

	finalPath := filepath.Join(s.dir, appType+".json")
	tmpPath := finalPath + ".tmp"
	// 0644 (not the 0600 used by clonarr.json) — sandbox state holds
	// release titles + parsed quality + matched CF names + scores. None
	// of that is credential-bearing, and power users want to read /
	// share / diff the file from a regular shell or SMB mount where
	// they aren't running as nobody. Owner-only writes still apply.
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil { //nolint:gosec // non-sensitive sandbox data; world-readable matches the user-facing share-and-diff workflow
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := os.Rename(tmpPath, finalPath); err != nil {
		_ = os.Remove(tmpPath) // best-effort cleanup; not fatal if it lingers
		return fmt.Errorf("rename temp file: %w", err)
	}
	return nil
}
