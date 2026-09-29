// © 2026 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the ISC
// license that can be found in the LICENSE.md file.

package metrics_test

import (
	"context"
	"net/http"

	"go.astrophena.name/base/metrics"
	"go.astrophena.name/base/web/service"
)

// Example_socketActivatedService shows a service that starts when a request
// reaches its systemd socket. service.Run creates the registry, puts it in the
// request context, and mounts GET /debug/metrics on the admin endpoint.
//
// The example.service unit starts the binary with:
//
//	[Service]
//	ExecStart=/usr/local/bin/example -admin-addr=sd-socket:example-admin.socket -exit-idle-time=5m
//
// The matching example-admin.socket unit contains:
//
//	[Socket]
//	ListenStream=/run/example/admin-socket
//	Service=example.service
//	[Install]
//	WantedBy=sockets.target
//
// Enable the socket unit. A request starts example.service. Scraping metrics
// also starts it and resets its idle timer. Scrape on demand if the service
// should sleep. Counters start at zero each time the process starts.
func Example_socketActivatedService() { service.Run(new(socketMetricService)) }

type socketMetricService struct{ requests *metrics.Counter }

func (s *socketMetricService) RegisterMetrics(_ context.Context, r *metrics.Registry) error {
	s.requests = metrics.MustCounter("example_requests_total", "Requests handled.")
	return r.Register(s.requests)
}

func (s *socketMetricService) AdminEndpoint(_ context.Context) (*service.EndpointConfig, error) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /hello", func(w http.ResponseWriter, r *http.Request) {
		metrics.Get(r.Context()).Add(s.requests, 1)
		_, _ = w.Write([]byte("hello\n"))
	})
	return &service.EndpointConfig{Mux: mux}, nil
}
