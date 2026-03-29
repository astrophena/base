// © 2026 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the ISC
// license that can be found in the LICENSE.md file.

package service

import (
	"context"
	"errors"
	"flag"
	"net/http"
	"sync"
	"testing"
	"time"

	"go.astrophena.name/base/testutil"
)

type emptyService struct{}

func TestRun_Panic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("The code did not panic")
		}
	}()
	Run(new(emptyService))
}

func TestAdapter_IdleExit(t *testing.T) {
	a := &adapter{
		svc:             &runnerOnlyService{hasRunner: true},
		exitIdleTime:    100 * time.Millisecond,
		monitorInterval: 50 * time.Millisecond,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err := a.Run(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("a.Run() error = %v, want %v", err, context.Canceled)
	}
}

type testService struct {
	hasPublic, hasAdmin, hasRunner, hasFlags, hasState bool
	publicErr, adminErr, runnerErr                     error
	startupWG                                          *sync.WaitGroup
	isActive                                           bool
	gotStateDir                                        string
}

type publicOnlyService testService
type adminOnlyService testService
type runnerOnlyService testService
type fullService testService
type statefulService testService

func (s *testService) IsActive() bool {
	return s.isActive
}

func TestAdapter_IsActive(t *testing.T) {
	a := &adapter{
		svc:             &runnerOnlyService{hasRunner: true, isActive: true},
		exitIdleTime:    100 * time.Millisecond,
		monitorInterval: 50 * time.Millisecond,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	err := a.Run(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("a.Run() error = %v, want %v", err, context.DeadlineExceeded)
	}
}

func (s *publicOnlyService) PublicEndpoint(ctx context.Context) (*EndpointConfig, error) {
	return (*testService)(s).PublicEndpoint(ctx)
}
func (s *adminOnlyService) AdminEndpoint(ctx context.Context) (*EndpointConfig, error) {
	return (*testService)(s).AdminEndpoint(ctx)
}
func (s *runnerOnlyService) Run(ctx context.Context) error {
	return (*testService)(s).Run(ctx)
}
func (s *fullService) PublicEndpoint(ctx context.Context) (*EndpointConfig, error) {
	return (*testService)(s).PublicEndpoint(ctx)
}
func (s *fullService) AdminEndpoint(ctx context.Context) (*EndpointConfig, error) {
	return (*testService)(s).AdminEndpoint(ctx)
}
func (s *fullService) Run(ctx context.Context) error {
	return (*testService)(s).Run(ctx)
}

func (s *publicOnlyService) Flags(fs *flag.FlagSet)      { (*testService)(s).Flags(fs) }
func (s *adminOnlyService) Flags(fs *flag.FlagSet)       { (*testService)(s).Flags(fs) }
func (s *runnerOnlyService) Flags(fs *flag.FlagSet)      { (*testService)(s).Flags(fs) }
func (s *fullService) Flags(fs *flag.FlagSet)            { (*testService)(s).Flags(fs) }
func (s *statefulService) Flags(fs *flag.FlagSet)        { (*testService)(s).Flags(fs) }
func (s *publicOnlyService) IsActive() bool              { return (*testService)(s).IsActive() }
func (s *adminOnlyService) IsActive() bool               { return (*testService)(s).IsActive() }
func (s *runnerOnlyService) IsActive() bool              { return (*testService)(s).IsActive() }
func (s *fullService) IsActive() bool                    { return (*testService)(s).IsActive() }
func (s *statefulService) IsActive() bool                { return (*testService)(s).IsActive() }
func (s *statefulService) SetStateDir(dir string)        { (*testService)(s).gotStateDir = dir }
func (s *statefulService) Run(ctx context.Context) error { return (*testService)(s).Run(ctx) }

func (s *testService) PublicEndpoint(ctx context.Context) (*EndpointConfig, error) {
	if !s.hasPublic {
		return nil, nil
	}
	if s.publicErr != nil {
		return nil, s.publicErr
	}
	if s.startupWG != nil {
		s.startupWG.Done()
	}
	return &EndpointConfig{Mux: http.NewServeMux()}, nil
}

func (s *testService) AdminEndpoint(ctx context.Context) (*EndpointConfig, error) {
	if !s.hasAdmin {
		return nil, nil
	}
	if s.adminErr != nil {
		return nil, s.adminErr
	}
	if s.startupWG != nil {
		s.startupWG.Done()
	}
	return &EndpointConfig{Mux: http.NewServeMux()}, nil
}

func (s *testService) Run(ctx context.Context) error {
	if !s.hasRunner {
		// Should not be called.
		return errors.New("unexpected call to Run")
	}
	if s.runnerErr != nil {
		return s.runnerErr
	}
	<-ctx.Done()
	return ctx.Err()
}

func (s *testService) Flags(fs *flag.FlagSet) {
	if s.hasFlags {
		fs.Bool("test-flag", false, "A test flag.")
	}
}

func TestAdapter(t *testing.T) {
	cases := map[string]struct {
		svc           any
		wantAddr      bool
		wantAdminAddr bool
		wantStateDir  bool
		wantCustom    bool
		wantReady     int
		runErr        string
		stateDir      string
	}{
		"public only": {
			svc: &publicOnlyService{
				hasPublic: true,
				startupWG: &sync.WaitGroup{},
			},
			wantAddr:  true,
			wantReady: 1,
		},
		"admin only": {
			svc: &adminOnlyService{
				hasAdmin:  true,
				startupWG: new(sync.WaitGroup),
			},
			wantAdminAddr: true,
			wantReady:     1,
		},
		"runner only": {
			svc:       &runnerOnlyService{hasRunner: true},
			wantReady: 0,
		},
		"stateful service": {
			svc: &statefulService{
				hasRunner: true,
				hasState:  true,
			},
			wantStateDir: true,
			wantReady:    0,
			stateDir:     "/tmp/test-state",
		},
		"full service": {
			svc: &fullService{
				hasPublic: true,
				hasAdmin:  true,
				hasRunner: true,
				hasFlags:  true,
				startupWG: new(sync.WaitGroup),
			},
			wantAddr:      true,
			wantAdminAddr: true,
			wantCustom:    true,
			wantReady:     2,
		},
		"public endpoint config error": {
			svc:      &publicOnlyService{hasPublic: true, publicErr: errors.New("boom")},
			wantAddr: true,
			runErr:   "configuring public endpoint: boom",
		},
		"admin endpoint config error": {
			svc:           &adminOnlyService{hasAdmin: true, adminErr: errors.New("boom")},
			wantAdminAddr: true,
			runErr:        "configuring admin endpoint: boom",
		},
		"runner error": {
			svc:    &runnerOnlyService{hasRunner: true, runnerErr: errors.New("boom")},
			runErr: "boom",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			a := &adapter{svc: tc.svc}
			fs := flag.NewFlagSet("test", flag.ContinueOnError)
			a.Flags(fs)

			hasFlag := func(name string) bool { return fs.Lookup(name) != nil }
			testutil.AssertEqual(t, hasFlag("addr"), tc.wantAddr)
			testutil.AssertEqual(t, hasFlag("admin-addr"), tc.wantAdminAddr)
			testutil.AssertEqual(t, hasFlag("state-dir"), tc.wantStateDir)
			testutil.AssertEqual(t, hasFlag("test-flag"), tc.wantCustom)
			testutil.AssertEqual(t, hasFlag("exit-idle-time"), true)

			if tc.stateDir != "" {
				a.stateDir = tc.stateDir
			}

			// Use random ports.
			if tc.wantAddr {
				a.addr = "localhost:0"
			}
			if tc.wantAdminAddr {
				a.adminAddr = "localhost:0"
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			var ts *testService
			switch s := tc.svc.(type) {
			case *publicOnlyService:
				ts = (*testService)(s)
			case *adminOnlyService:
				ts = (*testService)(s)
			case *runnerOnlyService:
				ts = (*testService)(s)
			case *fullService:
				ts = (*testService)(s)
			}
			if ts != nil && ts.startupWG != nil {
				ts.startupWG.Add(tc.wantReady)
			}

			runErrCh := make(chan error, 1)
			go func() {
				runErrCh <- a.Run(ctx)
			}()

			// Wait for servers to be ready if needed.
			if ts != nil && ts.startupWG != nil {
				readyCh := make(chan struct{})
				go func() {
					ts.startupWG.Wait()
					close(readyCh)
				}()
				select {
				case <-readyCh:
					// all good
				case <-time.After(3 * time.Second):
					t.Fatal("timed out waiting for servers to start")
				}
			}

			// Cancel to shutdown services that started successfully.
			cancel()

			select {
			case err := <-runErrCh:
				if tc.runErr != "" {
					if err == nil || err.Error() != tc.runErr {
						t.Errorf("a.Run() error = %v, want %q", err, tc.runErr)
					}
				} else if err != nil && !errors.Is(err, context.Canceled) {
					// If context is canceled, errgroup returns Canceled.
					// Otherwise, a clean shutdown should return nil.
					t.Errorf("a.Run() returned unexpected error: %v", err)
				}

				// Verify StatefulService received the state dir.
				if tc.stateDir != "" {
					if s, ok := tc.svc.(*statefulService); ok {
						testutil.AssertEqual(t, (*testService)(s).gotStateDir, tc.stateDir)
					}
				}
			case <-time.After(3 * time.Second):
				t.Fatal("timed out waiting for Run to return")
			}
		})
	}
}
