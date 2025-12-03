// © 2025 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the ISC
// license that can be found in the LICENSE.md file.

package web

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// Validatable is an interface for types that can validate themselves.
// It is used by [HandleJSON] to automatically validate request bodies.
type Validatable interface {
	Validate() error
}

// HandleJSON provides a wrapper for creating HTTP handlers that work with
// JSON requests and responses. It simplifies the common pattern of decoding a
// JSON request, validating it, executing business logic, and encoding a JSON
// response.
//
// The generic type Req is the expected request body type, and Resp is the
// success response body type.
//
// The handler performs the following steps:
//
//   - If the request method is not GET or HEAD, it attempts to decode the
//     request body into a value of type Req. If decoding fails, it sends a
//     400 Bad Request response.
//   - If the decoded request object implements the [Validatable] interface, its
//     Validate method is called. If validation fails, a 400 Bad Request
//     response is sent.
//   - The provided logic function is called with the request and the decoded
//     request object.
//   - If the logic function returns an error, [RespondJSONError] is used to
//     send an appropriate error response. The error can be wrapped with a
//     [StatusErr] to control the HTTP status code.
//   - If the logic function succeeds, the returned response object of type
//     Resp is sent to the client using [RespondJSON] with a 200 OK status.
func HandleJSON[Req, Resp any](logic func(r *http.Request, req Req) (Resp, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req Req
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			if r.Body == http.NoBody {
				RespondJSONError(w, r, fmt.Errorf("%w: request body is required", ErrBadRequest))
				return
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				// Handle EOF for empty body, which json.Decoder treats as an error.
				if err == io.EOF {
					RespondJSONError(w, r, fmt.Errorf("%w: request body is required", ErrBadRequest))
				} else {
					RespondJSONError(w, r, fmt.Errorf("%w: failed to decode request body: %v", ErrBadRequest, err))
				}
				return
			}
		}

		if v, ok := any(req).(Validatable); ok {
			if err := v.Validate(); err != nil {
				RespondJSONError(w, r, fmt.Errorf("%w: validation failed: %v", ErrBadRequest, err))
				return
			}
		}

		resp, err := logic(r, req)
		if err != nil {
			RespondJSONError(w, r, err)
			return
		}

		RespondJSON(w, resp)
	}
}
