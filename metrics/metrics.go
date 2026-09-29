// © 2026 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the ISC
// license that can be found in the LICENSE.md file.

// Package metrics lets a program report numbers: events counted, current
// values, and measurements grouped into ranges.
//
// Define metrics once and register them before the program starts work. Put
// the Registry in a context, then use Get(ctx) to record values. With no
// registry in the context, Get returns a writer that does nothing.
//
// Write and Handler export Prometheus text. Collectors read live values when
// someone asks for a snapshot. Values are lost when the process exits.
// Scraping a socket-activated service starts it, so scrape on demand if the
// service should sleep. Monitor startup failures and startup delay outside
// the service.
package metrics

import (
	"context"
	"fmt"
	"math"
	"slices"
	"strings"

	"go.astrophena.name/base/ctxkey"
)

type kind string

const (
	counterKind   kind = "counter"
	gaugeKind     kind = "gauge"
	histogramKind kind = "histogram"
)

type descriptor struct {
	name, help string
	labels     []string
	buckets    []float64
	kind       kind
}

// Metric is a counter, gauge, or histogram definition. It cannot be changed
// after creation and can be used in several registries.
type Metric interface{ descriptor() *descriptor }

// Counter counts events. It can only increase.
type Counter struct{ d descriptor }

// Gauge holds a value that can rise or fall.
type Gauge struct{ d descriptor }

// Histogram counts observations in ranges set by its bucket boundaries.
type Histogram struct{ d descriptor }

func (c *Counter) descriptor() *descriptor   { return &c.d }
func (g *Gauge) descriptor() *descriptor     { return &g.d }
func (h *Histogram) descriptor() *descriptor { return &h.d }

// NewCounter defines a counter with fixed label names.
func NewCounter(name, help string, labels ...string) (*Counter, error) {
	d, err := newDescriptor(counterKind, name, help, labels, nil)
	if err != nil {
		return nil, err
	}
	return &Counter{d}, nil
}

// MustCounter is like NewCounter but panics on an invalid definition.
func MustCounter(name, help string, labels ...string) *Counter {
	c, err := NewCounter(name, help, labels...)
	if err != nil {
		panic(err)
	}
	return c
}

// NewGauge defines a gauge with fixed label names.
func NewGauge(name, help string, labels ...string) (*Gauge, error) {
	d, err := newDescriptor(gaugeKind, name, help, labels, nil)
	if err != nil {
		return nil, err
	}
	return &Gauge{d}, nil
}

// MustGauge is like NewGauge but panics on an invalid definition.
func MustGauge(name, help string, labels ...string) *Gauge {
	g, err := NewGauge(name, help, labels...)
	if err != nil {
		panic(err)
	}
	return g
}

// NewHistogram defines a histogram. Buckets must contain at least one finite
// boundary, in increasing order. An overflow bucket is added automatically.
func NewHistogram(name, help string, buckets []float64, labels ...string) (*Histogram, error) {
	d, err := newDescriptor(histogramKind, name, help, labels, buckets)
	if err != nil {
		return nil, err
	}
	return &Histogram{d}, nil
}

// MustHistogram is like NewHistogram but panics on an invalid definition.
func MustHistogram(name, help string, buckets []float64, labels ...string) *Histogram {
	h, err := NewHistogram(name, help, buckets, labels...)
	if err != nil {
		panic(err)
	}
	return h
}

func newDescriptor(k kind, name, help string, labels []string, buckets []float64) (descriptor, error) {
	if !validName(name, true) {
		return descriptor{}, fmt.Errorf("metrics: invalid metric name %q", name)
	}
	if strings.ContainsRune(help, '\x00') {
		return descriptor{}, fmt.Errorf("metrics: help for %q contains NUL", name)
	}
	seen := make(map[string]bool, len(labels))
	for _, label := range labels {
		if !validName(label, false) || strings.HasPrefix(label, "__") || seen[label] || (k == histogramKind && label == "le") {
			return descriptor{}, fmt.Errorf("metrics: invalid or duplicate label %q for %q", label, name)
		}
		seen[label] = true
	}
	if k == histogramKind {
		if len(buckets) == 0 {
			return descriptor{}, fmt.Errorf("metrics: histogram %q has no buckets", name)
		}
		for i, b := range buckets {
			if math.IsNaN(b) || math.IsInf(b, 0) || (i > 0 && b <= buckets[i-1]) {
				return descriptor{}, fmt.Errorf("metrics: invalid buckets for %q", name)
			}
		}
	}
	return descriptor{name: name, help: help, labels: slices.Clone(labels), buckets: slices.Clone(buckets), kind: k}, nil
}

func validName(s string, metric bool) bool {
	if s == "" {
		return false
	}
	for i, c := range s {
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '_' || i > 0 && c >= '0' && c <= '9' || metric && c == ':' {
			continue
		}
		return false
	}
	return true
}

var registryKey = ctxkey.New[*Registry]("metrics.Registry", nil)

// Put returns a context carrying r. Register metrics before passing it to work.
func Put(ctx context.Context, r *Registry) context.Context { return registryKey.WithValue(ctx, r) }

// Get returns a Writer for ctx. It does nothing if ctx has no registry.
func Get(ctx context.Context) Writer { return Writer{registryKey.Value(ctx)} }
