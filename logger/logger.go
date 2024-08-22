// © 2024 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the ISC
// license that can be found in the LICENSE.md file.

// Package logger provides a basic logger type.
package logger

// Logf is a simple printf-like logging function.
type Logf func(format string, args ...any)
