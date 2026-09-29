// © 2026 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the ISC
// license that can be found in the LICENSE.md file.

package web

import (
	"context"
	"net/http"
	"runtime"
	"strconv"
	"time"

	"go.astrophena.name/base/metrics"
)

var (
	requestLabels   = []string{"endpoint", "method", "route", "status"}
	requests        = metrics.MustCounter("http_server_requests_total", "HTTP requests completed.", requestLabels...)
	requestDuration = metrics.MustHistogram("http_server_request_duration_seconds", "Time until response or hijack, in seconds.",
		[]float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10}, requestLabels...)
	responseSize = metrics.MustHistogram("http_server_response_size_bytes", "HTTP response body bytes before hijack.",
		[]float64{1024, 4096, 16384, 65536, 262144, 1048576, 4194304}, requestLabels...)
	inFlight            = metrics.MustGauge("http_server_requests_in_flight", "HTTP requests in flight.", "endpoint", "method", "route")
	connectionsAccepted = metrics.MustCounter("http_server_connections_accepted_total", "HTTP connections accepted.", "endpoint")
	connectionsOpen     = metrics.MustGauge("http_server_connections_open", "Connections managed by the HTTP server.", "endpoint")
	connectionsHijacked = metrics.MustCounter("http_server_connections_hijacked_total", "HTTP connections hijacked.", "endpoint")
	hijackedOpen        = metrics.MustGauge("http_server_connections_hijacked_open", "Open hijacked connections.", "endpoint")
	hijackedDuration    = metrics.MustHistogram("http_server_hijacked_connection_duration_seconds", "Time from hijack until connection close.",
		[]float64{1, 5, 15, 60, 300, 900, 3600, 21600}, "endpoint", "method", "route")

	goGoroutines  = metrics.MustGauge("go_goroutines", "Current goroutine count.")
	goGOMAXPROCS  = metrics.MustGauge("go_gomaxprocs", "Current GOMAXPROCS value.")
	goHeapAlloc   = metrics.MustGauge("go_heap_alloc_bytes", "Bytes allocated on the heap.")
	goHeapObjects = metrics.MustGauge("go_heap_objects", "Objects allocated on the heap.")
	goHeapSys     = metrics.MustGauge("go_heap_sys_bytes", "Heap bytes obtained from the system.")
	goStackInuse  = metrics.MustGauge("go_stack_inuse_bytes", "Bytes in stack spans.")
	goAllocTotal  = metrics.MustCounter("go_alloc_bytes_total", "Cumulative bytes allocated.")
	goGCCycles    = metrics.MustCounter("go_gc_cycles_total", "Completed GC cycles.")
	goGCPause     = metrics.MustCounter("go_gc_pause_seconds_total", "Cumulative GC pause duration in seconds.")
)

// RegisterMetrics adds HTTP and Go runtime metrics to r. Call it before serving
// requests. Use [metrics.Put] on the context passed to [Server.ListenAndServe]
// or on each request passed to [Server.ServeHTTP]. web/service.Run does both.
// Runtime values are read on scrape.
func RegisterMetrics(r *metrics.Registry) error {
	if err := r.Register(requests, requestDuration, responseSize, inFlight,
		connectionsAccepted, connectionsOpen, connectionsHijacked, hijackedOpen, hijackedDuration); err != nil {
		return err
	}
	return r.RegisterCollector([]metrics.Metric{
		goGoroutines, goGOMAXPROCS, goHeapAlloc, goHeapObjects, goHeapSys,
		goStackInuse, goAllocTotal, goGCCycles, goGCPause,
	}, collectRuntime)
}

func collectRuntime(_ context.Context, rec *metrics.Recorder) error {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	rec.Gauge(goGoroutines, float64(runtime.NumGoroutine()))
	rec.Gauge(goGOMAXPROCS, float64(runtime.GOMAXPROCS(0)))
	rec.Gauge(goHeapAlloc, float64(m.HeapAlloc))
	rec.Gauge(goHeapObjects, float64(m.HeapObjects))
	rec.Gauge(goHeapSys, float64(m.HeapSys))
	rec.Gauge(goStackInuse, float64(m.StackInuse))
	rec.Counter(goAllocTotal, float64(m.TotalAlloc))
	rec.Counter(goGCCycles, float64(m.NumGC))
	rec.Counter(goGCPause, float64(m.PauseTotalNs)/float64(time.Second))
	return nil
}

func (s *Server) metricsEndpoint() string {
	if s.MetricsEndpoint != "" {
		return s.MetricsEndpoint
	}
	return "server"
}

func metricMethod(method string) string {
	switch method {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch,
		http.MethodDelete, http.MethodHead, http.MethodOptions, http.MethodConnect, http.MethodTrace:
		return method
	default:
		return "OTHER"
	}
}

func (s *Server) recordRequest(ctx context.Context, endpoint, method, route string, status, size int, elapsed time.Duration) {
	w := metrics.Get(ctx)
	statusLabel := strconv.Itoa(status)
	w.Add(requests, 1, endpoint, method, route, statusLabel)
	w.Observe(requestDuration, elapsed.Seconds(), endpoint, method, route, statusLabel)
	w.Observe(responseSize, float64(size), endpoint, method, route, statusLabel)
}
