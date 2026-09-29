// © 2024 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the ISC
// license that can be found in the LICENSE.md file.

package web

import (
	"bufio"
	"context"
	"crypto/rand"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"go.astrophena.name/base/ctxkey"
	"go.astrophena.name/base/logger"
	"go.astrophena.name/base/metrics"
	"go.astrophena.name/base/syncx"
	"go.astrophena.name/base/systemd"
	"go.astrophena.name/base/web/internal/hashfs"
	"go.astrophena.name/base/web/internal/unionfs"
)

var (
	serverContextKey      = ctxkey.New[*Server]("web.server", nil)
	connNetworkContextKey = ctxkey.New("web.connNetwork", "tcp")
)

// Server serves requests from Mux with logging, security headers, and static
// files. Mux must be set before using the server.
//
// Configure Server before calling [Server.StaticHashName], [Server.ServeHTTP],
// or [Server.ListenAndServe]. Do not change its fields afterward.
type Server struct {
	// Mux handles application routes. It must not be nil. Server adds /static/
	// and, when Debuggable is true, /debug/ routes to it.
	Mux *http.ServeMux
	// Debuggable adds /debug/ routes, including pprof and a force-GC handler.
	// Expose these routes only through a protected endpoint.
	Debuggable bool
	// Middleware wraps all requests, including static and debug routes.
	Middleware []Middleware
	// Addr selects a listener: "host:port" for TCP, an absolute path for a
	// Unix socket, or "sd-socket:<name>" for a systemd socket. In the last
	// form, name must match a name in LISTEN_FDNAMES.
	Addr string
	// Ready is called after the listener is opened.
	Ready func()
	// StaticFS adds files under /static/. The filesystem must contain a static
	// directory. Its files take precedence over embedded files with the same
	// names.
	StaticFS fs.FS
	// CrossOriginProtection handles CSRF checks. A new default instance is used
	// when this field is nil.
	CrossOriginProtection *http.CrossOriginProtection
	// CSP selects a Content Security Policy by request. Requests with no
	// matching policy use the default policy.
	CSP *CSPMux
	// NotifySystemd sends ready and stopping notifications and runs the
	// systemd watchdog when configured.
	NotifySystemd bool
	// TrustedProxies lists TCP proxy ranges allowed to set X-Forwarded-For.
	// Nil trusts 127.0.0.0/8; an empty non-nil slice trusts no TCP proxies.
	// Requests over Unix sockets always trust X-Forwarded-For.
	TrustedProxies []netip.Prefix
	// MetricsEndpoint labels this server's HTTP metrics. It defaults to "server".
	MetricsEndpoint string

	handler syncx.Lazy[*handler]

	// listenerNetwork stores the accepted listener network (e.g. "tcp", "unix").
	// It is set when ListenAndServe starts.
	listenerNetwork string
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
	status   int
	size     int
	onHijack func(net.Conn) net.Conn
}

// WriteHeader captures the status code before writing it to the underlying
// ResponseWriter.
func (r *statusRecorder) WriteHeader(status int) {
	if status == http.StatusSwitchingProtocols || status >= 200 {
		r.status = status
	}
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
		conn, rw, err := hijacker.Hijack()
		if err == nil && r.status == 0 {
			r.status = http.StatusSwitchingProtocols
		}
		if err == nil && r.onHijack != nil {
			conn = r.onHijack(conn)
		}
		return conn, rw, err
	}
	return nil, nil, errors.New("hijacking is not supported for this connection")
}

type trackedConn struct {
	net.Conn
	onClose func()
	once    sync.Once
	err     error
}

func (c *trackedConn) Close() error {
	c.once.Do(func() {
		c.err = c.Conn.Close()
		c.onClose()
	})
	return c.err
}

func (s *Server) logRequest(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		l := logger.Get(r.Context())
		sl := l.With(
			slog.String("ip", s.realIP(r)),
			slog.String("method", r.Method),
			slog.String("url", r.URL.String()),
			slog.String("user_agent", r.UserAgent()),
			slog.String("host", r.Host),
		)
		ctx := logger.Put(r.Context(), &logger.Logger{
			Logger: sl.With("request_id", rand.Text()),
			Level:  l.Level,
		})
		r = r.WithContext(ctx)
		_, route := s.Mux.Handler(r)
		if route == "" {
			route = "unmatched"
		}
		endpoint, method := s.metricsEndpoint(), metricMethod(r.Method)
		mw := metrics.Get(ctx)
		mw.Adjust(inFlight, 1, endpoint, method, route)

		recorder := &statusRecorder{ResponseWriter: w}
		completed := false
		complete := func() {
			if completed {
				return
			}
			completed = true
			if recorder.status == 0 {
				recorder.status = http.StatusOK
			}
			elapsed := time.Since(start)
			mw.Adjust(inFlight, -1, endpoint, method, route)
			s.recordRequest(ctx, endpoint, method, route, recorder.status, recorder.size, elapsed)
			logger.Info(ctx, "handled request",
				slog.Int("status", recorder.status),
				slog.Int("size", recorder.size),
				slog.Duration("duration", elapsed),
			)
		}
		recorder.onHijack = func(conn net.Conn) net.Conn {
			complete()
			mw.Add(connectionsHijacked, 1, endpoint)
			mw.Adjust(hijackedOpen, 1, endpoint)
			start := time.Now()
			return &trackedConn{Conn: conn, onClose: func() {
				mw.Adjust(hijackedOpen, -1, endpoint)
				mw.Observe(hijackedDuration, time.Since(start).Seconds(), endpoint, method, route)
			}}
		}
		defer complete()
		next.ServeHTTP(recorder, r)
	})
}

func (s *Server) realIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	addr, err := netip.ParseAddr(strings.TrimSpace(host))
	trusted := s.isTrustedForwardedSource(r, err == nil, addr)

	if trusted {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			for part := range strings.SplitSeq(xff, ",") {
				ip := strings.TrimSpace(part)
				if ip == "" {
					continue
				}
				if p, err := netip.ParseAddr(ip); err == nil {
					return p.String()
				}
			}
		}
	}

	if err == nil {
		return addr.String()
	}
	return strings.TrimSpace(host)
}

func (s *Server) isTrustedProxy(addr netip.Addr) bool {
	for _, prefix := range s.trustedProxies() {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}

var defaultTrustedProxies = []netip.Prefix{
	netip.MustParsePrefix("127.0.0.0/8"),
}

func (s *Server) trustedProxies() []netip.Prefix {
	if s.TrustedProxies == nil {
		return defaultTrustedProxies
	}
	return s.TrustedProxies
}

func (s *Server) isTrustedForwardedSource(r *http.Request, hasAddr bool, addr netip.Addr) bool {
	if network := connNetworkContextKey.Value(r.Context()); network == "unix" {
		return true
	}
	if s.listenerNetwork == "unix" {
		return true
	}
	return hasAddr && s.isTrustedProxy(addr)
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

func (s *Server) recoverPanic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				w.Header().Set("Connection", "close")
				RespondError(w, r, fmt.Errorf("%w: %s", ErrInternalServerError, err))
			}
		}()
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

	s.Mux.Handle("GET /static/", hashfs.FileServer(h.static))
	if s.Debuggable {
		Debugger(s.Mux).Handle("xff", "X-Forwarded-For", s.xffDebugHandler())
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
	mws = append(mws, s.recoverPanic) // always should be the last one
	for _, middleware := range slices.Backward(mws) {
		h.handler = middleware(h.handler)
	}

	return h
}

// StaticHashName returns the hashed path for a static file. Pass a path
// such as "static/css/main.css". If the file cannot be read, it returns name.
func (s *Server) StaticHashName(name string) string {
	return s.handler.Get(s.initHandler).static.HashName(name)
}

// StaticHashName returns the hashed static file path for the server in ctx.
//
// Use it with a request served by [Server.ListenAndServe]. It panics if ctx
// has no server. A request passed directly to [Server.ServeHTTP] normally has
// no server in its context.
func StaticHashName(ctx context.Context, name string) string {
	s, ok := serverContextKey.ValueOk(ctx)
	if !ok {
		panic("web: StaticHashName called on a context without a server")
	}
	return s.StaticHashName(name)
}

// ListenAndServe starts the HTTP server. Canceling ctx stops HTTP serving.
// A handler owns any connection it hijacks and must close it itself.
func (s *Server) ListenAndServe(ctx context.Context) error {
	var l net.Listener
	var err error

	if after, ok := strings.CutPrefix(s.Addr, "sd-socket:"); ok {
		name := after
		if name == "" {
			return errors.New("web: socket activation address is missing name (e.g., sd-socket:name)")
		}
		l, err = systemd.Socket(ctx, name)
		if err != nil {
			return fmt.Errorf("%w: failed to get systemd socket: %v", errListen, err)
		}
		logger.Info(ctx, "using systemd socket", slog.String("name", name), slog.String("addr", l.Addr().String()))
	} else {
		if s.Addr == "" {
			return errNoAddr
		}

		network := "tcp"
		if strings.HasPrefix(s.Addr, "/") {
			network = "unix"
		}

		l, err = net.Listen(network, s.Addr)
		if err != nil {
			return fmt.Errorf("%w: %v", errListen, err)
		}
		scheme, host := "http", l.Addr().String()
		if network == "unix" {
			if err := os.Chmod(s.Addr, 0o666); err != nil {
				return fmt.Errorf("%w: failed to set socket permissions: %v", errListen, err)
			}
			scheme = "unix"
		}
		logger.Info(ctx, "listening for HTTP requests", slog.String("addr", fmt.Sprintf("%s://%s", scheme, host)))
	}
	s.listenerNetwork = l.Addr().Network()

	baseLogger := logger.Get(ctx)
	mw := metrics.Get(ctx)
	endpoint := s.metricsEndpoint()
	httpSrv := &http.Server{
		ErrorLog: slog.NewLogLogger(baseLogger.Handler(), slog.LevelError),
		Handler:  s,
		BaseContext: func(_ net.Listener) context.Context {
			baseCtx := logger.Put(ctx, baseLogger)
			return serverContextKey.WithValue(baseCtx, s)
		},
		ConnContext: func(ctx context.Context, c net.Conn) context.Context {
			return connNetworkContextKey.WithValue(ctx, c.RemoteAddr().Network())
		},
		ConnState: func(_ net.Conn, state http.ConnState) {
			switch state {
			case http.StateNew:
				mw.Add(connectionsAccepted, 1, endpoint)
				mw.Adjust(connectionsOpen, 1, endpoint)
			case http.StateClosed:
				mw.Adjust(connectionsOpen, -1, endpoint)
			case http.StateHijacked:
				mw.Adjust(connectionsOpen, -1, endpoint)
			}
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

	if s.NotifySystemd {
		systemd.Notify(ctx, systemd.Ready)
		systemd.Notify(ctx, systemd.Status(fmt.Sprintf("Listening for HTTP requests on %s...", s.Addr)))
		systemd.Watchdog(ctx)
	}

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		logger.Info(ctx, "HTTP server gracefully shutting down")

		if s.NotifySystemd {
			systemd.Notify(ctx, systemd.Stopping)
		}

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

// StaticFS contains the embedded files served under /static/.
//
// Use [Server.StaticHashName] to make URLs that change when custom static
// files change.
var StaticFS = hashfs.NewFS(staticFS)
