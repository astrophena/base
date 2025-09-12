// © 2025 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the ISC
// license that can be found in the LICENSE.md file.

package web_test

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.astrophena.name/base/testutil"
	"go.astrophena.name/base/web"
)

// Test request and response types. {{{

type testRequest struct {
	Name  string `json:"name"`
	Value int    `json:"value"`
}

var errNameRequired = errors.New("name is required")

func (r testRequest) Validate() error {
	if r.Name == "" {
		return errNameRequired
	}
	return nil
}

type testResponse struct {
	Message string `json:"message"`
	Success bool   `json:"success"`
}

// }}}

// Test logic function for various scenarios.
func testLogic(r *http.Request, req testRequest) (testResponse, error) {
	switch req.Name {
	case "error":
		return testResponse{}, fmt.Errorf("%w: resource not found", web.ErrNotFound)
	case "panic":
		panic("test panic")
	default:
		return testResponse{
			Message: fmt.Sprintf("Received: %s with value %d", req.Name, req.Value),
			Success: true,
		}, nil
	}
}

func TestHandleJSON(t *testing.T) {
	handler := web.HandleJSON(testLogic)

	cases := map[string]struct {
		method         string
		body           string
		wantStatusCode int
		wantInBody     string
	}{
		"Successful POST": {
			method:         http.MethodPost,
			body:           `{"name": "test", "value": 123}`,
			wantStatusCode: http.StatusOK,
			wantInBody:     `"message": "Received: test with value 123"`,
		},
		"Invalid JSON": {
			method:         http.MethodPost,
			body:           `{"name": "test", "value": 123`,
			wantStatusCode: http.StatusBadRequest,
			wantInBody:     `"error": "bad request: failed to decode request body`,
		},
		"Empty Body": {
			method:         http.MethodPost,
			body:           ``,
			wantStatusCode: http.StatusBadRequest,
			wantInBody:     `"error": "bad request: request body is required"`,
		},
		"Validation Error": {
			method:         http.MethodPost,
			body:           `{"value": 456}`,
			wantStatusCode: http.StatusBadRequest,
			wantInBody:     `"error": "bad request: validation failed: name is required"`,
		},
		"Logic Error": {
			method:         http.MethodPost,
			body:           `{"name": "error"}`,
			wantStatusCode: http.StatusNotFound,
			wantInBody:     `"error": "not found: resource not found"`,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, "/", strings.NewReader(tc.body))
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			testutil.AssertEqual(t, tc.wantStatusCode, w.Code)

			if !strings.Contains(w.Body.String(), tc.wantInBody) {
				t.Errorf("expected response body to contain %q, but got %q", tc.wantInBody, w.Body.String())
			}
		})
	}
}

func TestHandleJSON_NoRequestBody(t *testing.T) {
	type emptyReq struct{}
	type emptyResp struct {
		OK bool `json:"ok"`
	}

	handler := web.HandleJSON(func(r *http.Request, req emptyReq) (emptyResp, error) {
		return emptyResp{OK: true}, nil
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.AssertEqual(t, http.StatusOK, w.Code)
	testutil.AssertEqual(t, "{\n  \"ok\": true\n}\n", w.Body.String())
}
