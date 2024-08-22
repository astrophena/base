// © 2024 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the ISC
// license that can be found in the LICENSE.md file.

package logger

import (
	"fmt"
	"testing"

	"go.astrophena.name/base/testutil"
)

func TestLogfWriter(t *testing.T) {
	var (
		logged  bool
		message string
	)
	logf := func(format string, args ...any) {
		logged = true
		message = fmt.Sprintf(format, args...)
	}
	Logf(logf).Write([]byte("hello"))
	testutil.AssertEqual(t, logged, true)
	testutil.AssertEqual(t, message, "hello")
}
