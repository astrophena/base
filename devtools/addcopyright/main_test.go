// © 2026 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the ISC
// license that can be found in the LICENSE.md file.

package main

import (
	"bytes"
	"flag"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"go.astrophena.name/base/logger"
	"go.astrophena.name/base/testutil"
	"go.astrophena.name/base/txtar"
)

var update = flag.Bool("update", false, "update golden files in testdata")

func TestProcessFiles(t *testing.T) {
	t.Parallel()

	testutil.RunGolden(t, "testdata/*.txtar", func(t *testing.T, tc string) []byte {
		tca, err := txtar.ParseFile(tc)
		if err != nil {
			t.Fatal(err)
		}

		dir := t.TempDir()

		// Separate config files (prefixed with "config/") from source files.
		// Config files are assembled into .devtools/config.txtar.
		var configAr txtar.Archive
		var sourceFiles []txtar.File
		var check, dry bool
		for _, f := range tca.Files {
			if after, ok := strings.CutPrefix(f.Name, "config/"); ok {
				if after == "flags" {
					str := string(f.Data)
					if strings.Contains(str, "check") {
						check = true
					}
					if strings.Contains(str, "dry") {
						dry = true
					}
					continue
				}
				configAr.Files = append(configAr.Files, txtar.File{
					Name: after,
					Data: f.Data,
				})
			} else {
				sourceFiles = append(sourceFiles, f)
			}
		}

		// Write .devtools/config.txtar.
		configDir := filepath.Join(dir, ".devtools")
		if err := os.MkdirAll(configDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(configDir, "config.txtar"), txtar.Format(&configAr), 0o644); err != nil {
			t.Fatal(err)
		}

		// Extract source files.
		srcAr := &txtar.Archive{Files: sourceFiles}
		testutil.ExtractTxtar(t, srcAr, dir)

		cfg, err := parseConfig(dir)
		if err != nil {
			t.Fatal(err)
		}

		// Collect source file paths.
		var files []string
		for _, f := range sourceFiles {
			files = append(files, filepath.Join(dir, f.Name))
		}
		sort.Strings(files)

		modTimeFn := func(_ string) (time.Time, error) {
			return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), nil
		}

		var logBuf bytes.Buffer
		l := logger.New(nil)
		opts := &slog.HandlerOptions{
			ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
				if a.Key == slog.TimeKey {
					return slog.Attr{}
				}
				return a
			},
		}
		l.Attach(slog.NewTextHandler(&logBuf, opts))
		ctx := logger.Put(t.Context(), l)

		err = processFiles(ctx, cfg, files, dry, check, modTimeFn)

		// Build a txtar from source files to capture the result.
		ar := new(txtar.Archive)
		if err != nil {
			ar.Files = append(ar.Files, txtar.File{
				Name: "error",
				Data: []byte(err.Error() + "\n"),
			})
		}
		if logBuf.Len() > 0 {
			cleanedLogs := bytes.ReplaceAll(logBuf.Bytes(), []byte(dir), []byte("[DIR]"))
			ar.Files = append(ar.Files, txtar.File{
				Name: "stderr",
				Data: cleanedLogs,
			})
		}

		for _, f := range files {
			rel, err := filepath.Rel(dir, f)
			if err != nil {
				t.Fatal(err)
			}
			content, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			ar.Files = append(ar.Files, txtar.File{
				Name: rel,
				Data: content,
			})
		}
		return txtar.Format(ar)
	}, *update)
}
