// © 2024 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the ISC
// license that can be found in the LICENSE.md file.

// Package cli provides helpers for creating simple, single-command
// command-line applications.
package cli

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"runtime"
	"runtime/pprof"

	"go.astrophena.name/base/logger"
	"go.astrophena.name/base/syncx"
	"go.astrophena.name/base/version"
)

// Main runs an application, handling signal-based cancellation and printing errors
// to stderr. It is intended to be called directly from a program's main function.
func Main(app App) {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	err := Run(ctx, app)

	if err == nil {
		return
	}

	if isPrintableError(err) {
		fmt.Fprintln(os.Stderr, err)
	}
	os.Exit(1)
}

type unprintableError struct{ err error }

func (e *unprintableError) Error() string { return e.err.Error() }
func (e *unprintableError) Unwrap() error { return e.err }

func isPrintableError(err error) bool {
	if errors.Is(err, flag.ErrHelp) {
		return false
	}
	var ue *unprintableError
	return !errors.As(err, &ue)
}

// ErrExitVersion signals that the application should exit successfully after
// printing the version information.
var ErrExitVersion = &unprintableError{errors.New("version flag exit")}

// ErrInvalidArgs indicates that the user provided invalid command-line
// arguments. It should be wrapped with more specific context about the error.
var ErrInvalidArgs = errors.New("invalid arguments")

// App represents a runnable command-line application.
type App interface {
	// Run executes the application's primary logic.
	Run(context.Context) error
}

// HasFlags is an App that can define its own command-line flags.
type HasFlags interface {
	App

	// Flags registers flags with the given FlagSet.
	Flags(*flag.FlagSet)
}

// AppFunc is an adapter to allow the use of ordinary functions as an App.
type AppFunc func(context.Context) error

// Run calls the underlying function.
func (f AppFunc) Run(ctx context.Context) error {
	return f(ctx)
}

type ctxKey int

var envKey ctxKey

// GetEnv retrieves the application's environment from a context.
// If the context has no environment, it returns one based on the current OS.
func GetEnv(ctx context.Context) *Env {
	e, ok := ctx.Value(envKey).(*Env)
	if !ok {
		return OSEnv()
	}
	return e
}

// WithEnv returns a new context that carries the provided application environment.
func WithEnv(ctx context.Context, e *Env) context.Context {
	return context.WithValue(ctx, envKey, e)
}

// Env encapsulates the application's environment, including arguments,
// standard I/O streams, and environment variables.
type Env struct {
	Args   []string
	Getenv func(string) string
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer

	logf syncx.Lazy[logger.Logf]
}

// Logf prints a formatted message to the environment's standard error.
func (e *Env) Logf(format string, args ...any) {
	e.logf.Get(func() logger.Logf {
		return log.New(e.Stderr, "", 0).Printf
	})(format, args...)
}

// OSEnv creates an Env based on the current operating system environment.
func OSEnv() *Env {
	return &Env{
		Args:   os.Args[1:],
		Getenv: os.Getenv,
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}
}

// Run executes an application. It parses flags, handles standard flags like
// -version and -cpuprofile, and then runs the app.
func Run(ctx context.Context, app App) error {
	name := version.CmdName()

	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	if fa, ok := app.(HasFlags); ok {
		fa.Flags(flags)
	}

	var (
		cpuProfile = flags.String("cpuprofile", "", "Write CPU profile to `file`.")
		memProfile = flags.String("memprofile", "", "Write memory profile to `file`.")
	)
	var showVersion bool
	if flags.Lookup("version") == nil {
		flags.BoolVar(&showVersion, "version", false, "Show version.")
	}

	env := GetEnv(ctx)

	flags.Usage = usage(flags, env.Stderr)
	flags.SetOutput(env.Stderr)
	if err := flags.Parse(env.Args); err != nil {
		// Already printed to stderr by flag package, so mark as an unprintable error.
		return &unprintableError{err}
	}
	if *cpuProfile != "" {
		f, err := os.Create(*cpuProfile)
		if err != nil {
			return fmt.Errorf("could not create CPU profile: %w", err)
		}
		defer f.Close()
		if err := pprof.StartCPUProfile(f); err != nil {
			return fmt.Errorf("could not start CPU profile: %w", err)
		}
		defer pprof.StopCPUProfile()
	}

	if showVersion {
		fmt.Fprint(env.Stderr, version.Version())
		return ErrExitVersion
	}

	env.Args = flags.Args()

	if err := app.Run(WithEnv(ctx, env)); err != nil {
		return err
	}

	if *memProfile != "" {
		f, err := os.Create(*memProfile)
		if err != nil {
			return fmt.Errorf("could not create memory profile: %w", err)
		}
		defer f.Close()
		runtime.GC() // get up-to-date statistics
		if err := pprof.WriteHeapProfile(f); err != nil {
			return fmt.Errorf("could not write memory profile: %w", err)
		}
	}

	return nil
}

func usage(flags *flag.FlagSet, stderr io.Writer) func() {
	return func() {
		if docSrc != nil {
			fmt.Fprintf(stderr, "%s\n", doc.Get(parseDocComment))
		}
		fmt.Fprint(stderr, "Available flags:\n\n")
		flags.PrintDefaults()
	}
}

var (
	docSrc []byte
	doc    syncx.Lazy[string]
)

// SetDocComment sets the main documentation for the application, which is
// displayed when a user passes the -help flag. It is intended to be used with
// Go's //go:embed directive.
//
// Example:
//
//	//go:embed doc.go
//	var doc []byte
//
//	func init() { cli.SetDocComment(doc) }
func SetDocComment(src []byte) { docSrc = src }

func parseDocComment() string {
	s := bufio.NewScanner(bytes.NewReader(docSrc))
	var (
		doc       string
		inComment bool
	)
	for s.Scan() {
		line := s.Text()
		if line == "/*" {
			inComment = true
			continue
		}
		if line == "*/" {
			// Comment ended, stop scanning.
			break
		}
		if inComment {
			doc += line + "\n"
		}
	}
	if err := s.Err(); err != nil {
		panic(err)
	}
	return doc
}
