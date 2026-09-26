// © 2025 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the ISC
// license that can be found in the LICENSE.md file.

/*
Pre-commit runs repository checks. Outside CI, its first run also installs a
.git/hooks/pre-commit script that runs the same checks before each commit.

Run it from a repository root. Configure checks in the pre-commit.json entry
of .devtools/config.txtar. Each JSON object has these fields:

  - run: command and arguments, such as ["go", "test", "./..."].
  - skip_in_ci: skip when CI=true.
  - only_in_ci: run only when CI=true.
*/
package main

import (
	_ "embed"

	"go.astrophena.name/base/cli"
)

//go:embed doc.go
var doc []byte

func init() { cli.SetDocComment(doc) }
