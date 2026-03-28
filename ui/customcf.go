package main

import (
	"crypto/rand"
	"encoding/hex"
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

// FileItem interface methods for CustomCF.
func (cf *CustomCF) GetID() string      { return cf.ID }
func (cf *CustomCF) GetName() string    { return cf.Name }
func (cf *CustomCF) SetName(n string)   { cf.Name = n }
func (cf *CustomCF) GetAppType() string { return cf.AppType }

// customCFStore manages custom CFs as individual JSON files in a directory.
// It embeds FileStore for generic CRUD and adds custom-CF-specific behavior (ID generation).
type customCFStore struct {
	*FileStore[CustomCF, *CustomCF]
}

func newCustomCFStore(dir string) *customCFStore {
	return &customCFStore{
		FileStore: NewFileStore[CustomCF, *CustomCF](dir),
	}
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

// Add saves one or more custom CFs. Generates IDs for items that don't have one.
// Skips duplicates (same Name + AppType). Returns the number added.
func (s *customCFStore) Add(cfs []CustomCF) (int, error) {
	for i := range cfs {
		if cfs[i].ID == "" {
			cfs[i].ID = generateCustomID()
		}
	}
	return s.FileStore.Add(cfs)
}

