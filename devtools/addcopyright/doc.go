// © 2025 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the ISC
// license that can be found in the LICENSE.md file.

/*
Addcopyright adds a copyright header to specified files.

It recursively walks through the current directory and checks if a file,
based on its extension, should have a copyright header. If the header is
missing, the tool prepends a copyright notice based on a template.

The tool is configured through a .devtools/config.txtar file in the project's
root directory. This file is a txtar archive and can contain the following
files:

  - copyright/exclusions.json: A JSON array of file paths to exclude from
    processing.
  - copyright/template.{ext}: A template for the copyright header for a specific
    file extension (e.g., template.go). The template can contain a
    formatting verb %d for the year.
  - copyright/header.{ext}: A string that identifies an existing copyright header
    for a specific file extension (e.g., header.go). If a file
    starts with this string, it's considered to already have a
    copyright header, and the tool will not add a new one.
*/
package main

import (
	_ "embed"

	"go.astrophena.name/base/cli"
)

//go:embed doc.go
var doc []byte

func init() { cli.SetDocComment(doc) }
