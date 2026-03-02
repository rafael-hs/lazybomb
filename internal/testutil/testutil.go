// Package testutil provides shared helpers for tests across the project.
package testutil

import (
	"os"
	"path/filepath"
)

// WriteRaw writes raw bytes to path, creating parent directories as needed.
func WriteRaw(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
