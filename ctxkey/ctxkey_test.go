// Copyright (c) Tailscale Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

package ctxkey

import (
	"fmt"
	"io"
	"regexp"
	"testing"
	"time"

	"go.astrophena.name/base/testutil"
)

func TestKey(t *testing.T) {
	ctx := t.Context()

	// Test keys with the same name as being distinct.
	k1 := New("same.Name", "")
	testutil.AssertEqual(t, k1.String(), "same.Name")
	k2 := New("same.Name", "")
	testutil.AssertEqual(t, k2.String(), "same.Name")
	testutil.AssertEqual(t, k1 == k2, false)
	ctx = k1.WithValue(ctx, "hello")
	testutil.AssertEqual(t, k1.Has(ctx), true)
	testutil.AssertEqual(t, k1.Value(ctx), "hello")
	testutil.AssertEqual(t, k2.Has(ctx), false)
	testutil.AssertEqual(t, k2.Value(ctx), "")
	ctx = k2.WithValue(ctx, "goodbye")
	testutil.AssertEqual(t, k1.Has(ctx), true)
	testutil.AssertEqual(t, k1.Value(ctx), "hello")
	testutil.AssertEqual(t, k2.Has(ctx), true)
	testutil.AssertEqual(t, k2.Value(ctx), "goodbye")

	// Test default value.
	k3 := New("mapreduce.Timeout", time.Hour)
	testutil.AssertEqual(t, k3.Has(ctx), false)
	testutil.AssertEqual(t, k3.Value(ctx), time.Hour)
	ctx = k3.WithValue(ctx, time.Minute)
	testutil.AssertEqual(t, k3.Has(ctx), true)
	testutil.AssertEqual(t, k3.Value(ctx), time.Minute)

	// Test incomparable value.
	k4 := New("slice", []int(nil))
	testutil.AssertEqual(t, k4.Has(ctx), false)
	testutil.AssertEqual(t, k4.Value(ctx), []int(nil))
	ctx = k4.WithValue(ctx, []int{1, 2, 3})
	testutil.AssertEqual(t, k4.Has(ctx), true)
	testutil.AssertEqual(t, k4.Value(ctx), []int{1, 2, 3})

	// Accessors should be allocation free.
	testutil.AssertEqual(t, testing.AllocsPerRun(100, func() {
		k1.Value(ctx)
		k1.Has(ctx)
		k1.ValueOk(ctx)
	}), 0.0)

	// Test keys that are created without New.
	var k5 Key[string]
	testutil.AssertEqual(t, k5.String(), "string")
	testutil.AssertEqual(t, k1 == k5, false) // should be different from key created by New
	testutil.AssertEqual(t, k5.Has(ctx), false)
	ctx = k5.WithValue(ctx, "fizz")
	testutil.AssertEqual(t, k5.Value(ctx), "fizz")
	var k6 Key[string]
	testutil.AssertEqual(t, k6.String(), "string")
	testutil.AssertEqual(t, k5 == k6, true)
	testutil.AssertEqual(t, k6.Has(ctx), true)
	ctx = k6.WithValue(ctx, "fizz")

	// Test interface value types.
	var k7 Key[any]
	testutil.AssertEqual(t, k7.Has(ctx), false)
	ctx = k7.WithValue(ctx, "whatever")
	testutil.AssertEqual(t, k7.Value(ctx), "whatever")
	ctx = k7.WithValue(ctx, []int{1, 2, 3})
	testutil.AssertEqual(t, k7.Value(ctx), []int{1, 2, 3})
	ctx = k7.WithValue(ctx, nil)
	testutil.AssertEqual(t, k7.Has(ctx), true)
	testutil.AssertEqual(t, k7.Value(ctx), nil)
	k8 := New[error]("error", io.EOF)
	testutil.AssertEqual(t, k8.Has(ctx), false)
	testutil.AssertEqual(t, k8.Value(ctx), io.EOF)
	ctx = k8.WithValue(ctx, nil)
	testutil.AssertEqual(t, k8.Value(ctx), nil)
	testutil.AssertEqual(t, k8.Has(ctx), true)
	err := fmt.Errorf("read error: %w", io.ErrUnexpectedEOF)
	ctx = k8.WithValue(ctx, err)
	testutil.AssertEqual(t, k8.Value(ctx), err)
	testutil.AssertEqual(t, k8.Has(ctx), true)
}

func TestStringer(t *testing.T) {
	ctx := t.Context()
	assertMatches(t, fmt.Sprint(New("foo.Bar", "").WithValue(ctx, "baz")), regexp.MustCompile("foo.Bar.*baz"))
	assertMatches(t, fmt.Sprint(New("", []int{}).WithValue(ctx, []int{1, 2, 3})), regexp.MustCompile(fmt.Sprintf("%[1]T.*%[1]v", []int{1, 2, 3})))
	assertMatches(t, fmt.Sprint(New("", 0).WithValue(ctx, 5)), regexp.MustCompile("int.*5"))
	assertMatches(t, fmt.Sprint(Key[time.Duration]{}.WithValue(ctx, time.Hour)), regexp.MustCompile(fmt.Sprintf("%[1]T.*%[1]v", time.Hour)))
}

func assertMatches(t *testing.T, got string, re *regexp.Regexp) {
	t.Helper()
	if !re.MatchString(got) {
		t.Fatalf("value does not match regexp:\ngot:    %q\nregexp: %q", got, re)
	}
}
