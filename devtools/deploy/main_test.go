// © 2026 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the ISC
// license that can be found in the LICENSE.md file.

package main

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"

	"go.astrophena.name/base/cli"
)

func TestBuildArtifactManifest(t *testing.T) {
	dir := t.TempDir()
	kernel := filepath.Join(dir, "kernel")
	rootfs := filepath.Join(dir, "rootfs.erofs")
	if err := os.WriteFile(kernel, []byte("kernel bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(rootfs, []byte("rootfs bytes"), 0o644); err != nil {
		t.Fatal(err)
	}

	publicKey, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	env := &cli.Env{Getenv: func(name string) string {
		if name == "GITHUB_SHA" {
			return "deadbeef"
		}
		return ""
	}}
	manifest, localPaths, err := buildArtifactManifest([]string{rootfs, kernel}, "20260608120000", publicKey, 6, env)
	if err != nil {
		t.Fatal(err)
	}

	if manifest.ReleaseID != "20260608120000" {
		t.Fatalf("ReleaseID = %q", manifest.ReleaseID)
	}
	if manifest.Build["sha"] != "deadbeef" {
		t.Fatalf("Build[sha] = %q", manifest.Build["sha"])
	}
	if got, want := manifest.SigningPublicKey, base64.StdEncoding.EncodeToString(publicKey); got != want {
		t.Fatalf("SigningPublicKey = %q, want %q", got, want)
	}
	if got, want := len(manifest.Files), 2; got != want {
		t.Fatalf("len(Files) = %d, want %d", got, want)
	}
	if manifest.Files[0].Path != "kernel" || manifest.Files[1].Path != "rootfs.erofs" {
		t.Fatalf("files not sorted by path: %#v", manifest.Files)
	}
	if got, want := len(manifest.Files[0].Chunks), 2; got != want {
		t.Fatalf("kernel chunks = %d, want %d", got, want)
	}
	if got, want := manifest.Files[0].Chunks[0].Size, int64(6); got != want {
		t.Fatalf("first kernel chunk size = %d, want %d", got, want)
	}
	if got, want := localPaths["kernel"], kernel; got != want {
		t.Fatalf("localPaths[kernel] = %q, want %q", got, want)
	}
}

func TestArtifactSigningKey(t *testing.T) {
	_, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	seed := privateKey.Seed()
	pkcs8, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	pemKey := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8}))

	cases := map[string]string{
		"base64 seed":        base64.StdEncoding.EncodeToString(seed),
		"base64 private key": base64.StdEncoding.EncodeToString(privateKey),
		"hex seed":           hex.EncodeToString(seed),
		"PEM PKCS8":          pemKey,
	}
	for name, value := range cases {
		t.Run(name, func(t *testing.T) {
			env := &cli.Env{Getenv: func(got string) string {
				if got == "KEY" {
					return value
				}
				return ""
			}}
			got, err := artifactSigningKey(env, "KEY")
			if err != nil {
				t.Fatal(err)
			}
			if !got.Equal(privateKey) {
				t.Fatal("parsed key does not match private key")
			}
		})
	}
}

func TestPresentChunkSet(t *testing.T) {
	present := presentChunkSet(map[string][]int{"rootfs.erofs": {0, 2}})
	if !present["rootfs.erofs"][0] || present["rootfs.erofs"][1] || !present["rootfs.erofs"][2] {
		t.Fatalf("unexpected present set: %#v", present)
	}
}
