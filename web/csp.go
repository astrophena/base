// © 2025 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the ISC
// license that can be found in the LICENSE.md file.

package web

import (
	"net/http"
	"reflect"
	"sort"
	"strings"
	"sync"
)

// Common CSP source values.
const (
	CSPSelf         = "'self'"
	CSPNone         = "'none'"
	CSPUnsafeInline = "'unsafe-inline'"
	CSPUnsafeEval   = "'unsafe-eval'"
)

// The default Content-Security-Policy.
// Based on https://github.com/tailscale/tailscale/blob/4ad3f01225745294474f1ae0de33e5a86824a744/safeweb/http.go.
var defaultCSP = CSP{
	DefaultSrc:           []string{CSPSelf},
	ScriptSrc:            []string{CSPSelf},
	FrameAncestors:       []string{CSPNone},
	FormAction:           []string{CSPSelf},
	BaseURI:              []string{CSPSelf},
	ObjectSrc:            []string{CSPSelf},
	BlockAllMixedContent: true,
}.Finalize()

// CSP holds the directives for a Content Security Policy. Empty fields are
// omitted. The zero value produces an empty header value.
//
// See https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Content-Security-Policy.
type CSP struct {
	DefaultSrc              []string `csp:"default-src"`
	ScriptSrc               []string `csp:"script-src"`
	StyleSrc                []string `csp:"style-src"`
	ImgSrc                  []string `csp:"img-src"`
	ConnectSrc              []string `csp:"connect-src"`
	FontSrc                 []string `csp:"font-src"`
	ObjectSrc               []string `csp:"object-src"`
	MediaSrc                []string `csp:"media-src"`
	FrameSrc                []string `csp:"frame-src"`
	ChildSrc                []string `csp:"child-src"`
	FormAction              []string `csp:"form-action"`
	FrameAncestors          []string `csp:"frame-ancestors"`
	BaseURI                 []string `csp:"base-uri"`
	Sandbox                 []string `csp:"sandbox"`
	PluginTypes             []string `csp:"plugin-types"`
	ReportURI               []string `csp:"report-uri"`
	ReportTo                []string `csp:"report-to"`
	WorkerSrc               []string `csp:"worker-src"`
	ManifestSrc             []string `csp:"manifest-src"`
	PrefetchSrc             []string `csp:"prefetch-src"`
	NavigateTo              []string `csp:"navigate-to"`
	BlockAllMixedContent    bool     `csp:"block-all-mixed-content"`
	UpgradeInsecureRequests bool     `csp:"upgrade-insecure-requests"`

	str *string
}

// String returns the CSP header value.
func (p CSP) String() string {
	if p.str != nil {
		return *p.str
	}
	return p.compute()
}

func (p CSP) compute() string {
	var directives []string
	val := reflect.ValueOf(p)
	typ := val.Type()

	for i := 0; i < val.NumField(); i++ {
		field := typ.Field(i)
		tag := field.Tag.Get("csp")
		if tag == "" {
			continue
		}

		value := val.Field(i)
		switch value.Kind() {
		case reflect.Slice:
			if value.Len() > 0 {
				sources := value.Interface().([]string)
				directives = append(directives, tag+" "+strings.Join(sources, " "))
			}
		case reflect.Bool:
			if value.Bool() {
				directives = append(directives, tag)
			}
		}
	}

	sort.Strings(directives)
	return strings.Join(directives, "; ")
}

// Finalize returns a copy with its header value cached. Call it after setting
// the policy fields. [CSPMux.Handle] calls Finalize for you.
func (p CSP) Finalize() CSP {
	s := p.compute()
	p.str = &s
	return p
}

// CSPMux selects a Content Security Policy using [http.ServeMux] patterns.
// Create one with [NewCSPMux].
type CSPMux struct {
	mu  sync.RWMutex
	mux *http.ServeMux
	m   map[string]CSP // map from pattern to CSP
}

// NewCSPMux creates a new [CSPMux].
func NewCSPMux() *CSPMux {
	return &CSPMux{
		mux: http.NewServeMux(),
		m:   make(map[string]CSP),
	}
}

// Handle registers a policy for pattern. It panics if the pattern is invalid,
// conflicts with another pattern, or is already registered.
func (mux *CSPMux) Handle(pattern string, policy CSP) {
	mux.mu.Lock()
	defer mux.mu.Unlock()

	if _, exist := mux.m[pattern]; exist {
		panic("web: multiple registrations for " + pattern)
	}

	// Use a dummy handler. We only care about the pattern matching.
	mux.mux.Handle(pattern, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	mux.m[pattern] = policy.Finalize()
}

// PolicyFor returns the policy for the best matching pattern. It returns a
// zero CSP and false when no pattern matches.
func (mux *CSPMux) PolicyFor(r *http.Request) (CSP, bool) {
	mux.mu.RLock()
	defer mux.mu.RUnlock()

	// Find the matching pattern using the internal ServeMux.
	_, pattern := mux.mux.Handler(r)
	if policy, ok := mux.m[pattern]; ok {
		return policy, true
	}

	return CSP{}, false
}
