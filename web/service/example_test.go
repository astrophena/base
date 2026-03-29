// © 2026 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the ISC
// license that can be found in the LICENSE.md file.

package service_test

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"go.astrophena.name/base/web/service"
)

// Here is an example of a simple service that exposes both public and admin
// endpoints, and also runs a background worker.
func Example() { service.Run(new(myService)) }

type myService struct{}

func (s *myService) PublicEndpoint(ctx context.Context) (*service.EndpointConfig, error) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Hello, world!")
	})
	return &service.EndpointConfig{
		Mux: mux,
	}, nil
}

func (s *myService) AdminEndpoint(ctx context.Context) (*service.EndpointConfig, error) {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "OK")
	})
	return &service.EndpointConfig{
		Mux: mux,
	}, nil
}

func (s *myService) Run(ctx context.Context) error {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			slog.InfoContext(ctx, "Doing some background work...")
		case <-ctx.Done():
			return nil
		}
	}
}
