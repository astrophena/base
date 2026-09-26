// © 2025 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the ISC
// license that can be found in the LICENSE.md file.

/*
Addcopyright adds missing copyright headers to Git-listed files with configured
extensions.

Run it from a repository root. Use -check to report missing headers without
writing files, or -dry to show the headers it would add.

Configure it in .devtools/config.txtar:

  - copyright/template.{ext} is a header template. Use %d for the file's
    modification year.
  - copyright/header.{ext} identifies an existing header by its prefix.
  - copyright/exclusions.json lists glob patterns or path suffixes to skip.
*/
package main

import (
	_ "embed"

	"go.astrophena.name/base/cli"
)

//go:embed doc.go
var doc []byte

func init() { cli.SetDocComment(doc) }
