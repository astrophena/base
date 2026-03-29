// © 2026 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the ISC
// license that can be found in the LICENSE.md file.

package safefile

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.astrophena.name/base/testutil"
)

func TestWriteFile(t *testing.T) {
	t.Parallel()

	t.Run("new file", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		file := filepath.Join(dir, "test.txt")
		data := []byte("hello")

		if err := WriteFile(file, data, 0o644); err != nil {
			t.Fatal(err)
		}

		got, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		testutil.AssertEqual(t, string(got), string(data))

		backups, err := filepath.Glob(file + ".*.bak")
		if err != nil {
			t.Fatal(err)
		}
		testutil.AssertEqual(t, len(backups), 0)
	})

	t.Run("overwrite", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		file := filepath.Join(dir, "test.txt")
		data1 := []byte("hello")
		data2 := []byte("world")

		if err := WriteFile(file, data1, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := WriteFile(file, data2, 0o644); err != nil {
			t.Fatal(err)
		}

		got, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		testutil.AssertEqual(t, string(got), string(data2))

		backups, err := filepath.Glob(file + ".*.bak")
		if err != nil {
			t.Fatal(err)
		}
		testutil.AssertEqual(t, len(backups), 1)

		backupData, err := os.ReadFile(backups[0])
		if err != nil {
			t.Fatal(err)
		}
		testutil.AssertEqual(t, string(backupData), string(data1))
	})

	t.Run("prune", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		file := filepath.Join(dir, "test.txt")

		// Create more than maxBackups files.
		for i := range maxBackups + 2 {
			data := []byte{byte(i)}
			if err := WriteFile(file, data, 0o644); err != nil {
				t.Fatal(err)
			}
			// Sleep to ensure unique backup timestamps.
			time.Sleep(2 * time.Millisecond)
		}

		backups, err := filepath.Glob(file + ".*.bak")
		if err != nil {
			t.Fatal(err)
		}
		testutil.AssertEqual(t, len(backups), maxBackups)
	})

	t.Run("glob characters", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		// Filename with glob meta-characters usually causing issues with filepath.Glob.
		file := filepath.Join(dir, "test-[1]*?.txt")

		for i := range maxBackups + 2 {
			data := []byte{byte(i)}
			if err := WriteFile(file, data, 0o644); err != nil {
				t.Fatal(err)
			}
			// Sleep to ensure unique backup timestamps.
			time.Sleep(2 * time.Millisecond)
		}

		// Count backups manually since we cannot use filepath.Glob reliably here.
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		var backups int
		for _, e := range entries {
			if e.Name() != "test-[1]*?.txt" && !e.IsDir() {
				backups++
			}
		}
		testutil.AssertEqual(t, backups, maxBackups)
	})

	t.Run("ignore unintended files", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		file := filepath.Join(dir, "test.txt")

		// Create a file that matches the old glob pattern but doesn't have a valid backup timestamp.
		unintended := file + ".user-backup.bak"
		if err := os.WriteFile(unintended, []byte("important"), 0o644); err != nil {
			t.Fatal(err)
		}

		for i := range maxBackups + 2 {
			data := []byte{byte(i)}
			if err := WriteFile(file, data, 0o644); err != nil {
				t.Fatal(err)
			}
			// Sleep to ensure unique backup timestamps.
			time.Sleep(2 * time.Millisecond)
		}

		// The unintended file should still exist!
		if _, err := os.Stat(unintended); err != nil {
			t.Fatalf("unintended file was deleted: %v", err)
		}
	})
}
