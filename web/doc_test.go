// © 2025 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the ISC
// license that can be found in the LICENSE.md file.

package web_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"

	"go.astrophena.name/base/web"
)

// Demonstrates how to use the response helpers within a custom HTTP handler.
func ExampleServer_customHandler() {
	// A simple struct for our API response.
	type User struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}

	users := map[int]User{
		1: {ID: 1, Name: "Alice"},
	}

	// This handler returns a user by ID or an error if the user is not found.
	getUserHandler := func(w http.ResponseWriter, r *http.Request) {
		idStr := r.PathValue("id")
		id, err := strconv.Atoi(idStr)
		if err != nil {
			// Respond with a 400 Bad Request error.
			web.RespondJSONError(w, r, fmt.Errorf("%w: invalid user id", web.ErrBadRequest))
			return
		}

		user, ok := users[id]
		if !ok {
			// Respond with a 404 Not Found error.
			web.RespondJSONError(w, r, fmt.Errorf("%w: user not found", web.ErrNotFound))
			return
		}

		// Respond with the user data as JSON.
		web.RespondJSON(w, user)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /users/{id}", getUserHandler)

	s := &web.Server{Mux: mux}

	// Simulate requests to test the handler.

	// Successful request.
	req1 := httptest.NewRequest(http.MethodGet, "/users/1", nil)
	w1 := httptest.NewRecorder()
	s.ServeHTTP(w1, req1)
	fmt.Println("Successful response:")
	fmt.Println(w1.Body.String())

	// Not found request.
	req2 := httptest.NewRequest(http.MethodGet, "/users/2", nil)
	w2 := httptest.NewRecorder()
	s.ServeHTTP(w2, req2)
	fmt.Println("Not found response:")
	fmt.Println(w2.Body.String())

	// Output:
	// Successful response:
	// {
	//   "id": 1,
	//   "name": "Alice"
	// }
	//
	// Not found response:
	// {
	//   "status": "error",
	//   "error": "not found: user not found"
	// }
}

// Shows how to set up a server with debugging and health check endpoints.
func ExampleServer_withDebugAndHealth() {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "Main page")
	})

	s := &web.Server{
		Mux:        mux,
		Debuggable: true, // This enables the /debug/ endpoint.
	}

	// The /health endpoint is enabled by default. We can add a custom check.
	h := web.Health(s.Mux)
	h.RegisterFunc("database", func() (status string, ok bool) {
		// In a real app, you would check the database connection.
		return "connected", true
	})

	// To prevent the example from blocking, we don't actually run ListenAndServe.
	// In a real application, you would call s.ListenAndServe(ctx).

	// Let's test the health endpoint.
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)

	fmt.Println(w.Body.String())

	// Output:
	// {
	//   "ok": true,
	//   "checks": {
	//     "database": {
	//       "status": "connected",
	//       "ok": true
	//     }
	//   }
	// }
}
