// © 2026 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the ISC
// license that can be found in the LICENSE.md file.

package filelock

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "run.lock")
	if IsLocked(path) {
		t.Fatal("missing file is locked")
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("IsLocked created the file: %v", err)
	}

	f, err := Acquire(path)
	if err != nil {
		t.Fatal(err)
	}
	if !IsLocked(path) {
		t.Fatal("acquired file is unlocked")
	}
	if _, err := Acquire(path); !errors.Is(err, ErrLocked) {
		t.Fatalf("Acquire conflict = %v; want %v", err, ErrLocked)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if IsLocked(path) {
		t.Fatal("closed file is locked")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("lock file disappeared: %v", err)
	}
}
