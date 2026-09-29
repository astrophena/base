// © 2026 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the ISC
// license that can be found in the LICENSE.md file.

package metrics_test

import (
	"context"
	"os"

	"go.astrophena.name/base/metrics"
)

func Example() {
	var registry metrics.Registry
	jobs := metrics.MustCounter("jobs_total", "Jobs completed.", "result")
	if err := registry.Register(jobs); err != nil {
		panic(err)
	}

	ctx := metrics.Put(context.Background(), &registry)
	metrics.Get(ctx).Add(jobs, 1, "success")
	if err := registry.Write(ctx, os.Stdout); err != nil {
		panic(err)
	}
	// Output:
	// # HELP jobs_total Jobs completed.
	// # TYPE jobs_total counter
	// jobs_total{result="success"} 1
}

func ExampleRegistry_RegisterCollector() {
	var registry metrics.Registry

	var (
		used      = metrics.MustGauge("system_disk_used_bytes", "Used disk space in bytes.", "mount")
		available = metrics.MustGauge("system_disk_available_bytes", "Available disk space in bytes.", "mount")
	)

	if err := registry.RegisterCollector(
		[]metrics.Metric{used, available},
		func(ctx context.Context, rec *metrics.Recorder) error {
			// A real collector reads the current system state here.
			rec.Gauge(used, 1024, "/")
			rec.Gauge(available, 4096, "/")
			return nil
		}); err != nil {
		panic(err)
	}

	if err := registry.Write(context.Background(), os.Stdout); err != nil {
		panic(err)
	}
	// Output:
	// # HELP system_disk_available_bytes Available disk space in bytes.
	// # TYPE system_disk_available_bytes gauge
	// system_disk_available_bytes{mount="/"} 4096
	// # HELP system_disk_used_bytes Used disk space in bytes.
	// # TYPE system_disk_used_bytes gauge
	// system_disk_used_bytes{mount="/"} 1024
}
