// © 2024 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the ISC
// license that can be found in the LICENSE.md file.

// Package logger provides a context-aware logger built on [slog].
package logger

import (
	"context"
	"io"
	"log/slog"
)

type ctxKey string

const loggerKey ctxKey = "logger"

// Logger encapsulates an [slog.Logger] and its [slog.LevelVar] for dynamic level control.
type Logger struct {
	*slog.Logger
	Level *slog.LevelVar
}

var defaultLogger = newDefaultLogger()

func newDefaultLogger() *Logger {
	level := new(slog.LevelVar)
	handler := slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: level})
	return &Logger{
		Logger: slog.New(handler),
		Level:  level,
	}
}

// Put returns a new context with the provided [Logger].
func Put(ctx context.Context, l *Logger) context.Context {
	return context.WithValue(ctx, loggerKey, l)
}

// Get retrieves the [Logger] from the context.
// If the context has no [Logger], it returns a default [Logger] that discards all
// messages.
func Get(ctx context.Context) *Logger {
	if l, ok := ctx.Value(loggerKey).(*Logger); ok {
		return l
	}
	return defaultLogger
}

// IsDefault returns true if l is the default [Logger].
func IsDefault(l *Logger) bool { return l == defaultLogger }

// LevelVar retrieves the [slog.LevelVar] associated with the [Logger] in the context.
// If the context has no [Logger], it returns a [slog.LevelVar] for a default
// [Logger].
func LevelVar(ctx context.Context) *slog.LevelVar {
	if l, ok := ctx.Value(loggerKey).(*Logger); ok {
		return l.Level
	}
	return defaultLogger.Level
}

// Debug logs a debug message.
func Debug(ctx context.Context, msg string, attrs ...slog.Attr) {
	Get(ctx).LogAttrs(ctx, slog.LevelDebug, msg, attrs...)
}

// Info logs an info message.
func Info(ctx context.Context, msg string, attrs ...slog.Attr) {
	Get(ctx).LogAttrs(ctx, slog.LevelInfo, msg, attrs...)
}

// Warn logs a warning message.
func Warn(ctx context.Context, msg string, attrs ...slog.Attr) {
	Get(ctx).LogAttrs(ctx, slog.LevelWarn, msg, attrs...)
}

// Error logs an error message.
func Error(ctx context.Context, msg string, attrs ...slog.Attr) {
	Get(ctx).LogAttrs(ctx, slog.LevelError, msg, attrs...)
}
