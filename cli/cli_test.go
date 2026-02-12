// © 2025 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the ISC
// license that can be found in the LICENSE.md file.

package cli_test

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"go.astrophena.name/base/cli"
	"go.astrophena.name/base/testutil"
	"go.astrophena.name/base/version"
)

func runTest(t *testing.T, app cli.App, args ...string) (stdout, stderr string, err error) {
	t.Helper()

	var out, errb bytes.Buffer
	env := &cli.Env{
		Args:   args,
		Stdin:  strings.NewReader(""),
		Stdout: &out,
		Stderr: &errb,
		Getenv: func(s string) string { return "" },
	}
	ctx := cli.WithEnv(context.Background(), env)

	runErr := cli.Run(ctx, app)

	return out.String(), errb.String(), runErr
}

// simpleApp prints its args to stdout.
type simpleApp struct{}

func (a *simpleApp) Run(ctx context.Context) error {
	env := cli.GetEnv(ctx)
	for _, arg := range env.Args {
		fmt.Fprintln(env.Stdout, arg)
	}
	return nil
}

// appWithFlags has some flags.
type appWithFlags struct {
	s string
	b bool
	i int
}

func (a *appWithFlags) Flags(f *flag.FlagSet) {
	f.StringVar(&a.s, "s", "default", "string flag")
	f.BoolVar(&a.b, "b", false, "bool flag")
	f.IntVar(&a.i, "i", 0, "int flag")
}

func (a *appWithFlags) Run(ctx context.Context) error {
	env := cli.GetEnv(ctx)
	fmt.Fprintf(env.Stdout, "s=%s, b=%v, i=%d", a.s, a.b, a.i)
	if len(env.Args) > 0 {
		fmt.Fprintf(env.Stdout, ", args=%v", env.Args)
	}
	return nil
}

var errAppFailed = errors.New("app failed deliberately")

// failingApp always returns an error.
var failingApp = cli.AppFunc(func(ctx context.Context) error {
	return errAppFailed
})

// invalidArgsApp returns ErrInvalidArgs.
var invalidArgsApp = cli.AppFunc(func(ctx context.Context) error {
	return fmt.Errorf("%w: missing filename", cli.ErrInvalidArgs)
})

// appWithVersionFlag defines its own -version flag.
type appWithVersionFlag struct {
	version bool
}

func (a *appWithVersionFlag) Flags(f *flag.FlagSet) {
	f.BoolVar(&a.version, "version", false, "app version")
}

func (a *appWithVersionFlag) Run(ctx context.Context) error {
	if a.version {
		fmt.Fprint(cli.GetEnv(ctx).Stdout, "app version printed")
	}
	return nil
}

func TestRun(t *testing.T) {
	t.Run("simple", func(t *testing.T) {
		stdout, stderr, err := runTest(t, &simpleApp{}, "hello", "world")
		testutil.AssertEqual(t, err, nil)
		testutil.AssertEqual(t, stderr, "")
		testutil.AssertEqual(t, stdout, "hello\nworld\n")
	})

	t.Run("failing", func(t *testing.T) {
		_, _, err := runTest(t, failingApp)
		if !errors.Is(err, errAppFailed) {
			t.Fatalf("want err %v, got %v", errAppFailed, err)
		}
	})

	t.Run("invalid args", func(t *testing.T) {
		_, stderr, err := runTest(t, invalidArgsApp)
		if !errors.Is(err, cli.ErrInvalidArgs) {
			t.Fatalf("want err to wrap cli.ErrInvalidArgs, got %v", err)
		}
		testutil.AssertEqual(t, err.Error(), "invalid arguments: missing filename")
		testutil.AssertEqual(t, stderr, "") // Run itself doesn't print the error.
	})

	t.Run("app with flags", func(t *testing.T) {
		t.Run("defaults", func(t *testing.T) {
			stdout, _, err := runTest(t, &appWithFlags{})
			testutil.AssertEqual(t, err, nil)
			testutil.AssertEqual(t, stdout, "s=default, b=false, i=0")
		})
		t.Run("with flags and args", func(t *testing.T) {
			stdout, _, err := runTest(t, &appWithFlags{}, "-s", "foo", "-b", "arg1", "arg2")
			testutil.AssertEqual(t, err, nil)
			testutil.AssertEqual(t, stdout, "s=foo, b=true, i=0, args=[arg1 arg2]")
		})
		t.Run("invalid flag value", func(t *testing.T) {
			_, stderr, err := runTest(t, &appWithFlags{}, "-i", "not-an-int")
			if err == nil {
				t.Fatal("expected an error, got nil")
			}
			// This error from the flag package is wrapped in an unprintableError.
			// cli.Main would not print it, but the flag package prints its own message.
			wantInStderr := "invalid value \"not-an-int\" for flag -i: parse error"
			if !strings.Contains(stderr, wantInStderr) {
				t.Errorf("stderr must contain %q, got: %q", wantInStderr, stderr)
			}
		})
	})

	t.Run("version flag", func(t *testing.T) {
		_, stderr, err := runTest(t, &simpleApp{}, "-version")
		if !errors.Is(err, cli.ErrExitVersion) {
			t.Fatalf("want err %v, got %v", cli.ErrExitVersion, err)
		}
		wantVersionOut := version.Version().String()
		testutil.AssertEqual(t, stderr, wantVersionOut)
	})

	t.Run("app with own version flag", func(t *testing.T) {
		stdout, stderr, err := runTest(t, &appWithVersionFlag{}, "-version")
		testutil.AssertEqual(t, err, nil)
		testutil.AssertEqual(t, stderr, "")
		testutil.AssertEqual(t, stdout, "app version printed")
	})

	t.Run("help flag", func(t *testing.T) {
		_, stderr, err := runTest(t, &simpleApp{}, "-h")
		if !errors.Is(err, flag.ErrHelp) {
			t.Fatalf("expected error to wrap flag.ErrHelp, but it didn't: %v", err)
		}
		if !strings.Contains(stderr, "Available flags:") {
			t.Errorf("stderr must contain 'Available flags:', got: %q", stderr)
		}
		if !strings.Contains(stderr, "To disable the pager, set the NO_PAGER environment variable.") {
			t.Errorf("stderr must contain NO_PAGER documentation, got: %q", stderr)
		}
	})

	t.Run("doc comment", func(t *testing.T) {
		const doc = "/*\nMy Test App\nThis is a test application.\n*/\npackage main"
		cli.SetDocComment([]byte(doc))
		t.Cleanup(func() {
			cli.SetDocComment(nil)
		})

		_, stderr, err := runTest(t, &simpleApp{}, "-h")
		if !errors.Is(err, flag.ErrHelp) {
			t.Fatalf("expected error to wrap flag.ErrHelp, but it didn't: %v", err)
		}

		wantDoc := "My Test App\nThis is a test application.\n"
		if !strings.Contains(stderr, wantDoc) {
			t.Errorf("stderr must contain doc comment %q, got: %q", wantDoc, stderr)
		}
	})

	t.Run("profiling flags", func(t *testing.T) {
		dir := t.TempDir()

		t.Run("CPU profile", func(t *testing.T) {
			cpuProfilePath := filepath.Join(dir, "cpu.prof")
			_, _, err := runTest(t, &simpleApp{}, "-cpuprofile", cpuProfilePath)
			testutil.AssertEqual(t, err, nil)
			if _, err := os.Stat(cpuProfilePath); os.IsNotExist(err) {
				t.Error("CPU profile file was not created")
			}
		})

		t.Run("memory profile", func(t *testing.T) {
			memProfilePath := filepath.Join(dir, "mem.prof")
			_, _, err := runTest(t, &simpleApp{}, "-memprofile", memProfilePath)
			testutil.AssertEqual(t, err, nil)
			if _, err := os.Stat(memProfilePath); os.IsNotExist(err) {
				t.Error("memory profile file was not created")
			}
		})

		t.Run("invalid CPU profile path", func(t *testing.T) {
			invalidPath := filepath.Join(dir, "nonexistent", "cpu.prof")
			_, _, err := runTest(t, &simpleApp{}, "-cpuprofile", invalidPath)
			if err == nil {
				t.Fatal("expected an error, got nil")
			}
			if !strings.Contains(err.Error(), "could not create CPU profile") {
				t.Errorf("error must be about creating cpu profile, got: %v", err)
			}
		})
	})
}

func TestPager(t *testing.T) {
	oldIsTerminal := cli.IsTerminal
	cli.IsTerminal = func(fd int) bool { return true }
	t.Cleanup(func() { cli.IsTerminal = oldIsTerminal })

	runWithTerminal := func(t *testing.T, envVars map[string]string, args ...string) (stdout, stderr string, err error) {
		t.Helper()

		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}

		var out bytes.Buffer
		env := &cli.Env{
			Args:   args,
			Stdin:  strings.NewReader(""),
			Stdout: &out,
			Stderr: w,
			Getenv: func(s string) string {
				if v, ok := envVars[s]; ok {
					return v
				}
				return ""
			},
		}
		ctx := cli.WithEnv(context.Background(), env)

		var stderrBytes []byte
		var wg sync.WaitGroup
		wg.Go(func() {
			stderrBytes, _ = io.ReadAll(r)
		})

		runErr := cli.Run(ctx, &simpleApp{})
		w.Close() // Close writer to unblock ReadAll
		wg.Wait()
		r.Close()

		return out.String(), string(stderrBytes), runErr
	}

	t.Run("pager is used", func(t *testing.T) {
		env := map[string]string{"PAGER": "true"}
		_, stderr, err := runWithTerminal(t, env, "-h")

		if !errors.Is(err, flag.ErrHelp) {
			t.Fatalf("expected ErrHelp, got %v", err)
		}
		if stderr != "" {
			t.Errorf("expected empty stderr because pager should have captured output, got: %q", stderr)
		}
	})

	t.Run("NO_PAGER disables pager", func(t *testing.T) {
		env := map[string]string{"PAGER": "true", "NO_PAGER": "1"}
		_, stderr, err := runWithTerminal(t, env, "-h")

		if !errors.Is(err, flag.ErrHelp) {
			t.Fatalf("expected ErrHelp, got %v", err)
		}
		if !strings.Contains(stderr, "Available flags:") {
			t.Errorf("expected help output on stderr, but it was not found. Stderr: %q", stderr)
		}
	})
}
