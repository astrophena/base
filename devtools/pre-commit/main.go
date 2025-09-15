// © 2025 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the ISC
// license that can be found in the LICENSE.md file.

// vim: foldmethod=marker

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"go.astrophena.name/base/cli"
	"go.astrophena.name/base/devtools/internal"
	"go.astrophena.name/base/txtar"
)

const hookShellScript = `#!/bin/sh
echo "==> Running pre-commit check..."
go tool pre-commit
`

// Types and helpers {{{

type check struct {
	Run      []string `json:"run"`
	SkipInCI bool     `json:"skip_in_ci"`
	OnlyInCI bool     `json:"only_in_ci"`
}

func loadChecks() ([]check, error) {
	ar, err := txtar.ParseFile(filepath.Join(".devtools", "config.txtar"))
	if err != nil {
		return nil, err
	}
	var checks []check
	for _, f := range ar.Files {
		if f.Name == "pre-commit.json" {
			if err := json.Unmarshal(f.Data, &checks); err != nil {
				return nil, err
			}
		}
	}
	return checks, nil
}

func (c check) run() error {
	if len(c.Run) == 0 {
		return errors.New("check has an empty 'run' field")
	}
	var buf bytes.Buffer
	cmd := exec.Command(c.Run[0], c.Run[1:]...)
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("check %q failed: %v:\n%v", c.Run, err, buf.String())
	}
	return nil
}

// }}}

func main() { cli.Main(cli.AppFunc(realMain)) }

func realMain(ctx context.Context) error { // {{{
	internal.EnsureRoot()
	env := cli.GetEnv(ctx)

	checks, err := loadChecks()
	if err != nil {
		return err
	}

	isCI := env.Getenv("CI") == "true"

	if !isCI {
		hookPath := filepath.Join(".git", "hooks", "pre-commit")
		if _, err := os.Stat(hookPath); errors.Is(err, fs.ErrNotExist) {
			if err := os.WriteFile(hookPath, []byte(hookShellScript), 0o755); err != nil {
				return err
			}
		}
	}

	var checksToRun []check
	for _, c := range checks {
		if isCI && c.SkipInCI {
			continue
		}
		if !isCI && c.OnlyInCI {
			continue
		}
		checksToRun = append(checksToRun, c)
	}

	totalChecks := len(checksToRun)
	for i, c := range checksToRun {
		progressMsg := fmt.Sprintf("[%d/%d] Running check\t%s", i+1, totalChecks, strings.Join(c.Run, " "))
		if isCI {
			fmt.Fprintln(env.Stdout, progressMsg)
		} else {
			fmt.Fprintf(env.Stdout, "\r\033[K%s", progressMsg)
		}

		if err := c.run(); err != nil {
			if !isCI {
				fmt.Fprintln(env.Stdout) // Newline after progress message on failure.
			}
			return err
		}
	}

	if totalChecks > 0 {
		successMsg := fmt.Sprintf("[%d/%d] All checks passed.", totalChecks, totalChecks)
		if isCI {
			fmt.Fprintln(env.Stdout, successMsg)
		} else {
			fmt.Fprintf(env.Stdout, "\r\033[K%s\n", successMsg)
		}
	}

	return nil
} // }}}
