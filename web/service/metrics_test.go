// © 2026 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the ISC
// license that can be found in the LICENSE.md file.

package service

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.astrophena.name/base/metrics"
	"go.astrophena.name/base/web"
)

type metricApp struct {
	r *metrics.Registry
	g *metrics.Gauge
}

var errEndMetricApp = errors.New("end metric app")

func (a *metricApp) RegisterMetrics(ctx context.Context, r *metrics.Registry) error {
	a.r = r
	a.g = metrics.MustGauge("test_worker_active", "Worker activity")
	return r.Register(a.g)
}

func (a *metricApp) Run(ctx context.Context) error {
	metrics.Get(ctx).Set(a.g, 1)
	return errEndMetricApp
}

func TestAdapterMetricsContext(t *testing.T) {
	a := new(metricApp)
	if err := (&adapter{svc: a}).Run(context.Background()); !errors.Is(err, errEndMetricApp) {
		t.Fatalf("Run error = %v", err)
	}
	var buf bytes.Buffer
	if err := a.r.Write(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "test_worker_active 1\n") || !strings.Contains(buf.String(), "go_goroutines ") {
		t.Fatalf("worker and runtime metrics missing:\n%s", &buf)
	}
}

func TestAdminMetricsRoute(t *testing.T) {
	for _, debuggable := range []bool{false, true} {
		t.Run(map[bool]string{false: "without debug page", true: "with debug page"}[debuggable], func(t *testing.T) {
			r := new(metrics.Registry)
			if err := web.RegisterMetrics(r); err != nil {
				t.Fatal(err)
			}
			mux := http.NewServeMux()
			if err := mountMetrics(mux, debuggable, r); err != nil {
				t.Fatal(err)
			}
			s := &web.Server{Mux: mux, Debuggable: debuggable, MetricsEndpoint: "admin"}
			ctx := metrics.Put(context.Background(), r)
			rec := httptest.NewRecorder()
			s.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/debug/metrics", nil).WithContext(ctx))
			if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "go_goroutines ") {
				t.Fatalf("metrics response: %d, %q", rec.Code, rec.Body.String())
			}
			if debuggable {
				rec = httptest.NewRecorder()
				s.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/debug/", nil).WithContext(ctx))
				if !strings.Contains(rec.Body.String(), "/debug/metrics") {
					t.Fatal("debug page has no metrics link")
				}
			}
		})
	}
}
