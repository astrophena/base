// © 2026 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the ISC
// license that can be found in the LICENSE.md file.

package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

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

func TestRunArtifactDeploymentUsesUploadToken(t *testing.T) {
	dir := t.TempDir()
	artifactPath := filepath.Join(dir, "rootfs.erofs")
	if err := os.WriteFile(artifactPath, []byte("artifact bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}

	var uploadBody artifactCreateUploadRequest
	var chunkCount int
	var completed bool
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/oidc":
			if got, want := r.Header.Get("Authorization"), "Bearer request-token"; got != want {
				t.Errorf("OIDC Authorization = %q, want %q", got, want)
			}
			if got, want := r.URL.Query().Get("audience"), "aud"; got != want {
				t.Errorf("audience = %q, want %q", got, want)
			}
			json.NewEncoder(w).Encode(tokenResponse{Value: "oidc-token"})

		case r.URL.Path == "/artifact/dungeon/uploads":
			if got, want := r.Method, http.MethodPost; got != want {
				t.Errorf("create method = %q, want %q", got, want)
			}
			if got, want := r.Header.Get("Authorization"), "Bearer oidc-token"; got != want {
				t.Errorf("create Authorization = %q, want %q", got, want)
			}
			if err := json.NewDecoder(r.Body).Decode(&uploadBody); err != nil {
				t.Errorf("decode upload body: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{
				"upload_id":"upload-1",
				"upload_token":"upload-token",
				"upload_token_expires_at":"`+time.Now().UTC().Add(time.Hour).Format(time.RFC3339)+`",
				"present_chunks":{},
				"files":[{"path":"rootfs.erofs","chunks":4}]
			}`)

		case r.URL.Path == "/artifact/dungeon/uploads/upload-1/files/rootfs.erofs/chunks/"+strconv.Itoa(chunkCount):
			if got, want := r.Method, http.MethodPut; got != want {
				t.Errorf("chunk method = %q, want %q", got, want)
			}
			if got, want := r.Header.Get("Authorization"), "Bearer upload-token"; got != want {
				t.Errorf("chunk Authorization = %q, want %q", got, want)
			}
			if got, want := r.Header.Get("Content-Type"), "application/octet-stream"; got != want {
				t.Errorf("chunk Content-Type = %q, want %q", got, want)
			}
			if r.Header.Get("X-Chunk-SHA256") == "" {
				t.Error("missing X-Chunk-SHA256")
			}
			chunkCount++
			json.NewEncoder(w).Encode(map[string]string{"status": "success"})

		case r.URL.Path == "/artifact/dungeon/uploads/upload-1/complete":
			if got, want := r.Method, http.MethodPost; got != want {
				t.Errorf("complete method = %q, want %q", got, want)
			}
			if got, want := r.Header.Get("Authorization"), "Bearer upload-token"; got != want {
				t.Errorf("complete Authorization = %q, want %q", got, want)
			}
			completed = true
			json.NewEncoder(w).Encode(map[string]string{"status": "success"})

		default:
			http.NotFound(w, r)
		}
	})
	oldTransport := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, r)
		return rec.Result(), nil
	})
	t.Cleanup(func() { http.DefaultTransport = oldTransport })

	env := &cli.Env{
		Args:   []string{"dungeon", artifactPath},
		Stderr: io.Discard,
		Getenv: func(name string) string {
			switch name {
			case "ACTIONS_ID_TOKEN_REQUEST_URL":
				return "https://actions.test/oidc?unused=1"
			case "ACTIONS_ID_TOKEN_REQUEST_TOKEN":
				return "request-token"
			case "KEY":
				return base64.StdEncoding.EncodeToString(privateKey.Seed())
			default:
				return ""
			}
		},
	}
	a := &app{
		serverURL:             "https://deploy.test",
		tokenAudience:         "aud",
		artifactChunkBytes:    4,
		artifactReleaseID:     "20260608120000",
		artifactSigningKeyEnv: "KEY",
	}
	if err := a.runArtifactDeployment(cli.WithEnv(context.Background(), env)); err != nil {
		t.Fatal(err)
	}
	if uploadBody.ReleaseID != "20260608120000" {
		t.Fatalf("ReleaseID = %q", uploadBody.ReleaseID)
	}
	if !bytes.Equal(uploadBody.Manifest, mustMarshalManifest(t, "20260608120000", artifactPath, privateKey.Public().(ed25519.PublicKey))) {
		t.Fatal("manifest bytes were not sent byte-for-byte")
	}
	if chunkCount != 4 {
		t.Fatalf("uploaded chunks = %d, want 4", chunkCount)
	}
	if !completed {
		t.Fatal("upload was not completed")
	}
}

func TestUploadArtifactFileChunksRetriesTemporaryNetworkError(t *testing.T) {
	dir := t.TempDir()
	artifactPath := filepath.Join(dir, "rootfs.erofs")
	if err := os.WriteFile(artifactPath, []byte("artifact"), 0o644); err != nil {
		t.Fatal(err)
	}
	defer replaceArtifactChunkUploadSleep(func(context.Context, time.Duration) bool { return true })()

	var attempts int
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		attempts++
		if attempts == 1 {
			return nil, temporaryNetError{}
		}
		return jsonResponse(t, http.StatusOK, `{"status":"success"}`), nil
	})}

	a := &app{serverURL: "https://deploy.test"}
	file := artifactManifestFile{
		Path: "rootfs.erofs",
		Chunks: []artifactManifestChunk{{
			Index:  0,
			Size:   int64(len("artifact")),
			SHA256: "sha",
		}},
	}
	if err := a.uploadArtifactFileChunks(context.Background(), client, "token", "dungeon", "upload-1", artifactPath, file, nil, io.Discard); err != nil {
		t.Fatal(err)
	}
	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2", attempts)
	}
}

func TestUploadArtifactFileChunksRetriesStalledUpload(t *testing.T) {
	dir := t.TempDir()
	artifactPath := filepath.Join(dir, "rootfs.erofs")
	if err := os.WriteFile(artifactPath, []byte("artifact"), 0o644); err != nil {
		t.Fatal(err)
	}
	defer replaceArtifactChunkUploadSleep(func(context.Context, time.Duration) bool { return true })()
	defer replaceArtifactChunkUploadStallTimeout(time.Millisecond)()

	var attempts int
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		attempts++
		if attempts == 1 {
			var buf [1]byte
			if _, err := r.Body.Read(buf[:]); err != nil {
				t.Errorf("read first upload byte: %v", err)
			}
			<-r.Context().Done()
			return nil, r.Context().Err()
		}
		return jsonResponse(t, http.StatusOK, `{"status":"success"}`), nil
	})}

	a := &app{serverURL: "https://deploy.test"}
	file := artifactManifestFile{
		Path: "rootfs.erofs",
		Chunks: []artifactManifestChunk{{
			Index:  0,
			Size:   int64(len("artifact")),
			SHA256: "sha",
		}},
	}
	if err := a.uploadArtifactFileChunks(context.Background(), client, "token", "dungeon", "upload-1", artifactPath, file, nil, io.Discard); err != nil {
		t.Fatal(err)
	}
	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2", attempts)
	}
}

func mustMarshalManifest(t *testing.T, releaseID, path string, publicKey ed25519.PublicKey) []byte {
	t.Helper()
	manifest, _, err := buildArtifactManifest([]string{path}, releaseID, publicKey, 4, &cli.Env{
		Getenv: func(string) string { return "" },
	})
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

type temporaryNetError struct{}

func (temporaryNetError) Error() string   { return "temporary network error" }
func (temporaryNetError) Timeout() bool   { return true }
func (temporaryNetError) Temporary() bool { return true }

func jsonResponse(t *testing.T, status int, body string) *http.Response {
	t.Helper()
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewBufferString(body)),
	}
}

func replaceArtifactChunkUploadSleep(fn func(context.Context, time.Duration) bool) func() {
	old := artifactChunkUploadSleep
	artifactChunkUploadSleep = fn
	return func() { artifactChunkUploadSleep = old }
}

func replaceArtifactChunkUploadStallTimeout(d time.Duration) func() {
	old := artifactChunkUploadStallTimeout
	artifactChunkUploadStallTimeout = d
	return func() { artifactChunkUploadStallTimeout = old }
}
