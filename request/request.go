// © 2024 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the ISC
// license that can be found in the LICENSE.md file.

// Package request provides a simplified way to make HTTP requests, especially for JSON APIs.
package request

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Params defines the parameters needed for making an HTTP request.
type Params struct {
	// Method is the HTTP method (GET, POST, etc.) for the request.
	Method string
	// URL is the target URL of the request.
	URL string
	// Headers is a map of key-value pairs for additional request headers.
	Headers map[string]string
	// Body is any data to be sent in the request body. It will be marshaled to
	// JSON or, if it's type is url.Values, as query string with Content-Type
	// header set to "application/x-www-form-urlencoded".
	Body any
	// HTTPClient is an optional custom http.Client to use for the request.
	// If not provided, DefaultClient will be used.
	HTTPClient *http.Client
	// Scrubber is an optional strings.Replacer that scrubs unwanted data from
	// error messages.
	Scrubber *strings.Replacer
}

// DefaultClient is the default [http.Client] used by [Make].
//
// It has a timeout of 10 seconds to prevent requests from hanging indefinitely.
var DefaultClient = &http.Client{
	Timeout: 10 * time.Second,
}

// IgnoreResponse is a type to use with [Make] to skip JSON unmarshaling of the response body.
type IgnoreResponse struct{}

// Bytes is a type to use with [Make] to return the raw response body.
type Bytes []byte

// StatusError represents an error where an HTTP request returned
// an unexpected status code.
type StatusError struct {
	// WantedStatusCode is the HTTP status code that was expected by the caller
	// (e.g., http.StatusOK).
	WantedStatusCode int
	// StatusCode is the actual HTTP status code received in the response.
	StatusCode int
	// Headers are the HTTP headers from the response.
	Headers http.Header
	// Body is the raw body of the HTTP response.
	Body []byte
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("want %d, got %d: %s", e.WantedStatusCode, e.StatusCode, e.Body)
}

// Make sends an HTTP request and tries to parse the response.
//
// The Response type parameter determines how the response body is handled:
//
//   - If Response is [IgnoreResponse], the response body is ignored and no parsing is attempted.
//   - If Response is [Bytes], the raw response body is returned without any parsing.
//   - Otherwise, the response body is expected to be JSON and is unmarshaled into a variable of type Response.
//
// It returns the parsed response of type Response and an error if the request fails or parsing fails.
// For non-200 status codes, it returns an error containing the status code and response body.
func Make[Response any](ctx context.Context, p Params) (Response, error) {
	var resp Response

	var (
		data        []byte
		contentType string
	)
	if p.Body != nil {
		switch v := p.Body.(type) {
		case url.Values:
			data = []byte(v.Encode())
			contentType = "application/x-www-form-urlencoded"
		default:
			var err error
			data, err = json.Marshal(v)
			if err != nil {
				return resp, scrubErr(err, p.Scrubber)
			}
			contentType = "application/json"
		}
	}

	var br io.Reader
	if data != nil {
		br = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, p.Method, p.URL, br)
	if err != nil {
		return resp, scrubErr(err, p.Scrubber)
	}

	if p.Headers != nil {
		for k, v := range p.Headers {
			req.Header.Set(k, v)
		}
	}
	if data != nil && contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	httpc := DefaultClient
	if p.HTTPClient != nil {
		httpc = p.HTTPClient
	}

	res, err := httpc.Do(req)
	if err != nil {
		return resp, scrubErr(err, p.Scrubber)
	}
	defer res.Body.Close()

	b, err := io.ReadAll(res.Body)
	if err != nil {
		return resp, scrubErr(err, p.Scrubber)
	}

	if res.StatusCode != http.StatusOK {
		return resp, scrubErr(fmt.Errorf("%s %q: %w", p.Method, p.URL, &StatusError{
			WantedStatusCode: http.StatusOK,
			StatusCode:       res.StatusCode,
			Headers:          res.Header,
			Body:             b,
		}), p.Scrubber)
	}

	switch v := any(&resp).(type) {
	case *IgnoreResponse:
		return resp, nil
	case *Bytes:
		*v = b
		return resp, nil
	default:
		if err := json.Unmarshal(b, &resp); err != nil {
			return resp, scrubErr(err, p.Scrubber)
		}
	}
	return resp, nil
}

type scrubbedError struct {
	err      error
	scrubber *strings.Replacer
}

func (se *scrubbedError) Error() string {
	if se.scrubber != nil {
		return se.scrubber.Replace(se.err.Error())
	}
	return se.err.Error()
}

func (se *scrubbedError) Unwrap() error { return se.err }

func scrubErr(err error, scrubber *strings.Replacer) error {
	return &scrubbedError{err: err, scrubber: scrubber}
}
