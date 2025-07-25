// © 2025 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the ISC
// license that can be found in the LICENSE.md file.

// Pre-commit implements a Git pre-commit hook to run tests before each commit.
package main

import (
	"bytes"
	"errors"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"

	"go.astrophena.name/base/devtools/internal"
)

const hookShellScript = `#!/bin/sh
echo "==> Running pre-commit check..."
go tool pre-commit
`

func main() {
	log.SetFlags(0)
	internal.EnsureRoot()

	isCI := os.Getenv("CI") == "true"

	if !isCI {
		hookPath := filepath.Join(".git", "hooks", "pre-commit")
		if _, err := os.Stat(hookPath); errors.Is(err, fs.ErrNotExist) {
			if err := os.WriteFile(hookPath, []byte(hookShellScript), 0o755); err != nil {
				log.Fatal(err)
			}
		}
	}

	var w bytes.Buffer

	run(&w, "gofmt", "-d", ".")
	if diff := w.String(); diff != "" {
		log.Fatalf("Run gofmt on these files:\n\t%v", diff)
	}

	run(&w, "go", "tool", "staticcheck", "./...")

	if isCI {
		run(&w, "go", "test", "-race", "./...")
	} else {
		run(&w, "go", "test", "./...")
	}

	run(&w, "go", "mod", "tidy", "--diff")

	run(&w, "go", "tool", "addcopyright")
	if isCI {
		run(&w, "git", "diff", "--exit-code")
	}
}

func run(buf *bytes.Buffer, cmd string, args ...string) {
	buf.Reset()
	c := exec.Command(cmd, args...)
	c.Stdout = buf
	c.Stderr = buf
	if err := c.Run(); err != nil {
		log.Fatalf("%s failed: %v:\n%v", cmd, err, buf.String())
	}
}
