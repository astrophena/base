// © 2024 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the ISC
// license that can be found in the LICENSE.md file.

/*
Package web provides a collection of functions and types for building web services.

It includes a configurable HTTP server that simplifies common tasks like setting
up middleware, serving static files with cache-busting, and graceful shutdown.
The package also offers helpers for standardized JSON and HTML responses,
including error handling.

# Key types and functions

  - [web.Server]: A configurable HTTP server with support for middleware,
    static file serving, and graceful shutdown.
  - [web.RespondJSON] and [web.RespondError]: Functions for consistent JSON and
    HTML error responses.
  - [web.Health]: A ready-to-use health check handler.
  - [web.Debugger]: A debug endpoint with version info, pprof links, and
    customizable key-value pairs.

# Usage

A simple server can be created and run like this:

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "Hello, world!")
	})

	s := &web.Server{
		Mux:  mux,
		Addr: ":8080",
	}

	if err := s.ListenAndServe(context.Background()); err != nil {
		log.Fatal(err)
	}
*/
package web
