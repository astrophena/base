// © 2025 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the ISC
// license that can be found in the LICENSE.md file.

//go:build !linux

package systemd

import (
	"context"
	"errors"
	"net"
)

var errNotSupported = errors.New("systemd: socket activation is not supported on this platform")

func socket(ctx context.Context, name string) (net.Listener, error) {
	return nil, errNotSupported
}
