// © 2025 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the ISC
// license that can be found in the LICENSE.md file.

//go:build !linux

package systemd

import "context"

// Notify is a no-op on non-Linux systems.
func Notify(ctx context.Context, state State) {}

// Watchdog is a no-op on non-Linux systems.
func Watchdog(ctx context.Context) {}
