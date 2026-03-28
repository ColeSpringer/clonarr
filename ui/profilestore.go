package main

// profileStore manages imported profiles as individual JSON files in a directory.
// It embeds FileStore for generic CRUD and adds profile-specific behavior.
type profileStore struct {
	*FileStore[ImportedProfile, *ImportedProfile]
}

func newProfileStore(dir string) *profileStore {
	return &profileStore{
		FileStore: NewFileStore[ImportedProfile, *ImportedProfile](dir),
	}
}
