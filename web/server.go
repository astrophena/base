// © 2024 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the ISC
// license that can be found in the LICENSE.md file.

package web

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"slices"
	"strings"
	"time"

	"go.astrophena.name/base/cli"
	"go.astrophena.name/base/logger"
	"go.astrophena.name/base/syncx"
	"go.astrophena.name/base/version"

	"github.com/benbjohnson/hashfs"
)

//go:generate curl --fail-with-body -s -o static/css/main.css https://astrophena.name/css/main.css

// Server is used to configure the HTTP server started by
// [Server.ListenAndServe].
//
// All fields of Server can't be modified after [Server.ListenAndServe]
// or [Server.ServeHTTP] is called for a first time.
type Server struct {
	// Mux is a http.ServeMux to serve.
	Mux *http.ServeMux
	// Debuggable specifies whether to register debug handlers at /debug/.
	Debuggable bool
	// Middleware specifies an optional slice of HTTP middleware that's applied to
	// each request.
	Middleware []Middleware
	// Addr is a network address to listen on (in the form of "host:port").
	Addr string
	// Ready specifies an optional function to be called when the server is ready
	// to serve requests.
	Ready func()

	handler syncx.Lazy[http.Handler]
}

// ServeHTTP implements the [http.Handler] interface.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.handler.Get(s.initHandler).ServeHTTP(w, r)
}

// The Content-Security-Policy header.
// Based on https://github.com/tailscale/tailscale/blob/4ad3f01225745294474f1ae0de33e5a86824a744/safeweb/http.go.
var cspHeader = strings.Join([]string{
	`default-src 'self'`,      // origin is the only valid source for all content types
	`script-src 'self'`,       // disallow inline javascript
	`frame-ancestors 'none'`,  // disallow framing of the page
	`form-action 'self'`,      // disallow form submissions to other origins
	`base-uri 'self'`,         // disallow base URIs from other origins
	`block-all-mixed-content`, // disallow mixed content when serving over HTTPS
	`object-src 'self'`,       // disallow embedding of resources from other origins
}, "; ")

var (
	errNoAddr = errors.New("server.Addr is empty")
	errListen = errors.New("failed to listen")
)

type Middleware func(http.Handler) http.Handler

var defaultMiddleware = []Middleware{
	setHeaders,
}

func setHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referer-Policy", "same-origin")
		w.Header().Set("Content-Security-Policy", cspHeader)
		next.ServeHTTP(w, r)
	})
}

func (s *Server) initHandler() http.Handler {
	if s.Mux == nil {
		panic("Server.Mux is nil")
	}

	// Initialize internal routes.
	s.Mux.Handle("GET /static/", hashfs.FileServer(StaticFS))
	s.Mux.HandleFunc("GET /version", func(w http.ResponseWriter, r *http.Request) { RespondJSON(w, version.Version()) })
	Health(s.Mux)
	if s.Debuggable {
		Debugger(s.Mux)
	}

	// Apply middleware.
	var handler http.Handler = s.Mux
	mws := append(defaultMiddleware, s.Middleware...)
	for _, middleware := range slices.Backward(mws) {
		handler = middleware(handler)
	}

	return handler
}

// ListenAndServe starts the HTTP server that can be stopped by canceling ctx.
func (s *Server) ListenAndServe(ctx context.Context) error {
	if s.Addr == "" {
		return errNoAddr
	}

	env := cli.GetEnv(ctx)

	l, err := net.Listen("tcp", s.Addr)
	if err != nil {
		return fmt.Errorf("%w: %v", errListen, err)
	}
	scheme, host := "http", l.Addr().String()

	env.Logf("Listening on %s://%s...", scheme, host)

	httpSrv := &http.Server{
		ErrorLog: log.New(logger.Logf(env.Logf), "", 0),
		Handler:  s,
		BaseContext: func(_ net.Listener) context.Context {
			return cli.WithEnv(context.Background(), cli.GetEnv(ctx))
		},
	}

	errCh := make(chan error, 1)

	go func() {
		if err := httpSrv.Serve(l); err != nil {
			if err != http.ErrServerClosed {
				errCh <- err
			}
		}
	}()

	if s.Ready != nil {
		s.Ready()
	}

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		env.Logf("Gracefully shutting down...")

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := httpSrv.Shutdown(shutdownCtx); err != nil {
			return err
		}
	}

	return nil
}

//go:embed static
var staticFS embed.FS

// StaticFS is an [embed.FS] that contains static resources served on /static/ path
// prefix of [Server] HTTP handlers.
var StaticFS = hashfs.NewFS(staticFS)
