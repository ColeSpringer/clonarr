package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// CustomCF represents a user-imported or user-created custom format not found in TRaSH guides.
type CustomCF struct {
	ID       string `json:"id"`       // synthetic ID: "custom:<hex>"
	Name     string `json:"name"`
	AppType  string `json:"appType"`  // "radarr" or "sonarr"
	Category string `json:"category"` // user-chosen category (default: "Custom")

	// CF definition
	IncludeInRename bool               `json:"includeInRename,omitempty"`
	ArrID           int                `json:"arrId,omitempty"`
	Specifications  []ArrSpecification `json:"specifications,omitempty"`

	// Developer mode: TRaSH guide fields (only populated when devMode is used)
	TrashID     string         `json:"trashId,omitempty"`
	TrashScores map[string]int `json:"trashScores,omitempty"`
	Description string         `json:"description,omitempty"`

	// Source info
	SourceInstance string `json:"sourceInstance,omitempty"` // instance name it was imported from
	ImportedAt     string `json:"importedAt,omitempty"`     // RFC3339
}

// customCFStore manages custom CFs as individual JSON files in a directory.
type customCFStore struct {
	mu  sync.RWMutex
	dir string // e.g. /config/custom-cfs
}

func newCustomCFStore(dir string) *customCFStore {
	return &customCFStore{dir: dir}
}

// generateCustomID creates a synthetic ID like "custom:a1b2c3d4e5f6".
func generateCustomID() string {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		// fallback: should never happen
		return "custom:fallback"
	}
	return "custom:" + hex.EncodeToString(b)
}

// Add saves one or more custom CFs. Skips duplicates (same Name + AppType).
func (s *customCFStore) Add(cfs []CustomCF) (added int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(s.dir, 0755); err != nil {
		return 0, fmt.Errorf("create custom-cfs dir: %w", err)
	}

	existing := s.listLocked("")
	existingKeys := make(map[string]bool)
	for _, cf := range existing {
		existingKeys[cf.Name+"\x00"+cf.AppType] = true
	}

	for _, cf := range cfs {
		if existingKeys[cf.Name+"\x00"+cf.AppType] {
			continue
		}
		if cf.ID == "" {
			cf.ID = generateCustomID()
		}
		if err := s.writeCF(cf); err != nil {
			return added, err
		}
		added++
	}
	return added, nil
}

// List returns all custom CFs, optionally filtered by app type.
func (s *customCFStore) List(appType string) []CustomCF {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.listLocked(appType)
}

func (s *customCFStore) listLocked(appType string) []CustomCF {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil
	}

	var result []CustomCF
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(s.dir, e.Name()))
		if err != nil {
			continue
		}
		var cf CustomCF
		if err := json.Unmarshal(data, &cf); err != nil {
			continue
		}
		if appType == "" || cf.AppType == appType {
			result = append(result, cf)
		}
	}
	return result
}

// Get returns a single custom CF by ID.
func (s *customCFStore) Get(id string) (CustomCF, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return CustomCF{}, false
	}

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(s.dir, e.Name()))
		if err != nil {
			continue
		}
		var cf CustomCF
		if err := json.Unmarshal(data, &cf); err != nil {
			continue
		}
		if cf.ID == id {
			return cf, true
		}
	}
	return CustomCF{}, false
}

// Delete removes a custom CF by ID.
func (s *customCFStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return fmt.Errorf("read custom-cfs dir: %w", err)
	}

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		path := filepath.Join(s.dir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var cf CustomCF
		if err := json.Unmarshal(data, &cf); err != nil {
			continue
		}
		if cf.ID == id {
			return os.Remove(path)
		}
	}
	return fmt.Errorf("custom CF %s not found", id)
}

// Update replaces an existing custom CF (matched by ID).
func (s *customCFStore) Update(cf CustomCF) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Find and remove old file
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return fmt.Errorf("read custom-cfs dir: %w", err)
	}

	found := false
	newFilename := sanitizeFilename(cf.Name) + ".json"
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		path := filepath.Join(s.dir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var existing CustomCF
		if err := json.Unmarshal(data, &existing); err != nil {
			continue
		}
		if existing.ID == cf.ID {
			found = true
			if e.Name() != newFilename {
				os.Remove(path)
			}
			break
		}
	}

	if !found {
		return fmt.Errorf("custom CF %s not found", cf.ID)
	}

	return s.writeCF(cf)
}

// writeCF writes a single custom CF to disk. Caller must hold mu.
func (s *customCFStore) writeCF(cf CustomCF) error {
	data, err := json.MarshalIndent(cf, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal custom CF: %w", err)
	}

	filename := sanitizeFilename(cf.Name) + ".json"
	path := filepath.Join(s.dir, filename)

	// Avoid overwriting existing file with different ID
	if existing, err := os.ReadFile(path); err == nil {
		var ecf CustomCF
		if json.Unmarshal(existing, &ecf) == nil && ecf.ID != cf.ID {
			idSuffix := strings.TrimPrefix(cf.ID, "custom:")
			if len(idSuffix) > 8 {
				idSuffix = idSuffix[:8]
			}
			filename = sanitizeFilename(cf.Name) + "_" + idSuffix + ".json"
			path = filepath.Join(s.dir, filename)
		}
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return fmt.Errorf("write custom CF: %w", err)
	}
	return os.Rename(tmp, path)
}
