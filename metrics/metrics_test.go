// © 2026 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the ISC
// license that can be found in the LICENSE.md file.

package metrics

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestDefinitionsAndRegistration(t *testing.T) {
	for _, name := range []string{"", "1bad", "bad-name"} {
		if _, err := NewCounter(name, "help"); err == nil {
			t.Errorf("NewCounter(%q) succeeded", name)
		}
	}
	if _, err := NewGauge("valid", "help", "bad:label"); err == nil {
		t.Fatal("accepted invalid label name")
	}
	if _, err := NewHistogram("valid", "help", []float64{1, 1}); err == nil {
		t.Fatal("accepted duplicate buckets")
	}

	r := new(Registry)
	h := MustHistogram("duration", "help", []float64{1})
	if err := r.Register(h); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(MustGauge("duration_sum", "help")); err == nil {
		t.Fatal("accepted histogram sample collision")
	}
	if err := r.Register((*Counter)(nil)); err == nil {
		t.Fatal("accepted typed nil definition")
	}
	if err := r.Register(MustCounter("other", "help"), MustGauge("duration", "help")); err == nil {
		t.Fatal("accepted duplicate in batch")
	}
	if _, ok := r.families["other"]; ok {
		t.Fatal("failed registration changed registry")
	}
}

func TestExport(t *testing.T) {
	r := new(Registry)
	c := MustCounter("jobs_total", "Jobs\\done\nnow", "kind")
	g := MustGauge("queue_size", "Queue size")
	h := MustHistogram("job_seconds", "Job duration", []float64{1, 2}, "kind")
	if err := r.Register(c, g, h); err != nil {
		t.Fatal(err)
	}
	w := Get(Put(context.Background(), r))
	w.Add(c, 2, "a\"b\\c\nd")
	w.Set(g, 3)
	w.Observe(h, .5, "build")
	w.Observe(h, 3, "build")

	var buf bytes.Buffer
	if err := r.Write(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	for _, line := range []string{
		`# HELP jobs_total Jobs\\done\nnow`,
		`jobs_total{kind="a\"b\\c\nd"} 2`,
		`job_seconds_bucket{kind="build",le="1"} 1`,
		`job_seconds_bucket{kind="build",le="2"} 1`,
		`job_seconds_bucket{kind="build",le="+Inf"} 2`,
		`job_seconds_sum{kind="build"} 3.5`,
		`queue_size 3`,
	} {
		if !strings.Contains(buf.String(), line+"\n") {
			t.Errorf("snapshot missing %q:\n%s", line, &buf)
		}
	}

	rec := httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != buf.String() {
		t.Fatalf("handler differs from Write: status %d, body %q", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "text/plain; version=0.0.4; charset=utf-8" {
		t.Fatalf("Content-Type = %q", got)
	}
	rec = httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/metrics", nil))
	if rec.Code != http.StatusMethodNotAllowed || rec.Header().Get("Allow") != http.MethodGet {
		t.Fatalf("POST status %d, Allow %q", rec.Code, rec.Header().Get("Allow"))
	}
}

func TestCollectorFailureLeavesWriterUntouched(t *testing.T) {
	r := new(Registry)
	g := MustGauge("temperature", "Temperature", "room")
	fail := false
	if err := r.RegisterCollector([]Metric{g}, func(ctx context.Context, rec *Recorder) error {
		rec.Gauge(g, 21, "living")
		if fail {
			return errors.New("sensor failed")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := r.Write(context.Background(), &buf); err != nil || !strings.Contains(buf.String(), `temperature{room="living"} 21`) {
		t.Fatalf("first scrape: %v, %q", err, &buf)
	}
	fail = true
	buf.Reset()
	if err := r.Write(context.Background(), &buf); err == nil || buf.Len() != 0 {
		t.Fatalf("failed scrape: %v, %q", err, &buf)
	}
	rec := httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusServiceUnavailable || strings.Contains(rec.Body.String(), "temperature") {
		t.Fatalf("failed HTTP scrape: %d, %q", rec.Code, rec.Body.String())
	}
}

func TestConcurrentRecordingAndNoop(t *testing.T) {
	c := MustCounter("work_total", "Work")
	Get(context.Background()).Add(c, 1)
	var zero Writer
	zero.Add(c, 1)
	r := new(Registry)
	if err := r.Register(c); err != nil {
		t.Fatal(err)
	}
	w := Get(Put(context.Background(), r))
	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() {
			for range 100 {
				w.Add(c, 1)
			}
		})
	}
	wg.Wait()
	var buf bytes.Buffer
	if err := r.Write(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "work_total 1000\n") {
		t.Fatalf("unexpected count: %s", &buf)
	}
}
