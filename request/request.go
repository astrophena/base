// © 2024 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the ISC
// license that can be found in the LICENSE.md file.

// Package request provides utilities for making HTTP requests.
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

// DefaultClient is a [http.Client] with nice defaults.
var DefaultClient = &http.Client{
	Timeout: 10 * time.Second,
}

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
	// HTTPClient is an optional custom HTTP client object to use for the request.
	// If not provided, DefaultClient will be used.
	HTTPClient *http.Client
	// Scrubber is an optional strings.Replacer that scrubs unwanted data from
	// error messages.
	Scrubber *strings.Replacer
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

// Make makes a HTTP request with the provided parameters and unmarshals the
// response body into the specified type.
//
// It supports JSON or URL-encoded format for request bodies and JSON for
// request responses.
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
		return resp, scrubErr(fmt.Errorf("%s %q: want 200, got %d: %s", p.Method, p.URL, res.StatusCode, b), p.Scrubber)
	}

	if err := json.Unmarshal(b, &resp); err != nil {
		return resp, scrubErr(err, p.Scrubber)
	}

	return resp, nil
}
