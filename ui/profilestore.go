package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// profileStore manages imported profiles as individual JSON files in a directory.
type profileStore struct {
	mu  sync.RWMutex
	dir string // e.g. /config/profiles
}

func newProfileStore(dir string) *profileStore {
	return &profileStore{dir: dir}
}

// Add saves one or more imported profiles as individual JSON files.
// Skips profiles that already exist (same Name + AppType) for idempotent migration.
func (ps *profileStore) Add(profiles []ImportedProfile) error {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	if err := os.MkdirAll(ps.dir, 0755); err != nil {
		return fmt.Errorf("create profiles dir: %w", err)
	}

	// Load existing profiles for dedup check
	existing := ps.listLocked("")
	existingKeys := make(map[string]bool)
	for _, ep := range existing {
		existingKeys[ep.Name+"\x00"+ep.AppType] = true
	}

	for _, p := range profiles {
		if existingKeys[p.Name+"\x00"+p.AppType] {
			continue // skip duplicate
		}
		if err := ps.writeProfile(p); err != nil {
			return err
		}
	}
	return nil
}

// List returns all imported profiles, optionally filtered by app type.
func (ps *profileStore) List(appType string) []ImportedProfile {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	return ps.listLocked(appType)
}

// listLocked reads profiles from disk. Caller must hold mu.
func (ps *profileStore) listLocked(appType string) []ImportedProfile {
	entries, err := os.ReadDir(ps.dir)
	if err != nil {
		return nil
	}

	var result []ImportedProfile
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(ps.dir, e.Name()))
		if err != nil {
			continue
		}
		var p ImportedProfile
		if err := json.Unmarshal(data, &p); err != nil {
			continue
		}
		if appType == "" || p.AppType == appType {
			result = append(result, p)
		}
	}
	return result
}

// Get returns a single profile by ID.
func (ps *profileStore) Get(id string) (ImportedProfile, bool) {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	entries, err := os.ReadDir(ps.dir)
	if err != nil {
		return ImportedProfile{}, false
	}

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(ps.dir, e.Name()))
		if err != nil {
			continue
		}
		var p ImportedProfile
		if err := json.Unmarshal(data, &p); err != nil {
			continue
		}
		if p.ID == id {
			return p, true
		}
	}
	return ImportedProfile{}, false
}

// Delete removes an imported profile by ID.
func (ps *profileStore) Delete(id string) error {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	entries, err := os.ReadDir(ps.dir)
	if err != nil {
		return fmt.Errorf("read profiles dir: %w", err)
	}

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		path := filepath.Join(ps.dir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var p ImportedProfile
		if err := json.Unmarshal(data, &p); err != nil {
			continue
		}
		if p.ID == id {
			return os.Remove(path)
		}
	}
	return fmt.Errorf("imported profile %s not found", id)
}

// Update replaces an existing profile (matched by ID) with new data.
func (ps *profileStore) Update(p ImportedProfile) error {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	// Write new file first, then remove old file (safe: writeProfile handles name collisions)
	if err := ps.writeProfile(p); err != nil {
		return err
	}

	// Remove old file if it has a different filename than the new one
	newFilename := sanitizeFilename(p.Name) + ".json"
	entries, err := os.ReadDir(ps.dir)
	if err != nil {
		return nil // write succeeded, cleanup failure is non-fatal
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") || e.Name() == newFilename {
			continue
		}
		path := filepath.Join(ps.dir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var ep ImportedProfile
		if err := json.Unmarshal(data, &ep); err != nil {
			continue
		}
		if ep.ID == p.ID {
			os.Remove(path)
			break
		}
	}

	return nil
}

// writeProfile writes a single profile to disk. Must be called with mu held.
func (ps *profileStore) writeProfile(p ImportedProfile) error {
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal profile: %w", err)
	}

	filename := sanitizeFilename(p.Name) + ".json"
	path := filepath.Join(ps.dir, filename)

	// Avoid overwriting existing file with different ID
	if existing, err := os.ReadFile(path); err == nil {
		var ep ImportedProfile
		if json.Unmarshal(existing, &ep) == nil && ep.ID != p.ID {
			// Name collision — add numeric suffix to both profile name and filename
			for suffix := 2; suffix < 100; suffix++ {
				candidate := fmt.Sprintf("%s (%d)", p.Name, suffix)
				candidateFile := sanitizeFilename(candidate) + ".json"
				candidatePath := filepath.Join(ps.dir, candidateFile)
				if _, err := os.Stat(candidatePath); err != nil {
					// File doesn't exist — use this name
					p.Name = candidate
					filename = candidateFile
					path = candidatePath
					break
				}
			}
		}
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return fmt.Errorf("write profile: %w", err)
	}
	return os.Rename(tmp, path)
}

// sanitizeFilename creates a safe filename from a profile name.
func sanitizeFilename(name string) string {
	name = strings.ToLower(name)
	name = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		if r == ' ' || r == '/' || r == '\\' {
			return '-'
		}
		return -1
	}, name)
	// Collapse multiple dashes
	for strings.Contains(name, "--") {
		name = strings.ReplaceAll(name, "--", "-")
	}
	name = strings.Trim(name, "-")
	if name == "" {
		name = "profile"
	}
	return name
}
