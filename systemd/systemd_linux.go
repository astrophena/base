// © 2025 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the ISC
// license that can be found in the LICENSE.md file.

//go:build linux

package systemd

import (
	"context"
	"log/slog"
	"net"
	"strconv"
	"sync/atomic"
	"time"

	"go.astrophena.name/base/cli"
	"go.astrophena.name/base/logger"
)

// Notify sends a message to systemd using the sd_notify protocol.
// See https://www.freedesktop.org/software/systemd/man/sd_notify.html.
func Notify(ctx context.Context, state State) {
	addr := &net.UnixAddr{
		Net:  "unixgram",
		Name: cli.GetEnv(ctx).Getenv("NOTIFY_SOCKET"),
	}

	if addr.Name == "" {
		// We're not running under systemd (NOTIFY_SOCKET is not set).
		return
	}

	conn, err := net.DialUnix(addr.Net, nil, addr)
	if err != nil {
		logger.Error(ctx, "sdnotify failed", slog.String("state", string(state)), slog.Any("err", err))
	}
	defer conn.Close()

	if _, err = conn.Write([]byte(state)); err != nil {
		logger.Error(ctx, "sdnotify failed", slog.String("state", string(state)), slog.Any("err", err))
	}
}

var watchdogStarted atomic.Bool

// Watchdog starts a systemd watchdog timer in a separate goroutine that is stopped when the context is canceled.
// When the watchdog is not enabled for the service, it does nothing.
func Watchdog(ctx context.Context) {
	// Don't start the watchdog if it's already started.
	if watchdogStarted.Load() {
		return
	}
	watchdogStarted.Store(true)

	interval := watchdogInterval(ctx)
	if interval > 0 {
		go func() {
			// Use the halved interval so we definitely don't miss the watchdog timeout.
			ticker := time.NewTicker(interval / 2)
			defer ticker.Stop()

			for {
				select {
				case <-ticker.C:
					Notify(ctx, watchdog)
				case <-ctx.Done():
					return
				}
			}
		}()
	}
}

// watchdogInterval returns the watchdog interval configured in systemd unit file.
func watchdogInterval(ctx context.Context) time.Duration {
	s, err := strconv.Atoi(cli.GetEnv(ctx).Getenv("WATCHDOG_USEC"))
	if err != nil {
		return 0
	}
	if s <= 0 {
		return 0
	}
	return time.Duration(s) * time.Microsecond
}
