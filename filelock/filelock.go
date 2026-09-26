// © 2026 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the ISC
// license that can be found in the LICENSE.md file.

// Package filelock provides non-blocking advisory file locks.
package filelock

import (
	"errors"
	"os"
	"syscall"
)

// ErrLocked means the lock is already held.
var ErrLocked = errors.New("file is locked")

// Acquire locks path exclusively without waiting. Close the returned file to
// release the lock. The file remains on disk after it is closed.
func Acquire(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, ErrLocked
		}
		return nil, err
	}
	return f, nil
}

// IsLocked reports whether another file descriptor holds a lock on path.
// A missing or unreadable file is treated as unlocked.
func IsLocked(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	return errors.Is(err, syscall.EWOULDBLOCK)
}
