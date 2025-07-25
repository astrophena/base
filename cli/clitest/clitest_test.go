// © 2025 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the ISC
// license that can be found in the LICENSE.md file.

package clitest_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"go.astrophena.name/base/cli"
	"go.astrophena.name/base/cli/clitest"
)

var (
	// errTest is a sentinel error for testing WantErr.
	errTest = errors.New("test error")
	// errWrapper is an error that wraps errTest.
	errWrapper = fmt.Errorf("wrapped: %w", errTest)
)

// customError is a unique error type for testing WantErrType.
type customError struct {
	msg string
}

func (e *customError) Error() string { return e.msg }

// testApp is a mock cli.App used for testing. Its behavior is controlled
// by the command-line arguments it receives.
type testApp struct {
	// checkFuncRan is used to verify that a Case's CheckFunc was executed.
	checkFuncRan bool
}

// Run executes the mock application's logic.
func (a *testApp) Run(ctx context.Context) error {
	env := cli.GetEnv(ctx)

	// Based on the first argument, perform a specific action.
	if len(env.Args) > 0 {
		switch env.Args[0] {
		case "error_is":
			return errWrapper
		case "error_as":
			return &customError{msg: "custom error"}
		case "print_stdout":
			fmt.Fprint(env.Stdout, "hello from stdout")
		case "print_stderr":
			fmt.Fprint(env.Stderr, "hello from stderr")
		case "read_stdin":
			data, _ := io.ReadAll(env.Stdin)
			fmt.Fprint(env.Stdout, string(data))
		case "read_env":
			fmt.Fprint(env.Stdout, env.Getenv("TEST_VAR"))
		}
	}

	return nil
}

func TestRun(t *testing.T) {
	setup := func(t *testing.T) *testApp {
		return &testApp{}
	}

	cases := map[string]clitest.Case[*testApp]{
		"nothing printed": {
			Args:               []string{},
			WantNothingPrinted: true,
		},
		"stdout": {
			Args:         []string{"print_stdout"},
			WantInStdout: "hello from stdout",
		},
		"stderr": {
			Args:         []string{"print_stderr"},
			WantInStderr: "hello from stderr",
		},
		"stdin": {
			Args:         []string{"read_stdin"},
			Stdin:        strings.NewReader("hello from stdin"),
			WantInStdout: "hello from stdin",
		},
		"env": {
			Args:         []string{"read_env"},
			Env:          map[string]string{"TEST_VAR": "hello from env"},
			WantInStdout: "hello from env",
		},
		"WantErr with errors.Is": {
			Args:    []string{"error_is"},
			WantErr: errTest,
		},
		"WantErrType with errors.As": {
			Args:        []string{"error_as"},
			WantErrType: &customError{},
		},
		"CheckFunc": {
			Args: []string{},
			CheckFunc: func(t *testing.T, a *testApp) {
				a.checkFuncRan = true
				if !a.checkFuncRan {
					t.Error("expected checkFuncRan to be true, but it's false")
				}
			},
		},
	}

	clitest.Run(t, setup, cases)
}
