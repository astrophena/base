// © 2025 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the ISC
// license that can be found in the LICENSE.md file.

// Package systemd provides a simple interface to systemd's sd-notify protocol.
package systemd

// State represents the sd-notify state.
// See https://www.freedesktop.org/software/systemd/man/latest/sd_notify.html#Well-known%20assignments for all possible values.
type State string

const (
	// Ready tells the service manager that service startup is
	// finished, or the service finished loading its configuration.
	// See https://www.freedesktop.org/software/systemd/man/sd_notify.html#READY=1.
	Ready State = "READY=1"

	// Stopping tells the service manager that service is stopping.
	// See https://www.freedesktop.org/software/systemd/man/sd_notify.html#STOPPING=1.
	Stopping State = "STOPPING=1"

	// Reloading tells the service manager that service is reloading.
	// See https://www.freedesktop.org/software/systemd/man/sd_notify.html#RELOADING=1.
	Reloading State = "RELOADING=1"

	// watchdog tells the service manager to update the watchdog timestamp.
	// See https://www.freedesktop.org/software/systemd/man/sd_notify.html#WATCHDOG=1.
	watchdog State = "WATCHDOG=1"
)

// Status returns a State that describes the service state.
// This is free-form and can be used for various purposes: general state feedback, fsck-like programs
// could pass completion percentages and failing programs could pass a human-readable error message.
// See https://www.freedesktop.org/software/systemd/man/latest/sd_notify.html#STATUS=%E2%80%A6.
func Status(status string) State {
	return State("STATUS=" + status)
}
