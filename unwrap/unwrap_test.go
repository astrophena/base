// © 2025 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the ISC
// license that can be found in the LICENSE.md file.

package unwrap

import (
	"errors"
	"testing"

	"go.astrophena.name/base/testutil"
)

func TestValue(t *testing.T) {
	t.Run("with nil error", func(t *testing.T) {
		result := Value("success", nil)
		testutil.AssertEqual(t, result, "success")
	})

	t.Run("with non-nil error", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Errorf("The code did not panic")
			}
		}()
		Value("failure", errors.New("something went wrong"))
	})
}

func TestNoError(t *testing.T) {
	t.Run("with nil error", func(t *testing.T) {
		// Should not panic.
		NoError(nil)
	})

	t.Run("with non-nil error", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Errorf("The code did not panic")
			}
		}()
		NoError(errors.New("something went wrong"))
	})
}
