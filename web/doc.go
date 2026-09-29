// © 2024 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the ISC
// license that can be found in the LICENSE.md file.

// Package web serves HTTP requests with logging, security headers, static
// files, and common error responses. Set Server.Mux and call
// Server.ListenAndServe. RegisterMetrics adds HTTP and Go runtime metrics.
package web
