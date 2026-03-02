package profile

import (
	"path/filepath"
	"testing"

	"github.com/rafael-hs/lazybomb/internal/testutil"
	"github.com/stretchr/testify/assert"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	return &Store{path: filepath.Join(t.TempDir(), "profiles.json")}
}

func TestLoad(t *testing.T) {
	t.Run("should return empty slice when file does not exist", func(t *testing.T) {
		s := newTestStore(t)

		profiles, err := s.Load()

		assert.NoError(t, err)
		assert.Empty(t, profiles)
	})

	t.Run("should return error when file contains invalid JSON", func(t *testing.T) {
		s := newTestStore(t)
		err := testutil.WriteRaw(s.path, []byte("not json at all"))
		assert.NoError(t, err)

		_, err = s.Load()

		assert.Error(t, err)
	})

	t.Run("should return all profiles when file is valid", func(t *testing.T) {
		s := newTestStore(t)
		_ = s.Save(Profile{Name: "a", URL: "https://a.example.com"})
		_ = s.Save(Profile{Name: "b", URL: "https://b.example.com"})

		profiles, err := s.Load()

		assert.NoError(t, err)
		assert.Len(t, profiles, 2)
	})
}

func TestSave(t *testing.T) {
	t.Run("should persist profile when profile is new", func(t *testing.T) {
		s := newTestStore(t)
		p := Profile{Name: "test", URL: "https://example.com", Method: "GET"}

		err := s.Save(p)
		assert.NoError(t, err)

		profiles, err := s.Load()
		assert.NoError(t, err)
		assert.Len(t, profiles, 1)
		assert.Equal(t, p.URL, profiles[0].URL)
	})

	t.Run("should replace existing profile when name matches", func(t *testing.T) {
		s := newTestStore(t)
		_ = s.Save(Profile{Name: "api", URL: "https://old.example.com", Method: "GET"})

		err := s.Save(Profile{Name: "api", URL: "https://new.example.com", Method: "POST"})
		assert.NoError(t, err)

		profiles, _ := s.Load()
		assert.Len(t, profiles, 1)
		assert.Equal(t, "https://new.example.com", profiles[0].URL)
		assert.Equal(t, "POST", profiles[0].Method)
	})

	t.Run("should append profile when names are different", func(t *testing.T) {
		s := newTestStore(t)
		_ = s.Save(Profile{Name: "a", URL: "https://a.example.com"})
		_ = s.Save(Profile{Name: "b", URL: "https://b.example.com"})
		_ = s.Save(Profile{Name: "c", URL: "https://c.example.com"})

		profiles, _ := s.Load()

		assert.Len(t, profiles, 3)
	})

	t.Run("should preserve all auth fields when saving", func(t *testing.T) {
		s := newTestStore(t)
		p := Profile{
			Name:        "secure",
			URL:         "https://api.example.com",
			Method:      "POST",
			AuthKind:    1, // Bearer
			AuthToken:   "my-secret-token",
			Concurrency: 5,
			Requests:    100,
			Timeout:     30,
		}

		_ = s.Save(p)
		profiles, _ := s.Load()

		assert.Equal(t, p.AuthKind, profiles[0].AuthKind)
		assert.Equal(t, p.AuthToken, profiles[0].AuthToken)
	})
}

func TestDelete(t *testing.T) {
	t.Run("should remove profile when name exists", func(t *testing.T) {
		s := newTestStore(t)
		_ = s.Save(Profile{Name: "keep", URL: "https://keep.example.com"})
		_ = s.Save(Profile{Name: "remove", URL: "https://remove.example.com"})

		err := s.Delete("remove")
		assert.NoError(t, err)

		profiles, _ := s.Load()
		assert.Len(t, profiles, 1)
		assert.Equal(t, "keep", profiles[0].Name)
	})

	t.Run("should not error when name does not exist", func(t *testing.T) {
		s := newTestStore(t)
		_ = s.Save(Profile{Name: "existing", URL: "https://example.com"})

		err := s.Delete("ghost")

		assert.NoError(t, err)
		profiles, _ := s.Load()
		assert.Len(t, profiles, 1)
	})
}
