// © 2024 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the ISC
// license that can be found in the LICENSE.md file.

// Package logger provides a context-aware logger built on [slog].
package logger

import (
	"context"
	"io"
	"log/slog"
	"sync"
)

type ctxKey string

const loggerKey ctxKey = "logger"

// multiHandler fans out log records to multiple handlers.
type multiHandler struct {
	mu       sync.RWMutex
	handlers []slog.Handler
}

func newMultiHandler(handlers ...slog.Handler) *multiHandler {
	return &multiHandler{
		handlers: handlers,
	}
}

func (h *multiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, handler := range h.handlers {
		if handler.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (h *multiHandler) Handle(ctx context.Context, r slog.Record) error {
	h.mu.RLock()
	defer h.mu.RUnlock()
	var firstErr error
	for _, handler := range h.handlers {
		if handler.Enabled(ctx, r.Level) {
			if err := handler.Handle(ctx, r); err != nil && firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

func (h *multiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	h.mu.RLock()
	defer h.mu.RUnlock()
	newHandlers := make([]slog.Handler, len(h.handlers))
	for i, handler := range h.handlers {
		newHandlers[i] = handler.WithAttrs(attrs)
	}
	return newMultiHandler(newHandlers...)
}

func (h *multiHandler) WithGroup(name string) slog.Handler {
	h.mu.RLock()
	defer h.mu.RUnlock()
	newHandlers := make([]slog.Handler, len(h.handlers))
	for i, handler := range h.handlers {
		newHandlers[i] = handler.WithGroup(name)
	}
	return newMultiHandler(newHandlers...)
}

func (h *multiHandler) Attach(handler slog.Handler) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.handlers = append(h.handlers, handler)
}

func (h *multiHandler) Detach(handler slog.Handler) {
	h.mu.Lock()
	defer h.mu.Unlock()
	newHandlers := make([]slog.Handler, 0, len(h.handlers))
	for _, h := range h.handlers {
		if h != handler {
			newHandlers = append(newHandlers, h)
		}
	}
	h.handlers = newHandlers
}

// Logger encapsulates an [slog.Logger] and allows attaching and detaching
// multiple [slog.Handler] at runtime.
//
// It also holds a [slog.LevelVar] that can be used to control the level of handlers that are created with it.
type Logger struct {
	*slog.Logger
	Level   *slog.LevelVar
	handler *multiHandler
}

// New creates a new Logger. The logger initially has no handlers.
// Its LevelVar is initialized to LevelInfo if level is nil.
func New(level *slog.LevelVar) *Logger {
	if level == nil {
		level = new(slog.LevelVar)
		level.Set(slog.LevelInfo)
	}
	mh := newMultiHandler()
	return &Logger{
		Logger:  slog.New(mh),
		Level:   level,
		handler: mh,
	}
}

// Attach attaches a handler to the logger.
func (l *Logger) Attach(h slog.Handler) {
	l.handler.Attach(h)
}

// Detach detaches a handler from the logger.
func (l *Logger) Detach(h slog.Handler) {
	l.handler.Detach(h)
}

var defaultLogger = newDefaultLogger()

func newDefaultLogger() *Logger {
	l := New(nil)
	l.Attach(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: l.Level}))
	return l
}

// Put returns a new context with the provided [Logger].
func Put(ctx context.Context, l *Logger) context.Context {
	return context.WithValue(ctx, loggerKey, l)
}

// Get retrieves the [Logger] from the context.
//
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
//
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
