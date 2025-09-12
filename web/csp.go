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

// CSP source constants.
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

// CSP represents a Content Security Policy.
// The zero value is an empty policy.
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

// Finalize computes and caches the string representation of the policy.
// CSPs are intended to be immutable; Finalize should be called after a policy
// is fully constructed.
func (p CSP) Finalize() CSP {
	s := p.compute()
	p.str = &s
	return p
}

// CSPMux is a multiplexer for Content Security Policies.
// It matches the URL of each incoming request against a list of registered
// patterns and returns the policy for the pattern that most closely matches the URL.
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

// Handle registers the CSP for the given pattern.
// If a policy already exists for pattern, Handle panics.
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

// PolicyFor returns the CSP for the given request.
// It finds the best matching pattern and returns its policy.
// If no pattern matches, it returns a zero CSP and false.
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
