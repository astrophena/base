// © 2025 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the ISC
// license that can be found in the LICENSE.md file.

// Package unwrap provides helper functions to panic on non-nil errors.
//
// This package is intended for use in initialization or test code where an
// error is considered a fatal, unrecoverable condition. It should be used
// sparingly in application logic where idiomatic error handling is preferred.
package unwrap

// Value unwraps and returns val if err is nil.
// It panics if err is not nil.
func Value[T any](val T, err error) T {
	NoError(err)
	return val
}

// NoError panics if err is not nil.
// It is used for checking operations that don't return a value.
func NoError(err error) {
	if err != nil {
		panic(err)
	}
}
