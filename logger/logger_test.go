// © 2024 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the ISC
// license that can be found in the LICENSE.md file.

package logger

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"go.astrophena.name/base/testutil"
)

func TestLogger(t *testing.T) {
	var buf bytes.Buffer
	level := new(slog.LevelVar)
	level.Set(slog.LevelDebug)
	h := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: level})
	l := &Logger{
		Logger: slog.New(h),
		Level:  level,
	}

	ctx := context.Background()

	t.Run("DefaultLogger", func(t *testing.T) {
		// This will use the default logger, which discards everything.
		// We check that it doesn't panic and that the buffer is empty.
		buf.Reset()
		Info(ctx, "hello")
		testutil.AssertEqual(t, buf.Len(), 0)
		testutil.AssertEqual(t, IsDefault(Get(ctx)), true)
	})

	ctx = Put(ctx, l)
	testutil.AssertEqual(t, IsDefault(Get(ctx)), false)

	t.Run("Get", func(t *testing.T) {
		got := Get(ctx)
		testutil.AssertEqual(t, got, l)
	})

	t.Run("LevelVar", func(t *testing.T) {
		got := LevelVar(ctx)
		testutil.AssertEqual(t, got, l.Level)

		buf.Reset()
		Debug(ctx, "should be logged")
		if buf.Len() == 0 {
			t.Fatal("expected log message, but buffer is empty")
		}

		got.Set(slog.LevelInfo)
		buf.Reset()
		Debug(ctx, "should not be logged")
		if buf.Len() != 0 {
			t.Fatalf("expected empty buffer, but got: %s", buf.String())
		}

		Info(ctx, "should be logged")
		if buf.Len() == 0 {
			t.Fatal("expected log message for info, but buffer is empty")
		}

		// Reset for other tests
		got.Set(slog.LevelDebug)
	})

	t.Run("Info", func(t *testing.T) {
		buf.Reset()
		Info(ctx, "info message", slog.String("key", "value"))
		if !strings.Contains(buf.String(), `"level":"INFO"`) {
			t.Errorf("log output should contain INFO level")
		}
		if !strings.Contains(buf.String(), `"msg":"info message"`) {
			t.Errorf("log output should contain correct message")
		}
		if !strings.Contains(buf.String(), `"key":"value"`) {
			t.Errorf("log output should contain correct attributes")
		}
	})

	t.Run("Debug", func(t *testing.T) {
		buf.Reset()
		Debug(ctx, "debug message", slog.Bool("ok", true))
		if !strings.Contains(buf.String(), `"level":"DEBUG"`) {
			t.Errorf("log output should contain DEBUG level")
		}
		if !strings.Contains(buf.String(), `"msg":"debug message"`) {
			t.Errorf("log output should contain correct message")
		}
		if !strings.Contains(buf.String(), `"ok":true`) {
			t.Errorf("log output should contain correct attributes")
		}
	})

	t.Run("Warn", func(t *testing.T) {
		buf.Reset()
		Warn(ctx, "warn message", slog.Int("code", 123))
		if !strings.Contains(buf.String(), `"level":"WARN"`) {
			t.Errorf("log output should contain WARN level")
		}
		if !strings.Contains(buf.String(), `"msg":"warn message"`) {
			t.Errorf("log output should contain correct message")
		}
		if !strings.Contains(buf.String(), `"code":123`) {
			t.Errorf("log output should contain correct attributes")
		}
	})

	t.Run("Error", func(t *testing.T) {
		buf.Reset()
		Error(ctx, "error message", slog.Any("err", "some error"))
		if !strings.Contains(buf.String(), `"level":"ERROR"`) {
			t.Errorf("log output should contain ERROR level")
		}
		if !strings.Contains(buf.String(), `"msg":"error message"`) {
			t.Errorf("log output should contain correct message")
		}
		if !strings.Contains(buf.String(), `"err":"some error"`) {
			t.Errorf("log output should contain correct attributes")
		}
	})
}
