// © 2024 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the ISC
// license that can be found in the LICENSE.md file.

// Package logger provides a basic logger type.
package logger

import "io"

// Logf is a simple printf-like logging function.
type Logf func(format string, args ...any)

// Write implements the [io.Writer] interface.
func (f Logf) Write(p []byte) (n int, err error) {
	f("%s", p)
	return len(p), nil
}

var _ io.Writer = (Logf)(nil)
