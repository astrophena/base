// © 2025 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the ISC
// license that can be found in the LICENSE.md file.

// Package unwrap provides helper functions to panic on non-nil errors.
package unwrap

// Value unwraps and returns val if err is nil.
// It panics if err is not nil.
func Value[T any](val T, err error) T {
	NoError(err)
	return val
}

// NoError panics if err is not nil.
func NoError(err error) {
	if err != nil {
		panic(err)
	}
}
