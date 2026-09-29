// © 2026 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the ISC
// license that can be found in the LICENSE.md file.

package metrics

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
)

type snapshot struct {
	d      *descriptor
	series []*series
}

// Write writes a Prometheus text snapshot. If a collector fails, nothing is
// written. If w fails, it may contain part of a snapshot.
func (r *Registry) Write(ctx context.Context, w io.Writer) error {
	data, err := r.gather(ctx)
	if err != nil {
		return err
	}
	if _, err := w.Write(data); err != nil {
		return err
	}
	return nil
}

// Handler serves snapshots on GET. A failed scrape returns HTTP 503.
func (r *Registry) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		data, err := r.gather(req.Context())
		if err != nil {
			http.Error(w, "metrics collection failed", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		_, _ = w.Write(data)
	})
}

func (r *Registry) gather(ctx context.Context) ([]byte, error) {
	r.mu.RLock()
	all := make(map[string]*snapshot, len(r.families))
	for name, f := range r.families {
		s := &snapshot{d: f.d, series: make([]*series, 0, len(f.series))}
		for _, item := range f.series {
			s.series = append(s.series, &series{
				labels: slices.Clone(item.labels), value: item.value,
				counts: slices.Clone(item.counts), count: item.count,
			})
		}
		all[name] = s
	}
	collectors := slices.Clone(r.collectors)
	r.mu.RUnlock()

	for _, c := range collectors {
		rec := &Recorder{defs: c.defs, values: make(map[*descriptor]map[string]*series)}
		if err := c.collect(ctx, rec); err != nil {
			return nil, fmt.Errorf("metrics: collect: %w", err)
		}
		if rec.err != nil {
			return nil, rec.err
		}
		for d, values := range rec.values {
			for _, value := range values {
				all[d.name].series = append(all[d.name].series, value)
			}
		}
	}

	names := make([]string, 0, len(all))
	for name := range all {
		names = append(names, name)
	}
	slices.Sort(names)
	var buf bytes.Buffer
	for _, name := range names {
		s := all[name]
		fmt.Fprintf(&buf, "# HELP %s %s\n# TYPE %s %s\n", name, escape(s.d.help, false), name, s.d.kind)
		slices.SortFunc(s.series, func(a, b *series) int {
			return slices.Compare(a.labels, b.labels)
		})
		for _, item := range s.series {
			writeSeries(&buf, s.d, item)
		}
	}
	return buf.Bytes(), nil
}

func writeSeries(buf *bytes.Buffer, d *descriptor, s *series) {
	if d.kind != histogramKind {
		writeSample(buf, d.name, d.labels, s.labels, s.value, "", "")
		return
	}
	var cumulative uint64
	for i, count := range s.counts {
		cumulative += count
		writeSample(buf, d.name+"_bucket", d.labels, s.labels, float64(cumulative), "le", formatFloat(d.buckets[i]))
	}
	writeSample(buf, d.name+"_bucket", d.labels, s.labels, float64(s.count), "le", "+Inf")
	writeSample(buf, d.name+"_sum", d.labels, s.labels, s.value, "", "")
	writeSample(buf, d.name+"_count", d.labels, s.labels, float64(s.count), "", "")
}

func writeSample(buf *bytes.Buffer, name string, keys, values []string, value float64, extraKey, extraValue string) {
	buf.WriteString(name)
	if len(keys) != 0 || extraKey != "" {
		buf.WriteByte('{')
		for i, key := range keys {
			if i > 0 {
				buf.WriteByte(',')
			}
			fmt.Fprintf(buf, "%s=\"%s\"", key, escape(values[i], true))
		}
		if extraKey != "" {
			if len(keys) != 0 {
				buf.WriteByte(',')
			}
			fmt.Fprintf(buf, "%s=\"%s\"", extraKey, extraValue)
		}
		buf.WriteByte('}')
	}
	fmt.Fprintf(buf, " %s\n", formatFloat(value))
}

func escape(s string, quote bool) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\n", "\\n")
	if quote {
		s = strings.ReplaceAll(s, "\"", "\\\"")
	}
	return s
}

func formatFloat(v float64) string { return strconv.FormatFloat(v, 'g', -1, 64) }
