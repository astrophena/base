// © 2026 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the ISC
// license that can be found in the LICENSE.md file.

package service

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"go.astrophena.name/base/cli"
	"go.astrophena.name/base/web"

	"golang.org/x/sync/errgroup"
)

const (
	// DefaultPublicAddr is the default address for public requests.
	DefaultPublicAddr = "localhost:3000"
	// DefaultAdminAddr is the default address for admin requests.
	DefaultAdminAddr = "localhost:3001"
)

// EndpointConfig defines the configuration for a single HTTP endpoint.
type EndpointConfig struct {
	CSP                   *web.CSPMux
	CrossOriginProtection *http.CrossOriginProtection
	Debuggable            bool
	Middleware            []web.Middleware
	Mux                   *http.ServeMux
	StaticFS              fs.FS
}

// PublicService is implemented by services that expose a public endpoint.
type PublicService interface {
	PublicEndpoint(ctx context.Context) (*EndpointConfig, error)
}

// AdminService is implemented by services that expose an admin/internal endpoint.
type AdminService interface {
	AdminEndpoint(ctx context.Context) (*EndpointConfig, error)
}

// StatefulService is implemented by services that require a persistent state directory.
// Implementing this service registers the -state-dir flag.
type StatefulService interface {
	SetStateDir(dir string)
}

// IsActiveChecker allows a service to report if it is currently processing work,
// preventing idle shutdown.
type IsActiveChecker interface {
	IsActive() bool
}

// ShutdownHook allows a service to perform cleanup before shutdown.
type ShutdownHook interface {
	Shutdown(ctx context.Context) error
}

// Run runs the service. It orchestrates endpoints and background workers based
// on the methods implemented by svc.
//
// It panics if svc does not implement at least one of [PublicService],
// [AdminService], or [cli.App].
func Run(svc any) {
	_, isPublic := svc.(PublicService)
	_, isAdmin := svc.(AdminService)
	_, isApp := svc.(cli.App)

	if !isPublic && !isAdmin && !isApp {
		panic("service must implement at least one of: PublicEndpoint, AdminEndpoint, or Run")
	}

	cli.Main(&adapter{svc: svc})
}

type adapter struct {
	svc any

	// flags
	addr         string
	adminAddr    string
	exitIdleTime time.Duration
	stateDir     string

	// internal state
	monitorInterval time.Duration // if zero, defaults to 30s
	lastActivity    atomic.Int64
}

type hasFlags interface {
	Flags(*flag.FlagSet)
}

func (a *adapter) Flags(fs *flag.FlagSet) {
	if s, ok := a.svc.(hasFlags); ok {
		s.Flags(fs)
	}
	// Defaults for local development.
	if _, ok := a.svc.(PublicService); ok {
		fs.StringVar(&a.addr, "addr", DefaultPublicAddr, "Listen on `host:port or Unix socket` for public requests.")
	}
	if _, ok := a.svc.(AdminService); ok {
		fs.StringVar(&a.adminAddr, "admin-addr", DefaultAdminAddr, "Listen on `host:port or Unix socket` for admin requests.")
	}
	if _, ok := a.svc.(StatefulService); ok {
		fs.StringVar(&a.stateDir, "state-dir", "", "Path to the `directory` where to keep state.")
	}
	fs.DurationVar(&a.exitIdleTime, "exit-idle-time", 0, "If non-zero, the service will exit after this duration of inactivity.")
}

func (a *adapter) Run(ctx context.Context) error {
	if s, ok := a.svc.(StatefulService); ok {
		s.SetStateDir(a.stateDir)
	}

	a.lastActivity.Store(time.Now().Unix())

	if a.exitIdleTime > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithCancel(ctx)
		go a.runActivityMonitor(ctx, cancel)
	}

	g, gctx := errgroup.WithContext(ctx)
	var readyWG sync.WaitGroup

	if s, ok := a.svc.(PublicService); ok {
		config, err := s.PublicEndpoint(gctx)
		if err != nil {
			return fmt.Errorf("configuring public endpoint: %w", err)
		}
		if config != nil {
			readyWG.Add(1)
			srv := a.newServer(a.addr, config, true, readyWG.Done)
			g.Go(func() error { return srv.ListenAndServe(gctx) })
		}
	}

	if s, ok := a.svc.(AdminService); ok {
		config, err := s.AdminEndpoint(gctx)
		if err != nil {
			return fmt.Errorf("configuring admin endpoint: %w", err)
		}
		if config != nil {
			readyWG.Add(1)
			// Only notify systemd if we didn't already do it for the public endpoint.
			_, hasPublic := a.svc.(PublicService)
			srv := a.newServer(a.adminAddr, config, !hasPublic, readyWG.Done)
			// Admin endpoints trust all requests by default.
			srv.Middleware = append([]web.Middleware{trustAllRequests}, srv.Middleware...)
			g.Go(func() error { return srv.ListenAndServe(gctx) })
		}
	}

	if s, ok := a.svc.(cli.App); ok {
		g.Go(func() error { return s.Run(gctx) })
	}

	waitErr := g.Wait()

	if shutdown, ok := a.svc.(ShutdownHook); ok {
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()
		if err := shutdown.Shutdown(shutdownCtx); err != nil {
			return errors.Join(waitErr, err)
		}
	}

	return waitErr
}

func (a *adapter) newServer(addr string, config *EndpointConfig, notify bool, readyFunc func()) *web.Server {
	middleware := append([]web.Middleware{a.activityTracker}, config.Middleware...)

	return &web.Server{
		Addr:                  addr,
		Debuggable:            config.Debuggable,
		Mux:                   config.Mux,
		Middleware:            middleware,
		StaticFS:              config.StaticFS,
		CrossOriginProtection: config.CrossOriginProtection,
		CSP:                   config.CSP,
		NotifySystemd:         notify,
		Ready:                 readyFunc,
	}
}

func (a *adapter) activityTracker(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a.lastActivity.Store(time.Now().Unix())
		next.ServeHTTP(w, r)
	})
}

func (a *adapter) runActivityMonitor(ctx context.Context, cancel context.CancelFunc) {
	interval := a.monitorInterval
	if interval == 0 {
		interval = 30 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if c, ok := a.svc.(IsActiveChecker); ok && c.IsActive() {
				continue
			}

			if time.Since(time.Unix(a.lastActivity.Load(), 0)) > a.exitIdleTime {
				cancel()
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

func trustAllRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r = web.TrustRequest(r)
		next.ServeHTTP(w, r)
	})
}
