// © 2024 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the ISC
// license that can be found in the LICENSE.md file.

package request_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"go.astrophena.name/base/request"
)

func TestMake(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/test" {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		if r.Body == nil {
			http.Error(w, "missing request body", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"message": "success"}`))
	}))
	defer ts.Close()

	cases := map[string]struct {
		params          request.Params
		want            string
		wantErr         bool
		wantInErrorText string
	}{
		"successful request": {
			params: request.Params{
				Method: http.MethodPost,
				URL:    ts.URL + "/test",
				Body:   map[string]string{"key": "value"},
			},
			want: `{"message": "success"}`,
		},
		"successful request with headers": {
			params: request.Params{
				Method: http.MethodPost,
				URL:    ts.URL + "/test",
				Headers: map[string]string{
					"X-Test": "test",
				},
				Body: map[string]string{"key": "value"},
			},
			want: `{"message": "success"}`,
		},
		"custom HTTP client": {
			params: request.Params{
				Method:     http.MethodPost,
				URL:        ts.URL + "/test",
				HTTPClient: &http.Client{},
				Body:       map[string]string{"key": "value"},
			},
			want: `{"message": "success"}`,
		},
		"assumes GET on empty Method": {
			params: request.Params{
				URL: ts.URL + "/test",
			},
			wantErr:         true,
			wantInErrorText: "GET \"" + ts.URL + "/test\": want 200, got 400: invalid request",
		},
		"invalid request method": {
			params: request.Params{
				Method: http.MethodGet,
				URL:    ts.URL + "/test",
			},
			wantErr:         true,
			wantInErrorText: "want 200, got 400: invalid request",
		},
		"invalid request path": {
			params: request.Params{
				Method: http.MethodPost,
				URL:    ts.URL + "/invalid",
			},
			wantErr:         true,
			wantInErrorText: "want 200, got 400: invalid request",
		},
		"invalid value for JSON": {
			params: request.Params{
				Method: http.MethodPost,
				URL:    ts.URL + "/test",
				Body:   make(chan int),
			},
			wantErr:         true,
			wantInErrorText: "json: unsupported type: chan int",
		},
		"scrubbed token": {
			params: request.Params{
				Method: http.MethodPost,
				URL:    ts.URL + "/hello",
				Body:   map[string]string{"key": "value"},
				Headers: map[string]string{
					"X-Token": "hello",
				},
				Scrubber: strings.NewReplacer("hello", "[EXPUNGED]"),
			},
			wantErr:         true,
			wantInErrorText: "[EXPUNGED]\": want 200, got 400: invalid request",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			var resp json.RawMessage
			resp, err := request.Make[json.RawMessage](context.Background(), tc.params)
			if err != nil {
				if !tc.wantErr {
					t.Fatalf("want error %v, got %v", tc.wantErr, err)
				}
			}

			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got none")
				}
				if !strings.Contains(err.Error(), tc.wantInErrorText) {
					t.Fatalf("got error %q, wanted in it %q", err.Error(), tc.wantInErrorText)
				}
			}

			if string(resp) != tc.want {
				t.Errorf("got %q, want %q", resp, tc.want)
			}
		})
	}
}

func TestMakeContentType(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(r.Header.Get("Content-Type")))
	}))
	defer ts.Close()

	cases := map[string]struct {
		body    any
		headers map[string]string
		want    string
	}{
		"JSON default": {
			body: map[string]string{"name": "test"}, want: "application/json",
		},
		"JSON caller header": {
			body:    map[string]string{"name": "test"},
			headers: map[string]string{"Content-Type": "application/vnd.example+json"},
			want:    "application/vnd.example+json",
		},
		"form default": {
			body: url.Values{"name": {"test"}}, want: "application/x-www-form-urlencoded",
		},
		"form caller header": {
			body:    url.Values{"name": {"test"}},
			headers: map[string]string{"content-type": "application/x-www-form-urlencoded; charset=utf-8"},
			want:    "application/x-www-form-urlencoded; charset=utf-8",
		},
		"raw bytes": {
			body: []byte("test"), want: "",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := request.Make[request.Bytes](t.Context(), request.Params{
				Method: http.MethodPost, URL: ts.URL,
				Body: tc.body, Headers: tc.headers,
			})
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tc.want {
				t.Fatalf("Content-Type = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestMakeIgnoreResponse(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/ok" {
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte("not JSON"))
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("invalid request"))
	}))
	defer ts.Close()

	if _, err := request.Make[request.IgnoreResponse](t.Context(), request.Params{
		URL: ts.URL + "/ok", WantStatusCode: http.StatusAccepted,
	}); err != nil {
		t.Fatalf("wanted status with non-JSON body: %v", err)
	}

	_, err := request.Make[request.IgnoreResponse](t.Context(), request.Params{URL: ts.URL + "/fail"})
	var statusErr *request.StatusError
	if !errors.As(err, &statusErr) {
		t.Fatalf("error = %v, want StatusError", err)
	}
	if statusErr.StatusCode != http.StatusBadRequest || string(statusErr.Body) != "invalid request" {
		t.Fatalf("StatusError = %+v", statusErr)
	}
}

func TestMake_Bytes(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ok":
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("raw response body"))
		case "/fail":
			http.Error(w, "something went wrong", http.StatusBadRequest)
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	t.Run("successful request returns raw bytes", func(t *testing.T) {
		want := []byte("raw response body")

		resp, err := request.Make[request.Bytes](context.Background(), request.Params{
			Method: http.MethodGet,
			URL:    ts.URL + "/ok",
		})

		if err != nil {
			t.Fatalf("returned an unexpected error: %v", err)
		}

		if !bytes.Equal(resp, want) {
			t.Errorf("got %q, want %q", string(resp), string(want))
		}
	})

	t.Run("failed request returns status error with body", func(t *testing.T) {
		// http.Error adds a newline character to the body.
		wantBody := []byte("something went wrong\n")

		_, err := request.Make[request.Bytes](context.Background(), request.Params{
			Method: http.MethodGet,
			URL:    ts.URL + "/fail",
		})

		if err == nil {
			t.Fatal("expected an error, but got nil")
		}

		var statusErr *request.StatusError
		if !errors.As(err, &statusErr) {
			t.Fatalf("error is not of type *request.StatusError: %T", err)
		}

		if statusErr.StatusCode != http.StatusBadRequest {
			t.Errorf("StatusError.StatusCode: got %d, want %d", statusErr.StatusCode, http.StatusBadRequest)
		}

		if !bytes.Equal(statusErr.Body, wantBody) {
			t.Errorf("StatusError.Body: got %q, want %q", string(statusErr.Body), string(wantBody))
		}
	})
}

func TestMakeMaxResponseBytes(t *testing.T) {
	const body = `{"ok":true}`
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer ts.Close()

	cases := map[string]func(int64) error{
		"JSON": func(limit int64) error {
			_, err := request.Make[struct{ OK bool }](t.Context(), request.Params{URL: ts.URL, MaxResponseBytes: limit})
			return err
		},
		"bytes": func(limit int64) error {
			_, err := request.Make[request.Bytes](t.Context(), request.Params{URL: ts.URL, MaxResponseBytes: limit})
			return err
		},
		"ignored": func(limit int64) error {
			_, err := request.Make[request.IgnoreResponse](t.Context(), request.Params{URL: ts.URL, MaxResponseBytes: limit})
			return err
		},
	}
	for name, call := range cases {
		t.Run(name, func(t *testing.T) {
			if err := call(int64(len(body))); err != nil {
				t.Fatalf("body at limit: %v", err)
			}
			if err := call(int64(len(body) - 1)); !errors.Is(err, request.ErrResponseTooLarge) {
				t.Fatalf("body over limit: got %v, want ErrResponseTooLarge", err)
			}
		})
	}
}

func TestMakeStatusErrorBodyLimit(t *testing.T) {
	const limit = 64 << 10
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Test", "present")
		w.WriteHeader(http.StatusBadRequest)
		size := limit + 1
		if r.URL.Path == "/exact" {
			size = limit
		}
		_, _ = w.Write([]byte(strings.Repeat("x", size)))
	}))
	defer ts.Close()

	cases := map[string]struct {
		path      string
		max       int64
		wantLen   int
		truncated bool
	}{
		"at default limit":     {path: "/exact", wantLen: limit},
		"over default limit":   {path: "/large", wantLen: limit, truncated: true},
		"smaller caller limit": {path: "/large", max: 10, wantLen: 10, truncated: true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := request.Make[request.IgnoreResponse](t.Context(), request.Params{
				URL: ts.URL + tc.path, MaxResponseBytes: tc.max,
			})
			var statusErr *request.StatusError
			if !errors.As(err, &statusErr) {
				t.Fatalf("error = %v, want StatusError", err)
			}
			if statusErr.StatusCode != http.StatusBadRequest || statusErr.Headers.Get("X-Test") != "present" {
				t.Fatalf("StatusError = %+v", statusErr)
			}
			if len(statusErr.Body) != tc.wantLen || statusErr.BodyTruncated != tc.truncated {
				t.Fatalf("body length = %d, truncated = %t; want %d, %t", len(statusErr.Body), statusErr.BodyTruncated, tc.wantLen, tc.truncated)
			}
			if tc.truncated && !strings.Contains(err.Error(), "[body truncated]") {
				t.Fatalf("error %q does not report truncation", err)
			}
		})
	}
}
