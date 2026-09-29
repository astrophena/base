// © 2026 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the ISC
// license that can be found in the LICENSE.md file.

/*
Package service provides a framework for running services.

A service is a Go object that implements one or more of the following
interfaces:

  - [PublicService]: for services that expose a public-facing HTTP endpoint.
  - [AdminService]: for services that expose an admin/internal HTTP endpoint.
  - [cli.App]: for services that run a background worker.

To run the service, call [Run] in its main function.

# Metrics

Run creates a metrics registry for the service. Implement [MetricService] to
add your own metrics. Use metrics.Get(ctx) to record values in an endpoint or
worker. GET /debug/metrics serves them on the admin endpoint, even if the
debug page is off. Admin middleware also applies to this route.

Scraping a socket-activated service starts it. Scrape on demand if it should
sleep. Values reset when the process restarts. HTTP request time starts after
activation; measure startup delay and queue time at the reverse proxy.

# Deployment and Routing

Services MUST be fronted by a reverse proxy (such as Caddy or nginx).
They have two kinds of endpoints: public and administrative.

The public endpoint is exposed to the internet as-is through the reverse proxy.
The service itself is responsible for any identity verification and
authentication required on public routes.

The administrative endpoint is intended for internal use only. The reverse proxy
is responsible for protecting it and authenticating access to it (for example,
by requiring Tailscale identity verification or HTTP basic auth). The service
does not need to implement any authentication for the admin endpoint; it can
trust that all requests reaching it have already been authorized by the reverse
proxy.
*/
package service
