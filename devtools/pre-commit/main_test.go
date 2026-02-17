package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.astrophena.name/base/cli"
	"go.astrophena.name/base/txtar"
)

func TestProgressMessage(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		current       int
		total         int
		command       []string
		terminalWidth int
		want          string
	}{
		"no terminal width does not shorten": {
			current:       1,
			total:         1,
			command:       []string{"very-long-command", "with", "arguments"},
			terminalWidth: 0,
			want:          "[1/1] Running check very-long-command with arguments",
		},
		"small width with ellipsis": {
			current:       2,
			total:         10,
			command:       []string{"go", "test", "./..."},
			terminalWidth: 28,
			want:          "[2/10] Running check go t...",
		},
		"very small width keeps prefix only": {
			current:       3,
			total:         10,
			command:       []string{"go", "test", "./..."},
			terminalWidth: 10,
			want:          "[3/10] Running check ",
		},
		"very small width trims without ellipsis": {
			current:       2,
			total:         100,
			command:       []string{"go", "test", "./..."},
			terminalWidth: 24,
			want:          "[2/100] Running check go",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {

			got := progressMessage(tc.current, tc.total, tc.command, tc.terminalWidth)
			if got != tc.want {
				t.Fatalf("progressMessage() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestProgressMessageUsesSpaceInsteadOfTab(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		current int
		total   int
		command []string
		width   int
	}{
		"narrow width": {
			current: 1,
			total:   2,
			command: []string{"go", "test", "./..."},
			width:   25,
		},
		"wide width": {
			current: 1,
			total:   2,
			command: []string{"go", "test", "./..."},
			width:   80,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {

			got := progressMessage(tc.current, tc.total, tc.command, tc.width)
			if strings.Contains(got, "\t") {
				t.Fatalf("progressMessage() contains tab: %q", got)
			}
		})
	}
}

type runCase struct {
	CI         string `json:"ci"`
	WantStdout string `json:"want_stdout"`
	WantHook   string `json:"want_hook"`
}

func TestRealMainRunBehaviorFromTxtar(t *testing.T) {

	cases := map[string]struct {
		archive string
	}{
		"ci filters checks and prints summary": {
			archive: "run_in_ci.txtar",
		},
		"non ci installs hook": {
			archive: "run_local_installs_hook.txtar",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {

			dir, config := extractRunCase(t, filepath.Join("testdata", tc.archive))
			var stdout bytes.Buffer

			ctx := cli.WithEnv(context.Background(), &cli.Env{
				Getenv: func(key string) string {
					if key == "CI" {
						return config.CI
					}
					return ""
				},
				Stdout: &stdout,
				Stderr: &bytes.Buffer{},
			})

			oldWD, err := os.Getwd()
			if err != nil {
				t.Fatalf("Getwd(): %v", err)
			}
			if err := os.Chdir(dir); err != nil {
				t.Fatalf("Chdir(%q): %v", dir, err)
			}
			t.Cleanup(func() {
				if err := os.Chdir(oldWD); err != nil {
					t.Fatalf("Chdir(%q): %v", oldWD, err)
				}
			})

			if err := realMain(ctx); err != nil {
				t.Fatalf("realMain(): %v", err)
			}

			if got := stdout.String(); got != config.WantStdout {
				t.Fatalf("stdout = %q, want %q", got, config.WantStdout)
			}

			if config.WantHook != "" {
				hookPath := filepath.Join(dir, ".git", "hooks", "pre-commit")
				hook, err := os.ReadFile(hookPath)
				if err != nil {
					t.Fatalf("ReadFile(%q): %v", hookPath, err)
				}
				if got := string(hook); got != config.WantHook {
					t.Fatalf("hook = %q, want %q", got, config.WantHook)
				}
			}
		})
	}
}

func extractRunCase(t *testing.T, path string) (string, runCase) {
	t.Helper()

	ar, err := txtar.ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile(%q): %v", path, err)
	}

	dir := t.TempDir()
	if err := txtar.Extract(ar, dir); err != nil {
		t.Fatalf("Extract(%q): %v", path, err)
	}

	preCommitJSONPath := filepath.Join(dir, "pre-commit.json")
	preCommitJSON, err := os.ReadFile(preCommitJSONPath)
	if err != nil {
		t.Fatalf("ReadFile(%q): %v", preCommitJSONPath, err)
	}
	if err := os.MkdirAll(filepath.Join(dir, ".devtools"), 0o755); err != nil {
		t.Fatalf("MkdirAll(.devtools): %v", err)
	}
	configPath := filepath.Join(dir, ".devtools", "config.txtar")
	configData := txtar.Format(&txtar.Archive{
		Files: []txtar.File{{Name: "pre-commit.json", Data: preCommitJSON}},
	})
	if err := os.WriteFile(configPath, configData, 0o644); err != nil {
		t.Fatalf("WriteFile(%q): %v", configPath, err)
	}

	var c runCase
	for _, f := range ar.Files {
		if f.Name != "case.json" {
			continue
		}
		if err := json.Unmarshal(f.Data, &c); err != nil {
			t.Fatalf("Unmarshal(%q): %v", path, err)
		}
		break
	}

	if c.WantStdout == "" {
		t.Fatalf("missing case.json.want_stdout in %q", path)
	}

	return dir, c
}
