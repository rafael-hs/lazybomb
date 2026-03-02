package profile

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// Profile stores a named request configuration.
type Profile struct {
	Name        string            `json:"name"`
	URL         string            `json:"url"`
	Method      string            `json:"method"`
	Headers     map[string]string `json:"headers,omitempty"`
	Body        string            `json:"body,omitempty"`
	Requests    int               `json:"requests"`
	Concurrency int               `json:"concurrency"`
	Duration    string            `json:"duration,omitempty"`
	RateLimit   float64           `json:"rate_limit,omitempty"`
	Timeout     int               `json:"timeout"`

	AuthKind     int    `json:"auth_kind,omitempty"`
	AuthToken    string `json:"auth_token,omitempty"`
	AuthUser     string `json:"auth_user,omitempty"`
	AuthPass     string `json:"auth_pass,omitempty"`
	AuthKeyName  string `json:"auth_key_name,omitempty"`
	AuthKeyValue string `json:"auth_key_value,omitempty"`
}

type profileStore struct {
	Profiles []Profile `json:"profiles"`
}

// Store manages profile persistence at a given file path.
type Store struct {
	path string
}

// NewStore returns a Store backed by the default user config directory.
func NewStore() (*Store, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	return &Store{path: filepath.Join(dir, "lazybomb", "profiles.json")}, nil
}

// Load reads profiles from disk. Returns an empty slice if the file does not exist.
func (s *Store) Load() ([]Profile, error) {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return []Profile{}, nil
	}
	if err != nil {
		return nil, err
	}

	var store profileStore
	if err := json.Unmarshal(data, &store); err != nil {
		return nil, err
	}
	return store.Profiles, nil
}

// Save persists a profile. If a profile with the same name exists it is replaced.
func (s *Store) Save(p Profile) error {
	profiles, err := s.Load()
	if err != nil {
		return err
	}

	replaced := false
	for i, existing := range profiles {
		if existing.Name == p.Name {
			profiles[i] = p
			replaced = true
			break
		}
	}
	if !replaced {
		profiles = append(profiles, p)
	}

	return s.write(profiles)
}

// Delete removes a profile by name. No-op if the name does not exist.
func (s *Store) Delete(name string) error {
	profiles, err := s.Load()
	if err != nil {
		return err
	}

	filtered := make([]Profile, 0, len(profiles))
	for _, p := range profiles {
		if p.Name != name {
			filtered = append(filtered, p)
		}
	}

	return s.write(filtered)
}

func (s *Store) write(profiles []Profile) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(profileStore{Profiles: profiles}, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(s.path, data, 0o644)
}
