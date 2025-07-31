// © 2025 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the ISC
// license that can be found in the LICENSE.md file.

/*
Pre-commit installs and runs a Git pre-commit hook.

On its first run in a non-CI environment, it automatically creates the
.git/hooks/pre-commit script. This script simply calls 'go tool pre-commit'
again, ensuring that the checks are run on every subsequent commit.

Checks are configured through a .devtools.txtar file in the project's root
directory. This file is a txtar archive and can contain a pre-commit.json file.
The pre-commit.json file should contain a JSON array of check objects, each with
the following fields:

  - run: A string array where the first element is the command to run and the
    rest are its arguments (e.g., ["go", "test", "./..."]).
  - skip_in_ci: A boolean that, if true, causes the check to be skipped when
    the CI environment variable is set to "true".
  - only_in_ci: A boolean that, if true, causes the check to run only when the
    CI environment variable is set to "true".
*/
package main

import (
	_ "embed"

	"go.astrophena.name/base/cli"
)

//go:embed doc.go
var doc []byte

func init() { cli.SetDocComment(doc) }
