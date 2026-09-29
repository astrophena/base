// © 2024 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the ISC
// license that can be found in the LICENSE.md file.

// Package request sends HTTP requests and decodes JSON responses. Use [Bytes]
// for raw responses or [IgnoreResponse] when the response needs no decoding.
// Make reads JSON and raw byte responses into memory. Set
// Params.MaxResponseBytes when the expected size is known. Use net/http for
// large downloads. Unexpected-status bodies are kept up to 64 KiB.
package request

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Params configures a request made by [Make].
type Params struct {
	// Method is the HTTP method. The default is GET.
	Method string
	// URL is the full request URL.
	URL string
	// Headers adds request headers. Make sets Content-Type for JSON and form
	// bodies unless Headers already supplies it.
	Headers map[string]string
	// Body is sent in the request body. Nil sends no body. By default, Make
	// encodes it as JSON. Two types are handled differently:
	//
	//   - url.Values becomes a form-encoded body.
	//   - []byte is sent as-is, without an automatic Content-Type.
	Body any
	// WantStatusCode is the required response status. The default is 200 OK.
	WantStatusCode int
	// HTTPClient is the client used for the request. Nil uses DefaultClient.
	HTTPClient *http.Client
	// MaxResponseBytes limits the body of a successful response. Zero means
	// no limit. It also limits how much of an unexpected-status body is kept.
	MaxResponseBytes int64
	// Scrubber replaces text in the returned error's Error() string. It does
	// not change the underlying error or a StatusError's Body and Headers.
	Scrubber *strings.Replacer
}

// DefaultClient is the [http.Client] used by [Make] when Params.HTTPClient is
// nil. It has a 10-second timeout.
var DefaultClient = &http.Client{
	Timeout: 10 * time.Second,
}

// IgnoreResponse tells [Make] to skip response decoding. Make checks the status
// and drains a successful response body without storing it.
type IgnoreResponse struct{}

// Bytes tells [Make] to return the raw response body.
type Bytes []byte

// ErrResponseTooLarge reports a successful response that exceeds
// Params.MaxResponseBytes.
var ErrResponseTooLarge = errors.New("response body too large")

// StatusError reports a response status that differs from the wanted status
// (200 unless Params.WantStatusCode is set). Use errors.As to inspect it after
// [Make] returns.
type StatusError struct {
	// WantedStatusCode is the expected HTTP status code.
	WantedStatusCode int
	// StatusCode is the HTTP status code received.
	StatusCode int
	// Headers are the response headers.
	Headers http.Header
	// Body contains up to 64 KiB of the response body, or fewer bytes when
	// Params.MaxResponseBytes sets a smaller limit.
	Body []byte
	// BodyTruncated reports whether Body omits bytes from the response.
	BodyTruncated bool
}

// Error includes the expected status, actual status, and retained body.
func (e *StatusError) Error() string {
	if e.BodyTruncated {
		return fmt.Sprintf("want %d, got %d: %s [body truncated]", e.WantedStatusCode, e.StatusCode, e.Body)
	}
	return fmt.Sprintf("want %d, got %d: %s", e.WantedStatusCode, e.StatusCode, e.Body)
}

// Make sends a request and returns its response. It uses ctx for cancellation.
//
// Response controls how the body is used:
//
//   - [IgnoreResponse] skips decoding.
//   - [Bytes] returns the raw body.
//   - Any other type is decoded from JSON, regardless of Content-Type.
//
// If the status differs from Params.WantStatusCode (200 by default), Make
// returns a wrapped [StatusError] with up to 64 KiB of its body. If a successful
// response exceeds Params.MaxResponseBytes, Make returns [ErrResponseTooLarge].
// Use the result only when err is nil.
func Make[Response any](ctx context.Context, p Params) (Response, error) {
	var resp Response
	if p.MaxResponseBytes < 0 {
		return resp, scrubErr(errors.New("max response bytes must not be negative"), p.Scrubber)
	}

	var (
		data        []byte
		contentType string
	)
	if p.Body != nil {
		switch v := p.Body.(type) {
		case []byte:
			data = v
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

	method := http.MethodGet
	if p.Method != "" {
		method = p.Method
	}

	req, err := http.NewRequestWithContext(ctx, method, p.URL, br)
	if err != nil {
		return resp, scrubErr(err, p.Scrubber)
	}

	if p.Headers != nil {
		for k, v := range p.Headers {
			req.Header.Set(k, v)
		}
	}
	if data != nil && contentType != "" {
		if _, ok := req.Header["Content-Type"]; !ok {
			req.Header.Set("Content-Type", contentType)
		}
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

	wantCode := http.StatusOK
	if p.WantStatusCode != 0 {
		wantCode = p.WantStatusCode
	}
	if res.StatusCode == wantCode {
		if _, ok := any(&resp).(*IgnoreResponse); ok {
			tooLarge, err := discardBody(res.Body, p.MaxResponseBytes)
			if err != nil {
				return resp, scrubErr(err, p.Scrubber)
			}
			if tooLarge {
				return resp, scrubErr(fmt.Errorf("%s %q: response exceeds %d bytes: %w", method, p.URL, p.MaxResponseBytes, ErrResponseTooLarge), p.Scrubber)
			}
			return resp, nil
		}
	}

	limit := p.MaxResponseBytes
	if res.StatusCode != wantCode && (limit == 0 || limit > maxStatusErrorBytes) {
		limit = maxStatusErrorBytes
	}
	b, truncated, err := readBody(res.Body, limit)
	if err != nil {
		return resp, scrubErr(err, p.Scrubber)
	}
	if res.StatusCode != wantCode {
		return resp, scrubErr(fmt.Errorf("%s %q: %w", method, p.URL, &StatusError{
			WantedStatusCode: wantCode,
			StatusCode:       res.StatusCode,
			Headers:          res.Header,
			Body:             b,
			BodyTruncated:    truncated,
		}), p.Scrubber)
	}
	if truncated {
		return resp, scrubErr(fmt.Errorf("%s %q: response exceeds %d bytes: %w", method, p.URL, p.MaxResponseBytes, ErrResponseTooLarge), p.Scrubber)
	}

	switch v := any(&resp).(type) {
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

const maxStatusErrorBytes = 64 << 10

func readBody(r io.Reader, limit int64) ([]byte, bool, error) {
	if limit == 0 {
		b, err := io.ReadAll(r)
		return b, false, err
	}
	b, err := io.ReadAll(io.LimitReader(r, limit))
	if err != nil || int64(len(b)) < limit {
		return b, false, err
	}
	more, err := hasMore(r)
	return b, more, err
}

func discardBody(r io.Reader, limit int64) (bool, error) {
	if limit == 0 {
		_, err := io.Copy(io.Discard, r)
		return false, err
	}
	n, err := io.Copy(io.Discard, io.LimitReader(r, limit))
	if err != nil || n < limit {
		return false, err
	}
	return hasMore(r)
}

func hasMore(r io.Reader) (bool, error) {
	var b [1]byte
	_, err := io.ReadFull(r, b[:])
	if err == io.EOF {
		return false, nil
	}
	return err == nil, err
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
