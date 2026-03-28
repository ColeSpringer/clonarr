package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// FileItem is the constraint for types stored in a FileStore.
// Methods are defined on pointer receivers of the concrete type.
type FileItem interface {
	GetID() string
	GetName() string
	SetName(string)
	GetAppType() string
}

// FileStore[T] provides generic file-backed CRUD for types whose pointer
// implements FileItem. T must be a struct type; the store works with *T
// internally to satisfy the interface.
type FileStore[T any, PT interface {
	*T
	FileItem
}] struct {
	mu  sync.RWMutex
	dir string
}

// NewFileStore creates a FileStore backed by the given directory.
func NewFileStore[T any, PT interface {
	*T
	FileItem
}](dir string) *FileStore[T, PT] {
	return &FileStore[T, PT]{dir: dir}
}

// Add saves one or more items. Skips duplicates (same Name + AppType).
// Returns the number of items actually added.
func (fs *FileStore[T, PT]) Add(items []T) (int, error) {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	if err := os.MkdirAll(fs.dir, 0755); err != nil {
		return 0, fmt.Errorf("create dir %s: %w", fs.dir, err)
	}

	existing := fs.listLocked("")
	existingKeys := make(map[string]bool, len(existing))
	for i := range existing {
		p := PT(&existing[i])
		existingKeys[p.GetName()+"\x00"+p.GetAppType()] = true
	}

	added := 0
	for i := range items {
		p := PT(&items[i])
		if existingKeys[p.GetName()+"\x00"+p.GetAppType()] {
			continue
		}
		if err := fs.writeItem(&items[i]); err != nil {
			return added, err
		}
		added++
	}
	return added, nil
}

// List returns all items, optionally filtered by app type.
func (fs *FileStore[T, PT]) List(appType string) []T {
	fs.mu.RLock()
	defer fs.mu.RUnlock()
	return fs.listLocked(appType)
}

// listLocked reads items from disk. Caller must hold mu.
func (fs *FileStore[T, PT]) listLocked(appType string) []T {
	entries, err := os.ReadDir(fs.dir)
	if err != nil {
		return nil
	}

	var result []T
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(fs.dir, e.Name()))
		if err != nil {
			continue
		}
		var item T
		if err := json.Unmarshal(data, &item); err != nil {
			continue
		}
		p := PT(&item)
		if appType == "" || p.GetAppType() == appType {
			result = append(result, item)
		}
	}
	return result
}

// Get returns a single item by ID.
func (fs *FileStore[T, PT]) Get(id string) (T, bool) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()

	entries, err := os.ReadDir(fs.dir)
	if err != nil {
		var zero T
		return zero, false
	}

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(fs.dir, e.Name()))
		if err != nil {
			continue
		}
		var item T
		if err := json.Unmarshal(data, &item); err != nil {
			continue
		}
		if PT(&item).GetID() == id {
			return item, true
		}
	}
	var zero T
	return zero, false
}

// Delete removes an item by ID.
func (fs *FileStore[T, PT]) Delete(id string) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	entries, err := os.ReadDir(fs.dir)
	if err != nil {
		return fmt.Errorf("read dir %s: %w", fs.dir, err)
	}

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		path := filepath.Join(fs.dir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var item T
		if err := json.Unmarshal(data, &item); err != nil {
			continue
		}
		if PT(&item).GetID() == id {
			return os.Remove(path)
		}
	}
	return fmt.Errorf("item %s not found", id)
}

// Update replaces an existing item (matched by ID).
func (fs *FileStore[T, PT]) Update(item T) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	p := PT(&item)
	newFilename := sanitizeFilename(p.GetName()) + ".json"

	entries, err := os.ReadDir(fs.dir)
	if err != nil {
		return fmt.Errorf("read dir %s: %w", fs.dir, err)
	}

	found := false
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		path := filepath.Join(fs.dir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var existing T
		if err := json.Unmarshal(data, &existing); err != nil {
			continue
		}
		if PT(&existing).GetID() == p.GetID() {
			found = true
			if e.Name() != newFilename {
				os.Remove(path)
			}
			break
		}
	}

	if !found {
		return fmt.Errorf("item %s not found", p.GetID())
	}

	return fs.writeItem(&item)
}

// writeItem writes a single item to disk. Caller must hold mu.
func (fs *FileStore[T, PT]) writeItem(item *T) error {
	p := PT(item)

	data, err := json.MarshalIndent(item, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal item: %w", err)
	}

	filename := sanitizeFilename(p.GetName()) + ".json"
	path := filepath.Join(fs.dir, filename)

	// Avoid overwriting existing file with different ID (collision handling with numeric suffix)
	if existingData, err := os.ReadFile(path); err == nil {
		var existingItem T
		if json.Unmarshal(existingData, &existingItem) == nil && PT(&existingItem).GetID() != p.GetID() {
			for suffix := 2; suffix < 100; suffix++ {
				candidate := fmt.Sprintf("%s (%d)", p.GetName(), suffix)
				candidateFile := sanitizeFilename(candidate) + ".json"
				candidatePath := filepath.Join(fs.dir, candidateFile)
				if _, err := os.Stat(candidatePath); err != nil {
					p.SetName(candidate)
					// Re-marshal with updated name
					data, err = json.MarshalIndent(item, "", "  ")
					if err != nil {
						return fmt.Errorf("marshal item: %w", err)
					}
					filename = candidateFile
					path = candidatePath
					break
				}
			}
		}
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return fmt.Errorf("write item: %w", err)
	}
	return os.Rename(tmp, path)
}

// sanitizeFilename creates a safe filename from a name string.
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
		name = "item"
	}
	return name
}
