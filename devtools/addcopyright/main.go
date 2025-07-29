// © 2024 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the ISC
// license that can be found in the LICENSE.md file.

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"go.astrophena.name/base/cli"
	"go.astrophena.name/base/devtools/internal"
	"go.astrophena.name/base/txtar"
)

type config struct {
	exclusions []string
	headers    map[string]string
	templates  map[string]string
}

func (cfg *config) isExcluded(path string) bool {
	for _, ex := range cfg.exclusions {
		if strings.HasSuffix(path, ex) {
			return true
		}
	}
	return false
}

func parseConfig() (*config, error) {
	cfg := &config{
		headers:   make(map[string]string),
		templates: make(map[string]string),
	}

	ar, err := txtar.ParseFile(".devtools.txtar")
	if err != nil {
		return nil, err
	}

	for _, f := range ar.Files {
		if f.Name == "copyright/exclusions.json" {
			if err := json.Unmarshal(f.Data, &cfg.exclusions); err != nil {
				return nil, err
			}
		}
		ext := filepath.Ext(f.Name)
		if strings.HasPrefix(f.Name, "copyright/template") {
			cfg.templates[ext] = string(f.Data)
		}
		if strings.HasPrefix(f.Name, "copyright/header") {
			cfg.headers[ext] = strings.TrimSuffix(string(f.Data), "\n")
		}
	}

	return cfg, nil
}

func main() { cli.Main(new(app)) }

type app struct {
	dry   bool
	check bool
}

func (a *app) Flags(fs *flag.FlagSet) {
	fs.BoolVar(&a.dry, "dry", false, "Print the files that would have a copyright header added, without making changes.")
	fs.BoolVar(&a.check, "check", false, "Check if files have copyright headers.")
}

func (a *app) Run(ctx context.Context) error {
	internal.EnsureRoot()

	env := cli.GetEnv(ctx)

	cfg, err := parseConfig()
	if err != nil {
		return err
	}

	var foundMissing bool

	walkErr := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() || cfg.isExcluded(path) {
			return nil
		}
		ext := filepath.Ext(path)
		tmpl, ok := cfg.templates[ext]
		if !ok {
			return nil
		}
		header, ok := cfg.headers[ext]
		if !ok {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		hasHeader := bytes.HasPrefix(content, []byte(header))

		// If in check mode, we just check and record if a header is missing.
		if a.check {
			if !hasHeader {
				env.Logf("File is missing copyright header: %s", path)
				foundMissing = true
			}
			return nil
		}

		// If not in check mode and the header is already present, skip.
		if hasHeader {
			return nil
		}

		// If not in check mode and the header is missing, add it.
		info, err := d.Info()
		if err != nil {
			return err
		}
		year := info.ModTime().Year()
		hdr := fmt.Sprintf(tmpl, year)

		if a.dry {
			env.Logf("Would add copyright header to file %s:\n%s", path, hdr)
			return nil
		}

		var buf bytes.Buffer
		buf.WriteString(hdr)
		buf.Write(content)
		return os.WriteFile(path, buf.Bytes(), 0o644)
	})

	if walkErr != nil {
		return walkErr
	}

	// If in check mode and we found files with missing headers, return an error
	// to produce a non-zero exit code.
	if a.check && foundMissing {
		return fmt.Errorf("found one or more files missing copyright headers")
	}

	return nil
}
