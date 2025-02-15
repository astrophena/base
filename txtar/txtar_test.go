// © 2024 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the ISC
// license that can be found in the LICENSE.md file.

package txtar

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestParse(t *testing.T) {
	cases := map[string]struct {
		in   []byte
		want *Archive
	}{
		"empty": {
			in: []byte{},
			want: &Archive{
				Comment: []byte{},
				Files:   []File{},
			},
		},
		"comment only": {
			in: []byte("# comment\n"),
			want: &Archive{
				Comment: []byte("# comment\n"),
				Files:   []File{},
			},
		},
		"one file": {
			in: []byte("-- foo.txt --\ncontent\n"),
			want: &Archive{
				Comment: []byte{},
				Files: []File{
					{Name: "foo.txt", Data: []byte("content\n")},
				},
			},
		},
		"two files": {
			in: []byte("-- foo.txt --\ncontent1\n-- bar.go --\ncontent2\n"),
			want: &Archive{
				Comment: []byte{},
				Files: []File{
					{Name: "foo.txt", Data: []byte("content1\n")},
					{Name: "bar.go", Data: []byte("content2\n")},
				},
			},
		},
		"comment and two files": {
			in: []byte("# comment\n-- foo.txt --\ncontent1\n-- bar.go --\ncontent2\n"),
			want: &Archive{
				Comment: []byte("# comment\n"),
				Files: []File{
					{Name: "foo.txt", Data: []byte("content1\n")},
					{Name: "bar.go", Data: []byte("content2\n")},
				},
			},
		},
		"file with no content": {
			in: []byte("-- foo.txt --\n-- bar.go --\ncontent\n"),
			want: &Archive{
				Comment: []byte{},
				Files: []File{
					{Name: "foo.txt", Data: []byte{}},
					{Name: "bar.go", Data: []byte("content\n")},
				},
			},
		},
		"file with whitespace around name": {
			in: []byte("--  foo.txt  --\ncontent\n"),
			want: &Archive{
				Comment: []byte{},
				Files: []File{
					{Name: "foo.txt", Data: []byte("content\n")},
				},
			},
		},
		"missing newline at end of file": {
			in: []byte("-- foo.txt --\ncontent"),
			want: &Archive{
				Comment: []byte{},
				Files: []File{
					{Name: "foo.txt", Data: []byte("content\n")},
				},
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := Parse(tc.in)
			if !equal(got, tc.want) {
				t.Errorf("Parse(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestFormat(t *testing.T) {
	cases := map[string]struct {
		in   *Archive
		want []byte
	}{
		"empty": {
			in:   &Archive{},
			want: []byte{},
		},
		"comment only": {
			in:   &Archive{Comment: []byte("# comment\n")},
			want: []byte("# comment\n"),
		},
		"one file": {
			in: &Archive{
				Files: []File{
					{Name: "foo.txt", Data: []byte("content\n")},
				},
			},
			want: []byte("-- foo.txt --\ncontent\n"),
		},
		"two files": {
			in: &Archive{
				Files: []File{
					{Name: "foo.txt", Data: []byte("content1\n")},
					{Name: "bar.go", Data: []byte("content2\n")},
				},
			},
			want: []byte("-- foo.txt --\ncontent1\n-- bar.go --\ncontent2\n"),
		},
		"comment and two files": {
			in: &Archive{
				Comment: []byte("# comment\n"),
				Files: []File{
					{Name: "foo.txt", Data: []byte("content1\n")},
					{Name: "bar.go", Data: []byte("content2\n")},
				},
			},
			want: []byte("# comment\n-- foo.txt --\ncontent1\n-- bar.go --\ncontent2\n"),
		},
		"file with no content": {
			in: &Archive{
				Files: []File{
					{Name: "foo.txt", Data: []byte{}},
					{Name: "bar.go", Data: []byte("content\n")},
				},
			},
			want: []byte("-- foo.txt --\n-- bar.go --\ncontent\n"),
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := Format(tc.in)
			if !bytes.Equal(got, tc.want) {
				t.Errorf("Format(%v) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func equal(a, b *Archive) bool {
	if !bytes.Equal(a.Comment, b.Comment) {
		return false
	}
	if len(a.Files) != len(b.Files) {
		return false
	}
	for i := range a.Files {
		if a.Files[i].Name != b.Files[i].Name || !bytes.Equal(a.Files[i].Data, b.Files[i].Data) {
			return false
		}
	}
	return true
}

func TestExtract(t *testing.T) {
	tempDir := t.TempDir()

	a := &Archive{
		Comment: []byte("# Test archive\n"),
		Files: []File{
			{Name: "file1.txt", Data: []byte("Content of file1\n")},
			{Name: "subdir/file2.txt", Data: []byte("Content of file2\n")},
		},
	}

	err := Extract(a, tempDir)
	if err != nil {
		t.Fatalf("Extract failed: %v", err)
	}

	verifyFile(t, filepath.Join(tempDir, "file1.txt"), "Content of file1\n")
	verifyFile(t, filepath.Join(tempDir, "subdir", "file2.txt"), "Content of file2\n")
}

func TestFromDir(t *testing.T) {
	tempDir := t.TempDir()
	createFile(t, filepath.Join(tempDir, "file1.txt"), "Content of file1\n")
	createFile(t, filepath.Join(tempDir, "file2.txt"), "Content of file2\n")

	a, err := FromDir(tempDir)
	if err != nil {
		t.Fatalf("FromDir failed: %v", err)
	}

	want := &Archive{
		Files: []File{
			{Name: "file1.txt", Data: []byte("Content of file1\n")},
			{Name: "file2.txt", Data: []byte("Content of file2\n")},
		},
	}

	if len(a.Files) != len(want.Files) {
		t.Fatalf("Incorrect number of files in archive.\nGot: %d, Want: %d", len(a.Files), len(want.Files))
	}

	// Files can be in different order, so need to iterate.
	for _, wf := range want.Files {
		found := false
		for _, gf := range a.Files {
			if wf.Name == gf.Name && bytes.Equal(wf.Data, gf.Data) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("File %s not found or content mismatch.", wf.Name)
		}
	}
}

func verifyFile(t *testing.T, path, wantContent string) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read file %s: %v", path, err)
	}
	if string(content) != wantContent {
		t.Errorf("File content mismatch for %s.\nGot: %q, Want: %q", path, content, wantContent)
	}
}

func createFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}
}
