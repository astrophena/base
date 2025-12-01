// © 2025 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the ISC
// license that can be found in the LICENSE.md file.

package systemd

import (
	"context"
	"net"
)

// Socket retrieves a named listener from systemd socket activation.
//
// This function is only implemented on Linux. On other platforms, it will
// always return an error.
func Socket(ctx context.Context, name string) (net.Listener, error) {
	return socket(ctx, name)
}
