// © 2024 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the ISC
// license that can be found in the LICENSE.md file.

package web

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"strings"

	"go.astrophena.name/base/cli"
)

type contextKey string

var trustedRequestKey = contextKey("trusted-request")

type trustedRequest struct{}

// IsTrustedRequest reports whether r is a trusted request.
// A trusted request, when resulting in an error handled by [RespondError], will
// have its underlying error message exposed to the client in the HTML response.
func IsTrustedRequest(r *http.Request) bool {
	_, ok := r.Context().Value(trustedRequestKey).(trustedRequest)
	return ok
}

// TrustRequest marks r as a trusted request and returns a new request
// with the trusted status embedded in its context.
// This function should typically be used for requests originating from
// service administrators or other privileged users.
func TrustRequest(r *http.Request) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), trustedRequestKey, trustedRequest{}))
}

// StatusErr is a sentinel error type used to represent HTTP status code errors.
type StatusErr int

// Error implements the error interface.
// It returns a lowercase representation of the HTTP status text for the wrapped code.
func (se StatusErr) Error() string { return strings.ToLower(http.StatusText(int(se))) }

const (
	// ErrBadRequest represents a bad request error (HTTP 400).
	ErrBadRequest StatusErr = http.StatusBadRequest
	// ErrUnauthorized represents an unauthorized access error (HTTP 401).
	ErrUnauthorized StatusErr = http.StatusUnauthorized
	// ErrForbidden represents a forbidden access error (HTTP 403).
	ErrForbidden StatusErr = http.StatusForbidden
	// ErrNotFound represents a not found error (HTTP 404).
	ErrNotFound StatusErr = http.StatusNotFound
	// ErrMethodNotAllowed represents a method not allowed error (HTTP 405).
	ErrMethodNotAllowed StatusErr = http.StatusMethodNotAllowed
	// ErrInternalServerError represents an internal server error (HTTP 500).
	ErrInternalServerError StatusErr = http.StatusInternalServerError
)

// errorResponse is a struct used to represent an error response in JSON format.
type errorResponse struct {
	Status string `json:"status"`
	Error  string `json:"error"`
}

// RespondJSON marshals the provided response object as JSON and writes it to
// the [http.ResponseWriter].
// It sets the Content-Type header to application/json before marshalling.
// In case of marshalling errors, it writes an internal server error with the error message.
func RespondJSON(w http.ResponseWriter, response any) { respondJSON(w, response, false) }

func respondJSON(w http.ResponseWriter, response any, wroteStatus bool) {
	w.Header().Set("Content-Type", "application/json")
	b, err := json.MarshalIndent(response, "", "  ")
	if err != nil {
		if !wroteStatus {
			w.WriteHeader(http.StatusInternalServerError)
		}
		w.Write(fmt.Appendf(nil, `{
  "status": "error",
  "error": "JSON marshal error: %s"
}`, escapeForJSON(err.Error())))
		return
	}
	w.Write(b)
	w.Write([]byte("\n"))
}

var (
	//go:embed templates/error.html
	errorTemplateStr string
	errorTemplate    = template.Must(template.New("error").Funcs(template.FuncMap{
		"static": StaticFS.HashName,
	}).Parse(errorTemplateStr))
)

// RespondError writes an error response in HTML format to w and logs the error
// using [logger.Logf] from context's environment ([cli.Env]) if error is
// [ErrInternalServerError].
//
// If the error is a [StatusErr] or wraps it, it extracts the HTTP status code and
// sets the response status code accordingly. Otherwise, it sets the response
// status code to [http.StatusInternalServerError].
//
// If the request is marked as trusted (see [IsTrustedRequest] and [TrustRequest]),
// the original error message will be included in the HTML response. This is
// useful for debugging by service administrators.
//
// You can wrap any error with [fmt.Errorf] to create a [StatusErr] and set a
// specific HTTP status code:
//
//	// This will set the status code to 404 (Not Found).
//	web.RespondError(w, r, fmt.Errorf("resource %w", web.ErrNotFound))
func RespondError(w http.ResponseWriter, r *http.Request, err error) {
	respondError(false, w, r, err)
}

// RespondJSONError writes an error response in JSON format to w and logs the
// error using [logger.Logf] from context's environment ([cli.Env]) if error is
// [ErrInternalServerError].
//
// If the error is a [StatusErr] or wraps it, it extracts the HTTP status code
// and sets the response status code accordingly. Otherwise, it sets the
// response status code to [http.StatusInternalServerError]. The error message
// is always included in the JSON response.
//
// You can wrap any error with [fmt.Errorf] to create a [StatusErr] and set a
// specific HTTP status code:
//
//	// This will set the status code to 404 (Not Found).
//	web.RespondJSONError(w, r, fmt.Errorf("resource %w", web.ErrNotFound))
func RespondJSONError(w http.ResponseWriter, r *http.Request, err error) {
	respondError(true, w, r, err)
}

func respondError(json bool, w http.ResponseWriter, r *http.Request, err error) {
	logf := cli.GetEnv(r.Context()).Logf

	var se StatusErr
	if !errors.As(err, &se) {
		se = ErrInternalServerError
	}
	if json {
		w.Header().Set("Content-Type", "application/json")
	}
	w.WriteHeader(int(se))
	if se == ErrInternalServerError {
		logf("%s: %s %s -> %v", http.StatusText(int(se)), r.Method, r.URL.Path, err)
	}
	if json {
		respondJSON(w, &errorResponse{Status: "error", Error: err.Error()}, true)
		return
	}

	data := struct {
		StatusCode int
		StatusText string
		IsTrusted  bool
		Error      error // set if IsTrusted is true
	}{
		StatusCode: int(se),
		StatusText: http.StatusText(int(se)),
		IsTrusted:  IsTrustedRequest(r),
	}
	if data.IsTrusted {
		data.Error = err
	}

	var buf bytes.Buffer
	if err := errorTemplate.Execute(&buf, data); err != nil {
		logf("web.RespondError: executing error template failed: %v", err)
		// Fallback, if template execution fails.
		fmt.Fprintf(w, "%d: %s", data.StatusCode, data.StatusText)
		return
	}
	buf.WriteTo(w)
}

func escapeForJSON(s string) string {
	var sb strings.Builder
	for _, ch := range s {
		switch ch {
		case '\\', '"', '/', '\b', '\n', '\r', '\t':
			// Escape these characters with a backslash.
			sb.WriteRune('\\')
			sb.WriteRune(ch)
		default:
			sb.WriteRune(ch)
		}
	}
	return sb.String()
}
