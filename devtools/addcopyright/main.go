// © 2024 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the ISC
// license that can be found in the LICENSE.md file.

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"go.astrophena.name/base/cli"
	"go.astrophena.name/base/logger"
	"go.astrophena.name/base/txtar"
)

func listFiles(ctx context.Context) ([]string, error) {
	cmd := exec.CommandContext(ctx, "git", "ls-files", "-z", "--cached", "--others", "--exclude-standard")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	return strings.Split(string(bytes.TrimRight(out, "\x00")), "\x00"), nil
}

type config struct {
	exclusions []string
	headers    map[string]string
	templates  map[string]string
}

func (cfg *config) isExcluded(path string) bool {
	for _, ex := range cfg.exclusions {
		if matched, err := filepath.Match(filepath.FromSlash(ex), filepath.FromSlash(path)); err == nil && matched {
			return true
		}
		if strings.HasSuffix(path, ex) {
			return true
		}
	}
	return false
}

func parseConfig(root string) (*config, error) {
	cfg := &config{
		headers:   make(map[string]string),
		templates: make(map[string]string),
	}

	ar, err := txtar.ParseFile(filepath.Join(root, ".devtools", "config.txtar"))
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
	if _, err := os.Stat(".git"); err != nil {
		return fmt.Errorf("run from the repository root: %w", err)
	}

	cfg, err := parseConfig(".")
	if err != nil {
		return err
	}

	files, err := listFiles(ctx)
	if err != nil {
		return err
	}

	return processFiles(ctx, cfg, files, a.dry, a.check)
}

func processFiles(ctx context.Context, cfg *config, files []string, dry, check bool) error {
	var missing bool

	for _, path := range files {
		if cfg.isExcluded(path) {
			continue
		}
		ext := filepath.Ext(path)
		tmpl, ok := cfg.templates[ext]
		if !ok {
			continue
		}
		header, ok := cfg.headers[ext]
		if !ok {
			continue
		}

		content, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}

		hasHeader := bytes.HasPrefix(content, []byte(header))
		if check {
			if !hasHeader {
				logger.Info(ctx, "file is missing copyright header", slog.String("path", path))
				missing = true
			}
			continue
		}

		if hasHeader {
			continue
		}

		info, err := os.Stat(path)
		if err != nil {
			return err
		}
		hdr := fmt.Sprintf(tmpl, info.ModTime().Year())

		if dry {
			logger.Info(ctx, "would add copyright header", slog.String("path", path), slog.String("header", hdr))
			continue
		}

		if err := os.WriteFile(path, append([]byte(hdr), content...), 0o644); err != nil {
			return err
		}
	}

	if check && missing {
		return errors.New("found one or more files missing copyright headers")
	}

	return nil
}
