// © 2026 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the ISC
// license that can be found in the LICENSE.md file.

// Package safefile provides atomic file writing with backups.
package safefile

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

const (
	backupTimeFormat = "20060102150405.999999999"
	maxBackups       = 10
)

// WriteFile writes data to a file safely and atomically.
//
// If the target file exists, it is backed up with a timestamped ".bak" extension
// before the atomic write completes. Up to 10 of the most recent backups are
// maintained, and older backups are removed.
func WriteFile(name string, data []byte, perm fs.FileMode) (err error) {
	// Create a temporary file in the same directory to ensure that it's on the
	// same filesystem, which is a requirement for an atomic os.Rename.
	f, err := os.CreateTemp(filepath.Dir(name), "."+filepath.Base(name)+".tmp")
	if err != nil {
		return err
	}
	tmpName := f.Name()

	defer func() {
		// Clean up the temporary file if something goes wrong.
		if f != nil {
			f.Close()
		}
		os.Remove(tmpName)
	}()

	// Write data to the temporary file.
	if _, err := f.Write(data); err != nil {
		return err
	}
	if err := f.Chmod(perm); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	f = nil // prevent double-close in defer

	// If the original file exists, create a backup.
	var backupName string
	if _, err := os.Stat(name); err == nil {
		backupName = name + "." + time.Now().UTC().Format(backupTimeFormat) + ".bak"
		if err := os.Rename(name, backupName); err != nil {
			return err
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}

	// Atomically move the temporary file to the final destination.
	if err := os.Rename(tmpName, name); err != nil {
		// If the rename fails, try to restore the backup.
		if backupName != "" {
			if restoreErr := os.Rename(backupName, name); restoreErr != nil {
				return fmt.Errorf("atomic write to %q failed: %w, and restoring backup from %q failed: %v", name, err, backupName, restoreErr)
			}
		}
		return err
	}

	return pruneBackups(name)
}

func pruneBackups(name string) error {
	dir := filepath.Dir(name)
	base := filepath.Base(name)
	prefix := base + "."
	suffix := ".bak"

	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	var backups []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		entryName := entry.Name()
		if !strings.HasPrefix(entryName, prefix) || !strings.HasSuffix(entryName, suffix) {
			continue
		}

		tsPart := entryName[len(prefix) : len(entryName)-len(suffix)]
		if _, err := time.Parse(backupTimeFormat, tsPart); err == nil {
			backups = append(backups, filepath.Join(dir, entryName))
		}
	}

	if len(backups) <= maxBackups {
		return nil
	}

	slices.Sort(backups)

	for i := 0; i < len(backups)-maxBackups; i++ {
		if err := os.Remove(backups[i]); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}

	return nil
}
