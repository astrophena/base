// © 2026 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the ISC
// license that can be found in the LICENSE.md file.

package metrics

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"slices"
	"sync"
)

type series struct {
	labels []string
	value  float64
	counts []uint64
	count  uint64
}

type family struct {
	d      *descriptor
	series map[string]*series
}

type collector struct {
	defs    map[*descriptor]bool
	collect func(context.Context, *Recorder) error
}

// Registry stores metric values. Its zero value is ready to use. Register
// metrics before recording or exporting values.
type Registry struct {
	mu         sync.RWMutex
	families   map[string]*family
	collectors []collector
}

// Register adds metrics that the program updates with a Writer. It returns an
// error if a name is already used or a definition is invalid.
func (r *Registry) Register(metrics ...Metric) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.register(metrics, false, nil)
}

// RegisterCollector adds counters and gauges read by collect on each scrape.
// The callback reports current values through Recorder. An error fails the
// whole scrape. The callback must not register more metrics.
func (r *Registry) RegisterCollector(metrics []Metric, collect func(context.Context, *Recorder) error) error {
	if collect == nil || len(metrics) == 0 {
		return fmt.Errorf("metrics: collector needs definitions and a callback")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.register(metrics, true, collect)
}

func (r *Registry) register(metrics []Metric, collected bool, collect func(context.Context, *Recorder) error) error {
	if r.families == nil {
		r.families = make(map[string]*family)
	}
	used := make(map[string]bool)
	defs := make(map[*descriptor]bool)
	for _, m := range metrics {
		if m == nil {
			return fmt.Errorf("metrics: nil definition")
		}
		var d *descriptor
		switch v := m.(type) {
		case *Counter:
			if v != nil {
				d = &v.d
			}
		case *Gauge:
			if v != nil {
				d = &v.d
			}
		case *Histogram:
			if v != nil {
				d = &v.d
			}
		}
		if d == nil || !validName(d.name, true) || collected && d.kind == histogramKind {
			return fmt.Errorf("metrics: invalid collector definition")
		}
		if _, ok := r.families[d.name]; ok || used[d.name] {
			return fmt.Errorf("metrics: duplicate metric %q", d.name)
		}
		used[d.name] = true
		defs[d] = true
	}
	// Histogram samples reserve these names even when the family is registered later.
	for name, f := range r.families {
		if f.d.kind == histogramKind {
			for _, suffix := range []string{"_bucket", "_sum", "_count"} {
				if used[name+suffix] {
					return fmt.Errorf("metrics: %q collides with histogram %q", name+suffix, name)
				}
			}
		}
	}
	for _, m := range metrics {
		d := m.descriptor()
		if d.kind != histogramKind {
			continue
		}
		for _, suffix := range []string{"_bucket", "_sum", "_count"} {
			if _, ok := r.families[d.name+suffix]; ok || used[d.name+suffix] {
				return fmt.Errorf("metrics: histogram %q collides with %q", d.name, d.name+suffix)
			}
		}
	}
	for _, m := range metrics {
		d := m.descriptor()
		f := &family{d: d, series: make(map[string]*series)}
		if !collected && len(d.labels) == 0 {
			f.series[""] = &series{counts: make([]uint64, len(d.buckets))}
		}
		r.families[d.name] = f
	}
	if collected {
		r.collectors = append(r.collectors, collector{defs, collect})
	}
	return nil
}

// Writer records values. Its zero value does nothing. It panics if a metric
// is unregistered, has the wrong number of labels, or receives an invalid value.
type Writer struct{ r *Registry }

// Add increases c by a finite, nonnegative delta.
func (w Writer) Add(c *Counter, delta float64, labels ...string) {
	if w.r == nil {
		return
	}
	validValue(delta, true)
	if c == nil {
		panic("metrics: nil counter")
	}
	w.r.update(&c.d, labels, func(s *series) {
		next := s.value + delta
		validValue(next, true)
		s.value = next
	})
}

// Set assigns a finite value to g.
func (w Writer) Set(g *Gauge, value float64, labels ...string) {
	if w.r == nil {
		return
	}
	validValue(value, false)
	if g == nil {
		panic("metrics: nil gauge")
	}
	w.r.update(&g.d, labels, func(s *series) { s.value = value })
}

// Adjust adds a finite delta to g.
func (w Writer) Adjust(g *Gauge, delta float64, labels ...string) {
	if w.r == nil {
		return
	}
	validValue(delta, false)
	if g == nil {
		panic("metrics: nil gauge")
	}
	w.r.update(&g.d, labels, func(s *series) {
		next := s.value + delta
		validValue(next, false)
		s.value = next
	})
}

// Observe records one finite value in h.
func (w Writer) Observe(h *Histogram, value float64, labels ...string) {
	if w.r == nil {
		return
	}
	validValue(value, false)
	if h == nil {
		panic("metrics: nil histogram")
	}
	w.r.update(&h.d, labels, func(s *series) {
		next := s.value + value
		validValue(next, false)
		s.value = next
		s.count++
		for i, b := range h.d.buckets {
			if value <= b {
				s.counts[i]++
				break
			}
		}
	})
}

func validValue(v float64, nonnegative bool) {
	if math.IsNaN(v) || math.IsInf(v, 0) || nonnegative && v < 0 {
		panic("metrics: invalid value")
	}
}

func (r *Registry) update(d *descriptor, labels []string, change func(*series)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	f := r.families[d.name]
	if f == nil || f.d != d || len(labels) != len(d.labels) {
		panic("metrics: unregistered definition or wrong label count")
	}
	key := labelKey(labels)
	s := f.series[key]
	if s == nil {
		s = &series{labels: slices.Clone(labels), counts: make([]uint64, len(d.buckets))}
		f.series[key] = s
	}
	change(s)
}

func labelKey(labels []string) string {
	var buf []byte
	for _, label := range labels {
		buf = binary.AppendUvarint(buf, uint64(len(label)))
		buf = append(buf, label...)
	}
	return string(buf)
}

// Recorder receives current values from a collector. A bad or duplicate value
// makes the scrape fail.
type Recorder struct {
	defs   map[*descriptor]bool
	values map[*descriptor]map[string]*series
	err    error
}

// Counter reports an absolute counter value for this scrape.
func (r *Recorder) Counter(c *Counter, value float64, labels ...string) {
	if c == nil {
		r.err = fmt.Errorf("metrics: nil collector counter")
		return
	}
	r.record(&c.d, value, true, labels)
}

// Gauge reports an absolute gauge value for this scrape.
func (r *Recorder) Gauge(g *Gauge, value float64, labels ...string) {
	if g == nil {
		r.err = fmt.Errorf("metrics: nil collector gauge")
		return
	}
	r.record(&g.d, value, false, labels)
}

func (r *Recorder) record(d *descriptor, value float64, nonnegative bool, labels []string) {
	if r.err != nil {
		return
	}
	if !r.defs[d] || len(labels) != len(d.labels) || math.IsNaN(value) || math.IsInf(value, 0) || nonnegative && value < 0 {
		r.err = fmt.Errorf("metrics: invalid collector value for %q", d.name)
		return
	}
	if r.values[d] == nil {
		r.values[d] = make(map[string]*series)
	}
	key := labelKey(labels)
	if _, ok := r.values[d][key]; ok {
		r.err = fmt.Errorf("metrics: duplicate collector sample for %q", d.name)
		return
	}
	r.values[d][key] = &series{labels: slices.Clone(labels), value: value}
}
