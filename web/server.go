// © 2024 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the ISC
// license that can be found in the LICENSE.md file.

package web

import (
	"bufio"
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"slices"
	"strings"
	"time"

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
	// Addr is a network address to listen on. For TCP, use "host:port". For a
	// Unix socket, use an absolute file path (e.g., "/run/service/socket").
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
	// CSP is a multiplexer for Content Security Policies.
	// If nil, a default restrictive policy is used.
	CSP *CSPMux

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

var (
	errNoAddr = errors.New("server.Addr is empty")
	errListen = errors.New("failed to listen")
)

type Middleware func(http.Handler) http.Handler

// statusRecorder captures the HTTP status code and response size.
type statusRecorder struct {
	http.ResponseWriter
	status int
	size   int
}

// WriteHeader captures the status code before writing it to the underlying
// ResponseWriter.
func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

// Write captures the number of bytes written and updates the status code if
// WriteHeader has not been called.
func (r *statusRecorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	size, err := r.ResponseWriter.Write(b)
	r.size += size
	return size, err
}

// Flush implements the [http.Flusher] interface.
func (r *statusRecorder) Flush() {
	if flusher, ok := r.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

// Hijack implements the [http.Hijacker] interface.
func (r *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hijacker, ok := r.ResponseWriter.(http.Hijacker); ok {
		return hijacker.Hijack()
	}
	return nil, nil, errors.New("hijacking is not supported for this connection")
}

func (s *Server) logRequest(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		l := logger.Get(r.Context())
		sl := l.With(
			slog.String("ip", realIP(r)),
			slog.String("method", r.Method),
			slog.String("url", r.URL.String()),
			slog.String("user_agent", r.UserAgent()),
		)
		ctx := logger.Put(r.Context(), &logger.Logger{
			Logger: sl,
			Level:  l.Level,
		})
		r = r.WithContext(ctx)

		recorder := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(recorder, r)

		logger.Info(ctx, "handled request",
			slog.Int("status", recorder.status),
			slog.Int("size", recorder.size),
			slog.Duration("duration", time.Since(start)),
		)
	})
}

func realIP(r *http.Request) string {
	if ip := r.Header.Get("X-Forwarded-For"); ip != "" {
		return strings.Split(ip, ",")[0]
	}
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	return ip
}

func (s *Server) setHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referer-Policy", "same-origin")

		var policy CSP
		if s.CSP != nil {
			p, ok := s.CSP.PolicyFor(r)
			if ok {
				policy = p
			} else {
				// No pattern matched, use the default policy.
				policy = defaultCSP
			}
		} else {
			// No CSPMux configured, use the default policy.
			policy = defaultCSP
		}

		if cspHeader := policy.String(); cspHeader != "" {
			w.Header().Set("Content-Security-Policy", cspHeader)
		}

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
	mws := append([]Middleware{s.logRequest, s.setHeaders}, s.Middleware...)
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

	network := "tcp"
	if strings.HasPrefix(s.Addr, "/") {
		network = "unix"
	}

	l, err := net.Listen(network, s.Addr)
	if err != nil {
		return fmt.Errorf("%w: %v", errListen, err)
	}
	scheme, host := "http", l.Addr().String()
	if network == "unix" {
		scheme = "unix"
	}

	logger.Info(ctx, "listening for HTTP requests", slog.String("addr", fmt.Sprintf("%s://%s", scheme, host)))

	baseLogger := logger.Get(ctx)
	httpSrv := &http.Server{
		ErrorLog: slog.NewLogLogger(baseLogger.Handler(), slog.LevelError),
		Handler:  s,
		BaseContext: func(_ net.Listener) context.Context {
			return logger.Put(ctx, baseLogger)
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
		logger.Info(ctx, "HTTP server gracefully shutting down")

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
