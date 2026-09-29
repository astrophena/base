// © 2024 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the ISC
// license that can be found in the LICENSE.md file.

package web

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"

	"go.astrophena.name/base/ctxkey"
	"go.astrophena.name/base/logger"
	"go.astrophena.name/base/web/internal/components"
)

var trustedRequestKey = ctxkey.New("web.isTrustedRequest", false)

// IsTrustedRequest reports whether [TrustRequest] marked r as trusted. For a
// trusted request, [RespondError] shows the error and stack trace in HTML.
func IsTrustedRequest(r *http.Request) bool { return trustedRequestKey.Value(r.Context()) }

// TrustRequest returns a copy of r marked as trusted. [RespondError] shows
// details for trusted requests. It does not change [RespondJSONError].
func TrustRequest(r *http.Request) *http.Request {
	return r.WithContext(trustedRequestKey.WithValue(r.Context(), true))
}

// StatusErr lets an error select an HTTP status code.
type StatusErr int

// Error implements the [error] interface.
func (se StatusErr) Error() string { return strings.ToLower(http.StatusText(int(se))) }

const (
	ErrBadRequest          StatusErr = http.StatusBadRequest          // 400
	ErrUnauthorized        StatusErr = http.StatusUnauthorized        // 401
	ErrForbidden           StatusErr = http.StatusForbidden           // 403
	ErrNotFound            StatusErr = http.StatusNotFound            // 404
	ErrMethodNotAllowed    StatusErr = http.StatusMethodNotAllowed    // 405
	ErrInternalServerError StatusErr = http.StatusInternalServerError // 500
)

type errorResponse struct {
	Status string `json:"status"`
	Error  string `json:"error"`
}

// RespondJSON writes response as JSON. If encoding fails, it writes HTTP 500
// and includes the encoding error in the response.
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

// RespondError writes an HTML error page. It logs HTTP 500 errors.
//
// A [StatusErr] anywhere in err sets the HTTP status. Other errors use HTTP 500.
//
// Trusted requests show the error and stack trace. Other requests show only
// the status page. See [TrustRequest].
//
// Wrap a StatusErr to set a specific status:
//
//	web.RespondError(w, r, fmt.Errorf("resource %w", web.ErrNotFound))
func RespondError(w http.ResponseWriter, r *http.Request, err error) {
	respondError(false, w, r, err)
}

// RespondJSONError writes a JSON error response. It logs HTTP 500 errors.
//
// A [StatusErr] anywhere in err sets the HTTP status. Other errors use HTTP 500.
// The JSON always includes err.Error(), even for untrusted requests. Avoid
// passing sensitive details to this function.
//
// Wrap a StatusErr to set a specific status:
//
//	web.RespondJSONError(w, r, fmt.Errorf("resource %w", web.ErrNotFound))
func RespondJSONError(w http.ResponseWriter, r *http.Request, err error) {
	respondError(true, w, r, err)
}

func respondError(json bool, w http.ResponseWriter, r *http.Request, err error) {
	var se StatusErr
	if !errors.As(err, &se) {
		se = ErrInternalServerError
	}
	if json {
		w.Header().Set("Content-Type", "application/json")
	}
	w.WriteHeader(int(se))
	if se == ErrInternalServerError {
		logger.Error(r.Context(), strings.ToLower(http.StatusText(int(se))), slog.Any("err", err))
	}
	if json {
		respondJSON(w, &errorResponse{Status: "error", Error: err.Error()}, true)
		return
	}

	errorPage := components.ErrorPage{
		StatusCode: int(se),
		StatusText: http.StatusText(int(se)),
		IsTrusted:  IsTrustedRequest(r),
	}
	if errorPage.IsTrusted {
		errorPage.Error = err
		errorPage.Stacktrace = string(debug.Stack())
	}
	if errorPage.StatusCode == http.StatusMethodNotAllowed {
		errorPage.Method = r.Method
	}

	layout := components.Layout{
		Title:      fmt.Sprintf("%d %s", errorPage.StatusCode, errorPage.StatusText),
		Stylesheet: StaticFS.HashName("static/css/main.css"),
		Content:    errorPage.Component(),
	}

	if err := layout.Component().Render(r.Context(), w); err != nil {
		logger.Error(r.Context(), "failed to render error layout", slog.Any("err", err))
		// Fallback, if template execution fails.
		fmt.Fprintf(w, "%d: %s", errorPage.StatusCode, errorPage.StatusText)
		return
	}
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
