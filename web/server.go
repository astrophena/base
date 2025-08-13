// © 2024 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the ISC
// license that can be found in the LICENSE.md file.

package web

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
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
	"go.astrophena.name/base/web/internal/hashfs"
	"go.astrophena.name/base/web/internal/unionfs"
)

// Server is used to configure the HTTP server started by
// [Server.ListenAndServe].
//
// All fields of Server can't be modified after [Server.StaticHashName], [Server.ListenAndServe]
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
	// StaticFS specifies an optional filesystem containing static assets (like CSS,
	// JS, images) to be served. If provided, it's combined with the embedded
	// StaticFS and served under the "/static/" path prefix.
	// Files in this FS take precedence over the embedded ones if names conflict.
	StaticFS fs.FS
	// CrossOriginProtection configures CSRF protection. Defaults are used if nil.
	CrossOriginProtection *http.CrossOriginProtection

	handler syncx.Lazy[*handler]
}

type handler struct {
	handler http.Handler
	csrf    *http.CrossOriginProtection
	static  *hashfs.FS
}

// ServeHTTP implements the [http.Handler] interface.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.handler.Get(s.initHandler).handler.ServeHTTP(w, r)
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

func (s *Server) initHandler() *handler {
	if s.Mux == nil {
		panic("Server.Mux is nil")
	}

	h := new(handler)

	var static unionfs.FS
	if s.StaticFS != nil {
		static = append(static, s.StaticFS)
	}
	static = append(static, staticFS)
	h.static = hashfs.NewFS(static)

	// Initialize internal routes.
	s.Mux.Handle("GET /static/", hashfs.FileServer(h.static))
	s.Mux.HandleFunc("GET /version", func(w http.ResponseWriter, r *http.Request) { RespondJSON(w, version.Version()) })
	Health(s.Mux)
	if s.Debuggable {
		Debugger(s.Mux)
	}

	if s.CrossOriginProtection != nil {
		h.csrf = s.CrossOriginProtection
	} else {
		h.csrf = http.NewCrossOriginProtection()
	}
	h.csrf.SetDenyHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		RespondError(w, r, fmt.Errorf("%w: CSRF protection failed", ErrForbidden))
	}))

	// Apply middleware.
	h.handler = h.csrf.Handler(s.Mux)
	mws := append(defaultMiddleware, s.Middleware...)
	for _, middleware := range slices.Backward(mws) {
		h.handler = middleware(h.handler)
	}

	return h
}

// StaticHashName returns the cache-busting hashed name for a static file path.
// If the path exists, its hashed name is returned. Otherwise, the original name is returned.
func (s *Server) StaticHashName(name string) string {
	return s.handler.Get(s.initHandler).static.HashName(name)
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

// StaticFS is a [hashfs.FS] containing the base static resources (like default CSS)
// served by the [Server] under the "/static/" path prefix.
//
// If you provide a custom Server.StaticFS, you must use the [Server.StaticHashName]
// method to generate correct hashed URLs for all static assets (both embedded and
// custom).
var StaticFS = hashfs.NewFS(staticFS)
