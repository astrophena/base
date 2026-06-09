// © 2025 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the ISC
// license that can be found in the LICENSE.md file.

package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"

	"go.astrophena.name/base/cli"
	"go.astrophena.name/base/request"
	"go.astrophena.name/base/version"
)

const (
	defaultArtifactChunkBytes = 64 << 20
	maxArtifactChunkBytes     = 64 << 20

	artifactChunkUploadAttempts   = 4
	artifactChunkUploadRetryDelay = 5 * time.Second
)

var (
	artifactChunkUploadAttemptTimeout = 10 * time.Minute
	artifactChunkUploadStallTimeout   = 2 * time.Minute
	artifactChunkUploadSleep          = sleepArtifactChunkUploadRetry
)

var errArtifactChunkUploadStalled = errors.New("artifact chunk upload stalled")

func main() { cli.Main(new(app)) }

type tokenResponse struct {
	Value string `json:"value"`
}

type app struct {
	// configuration
	typ                   string // service, site, or artifact
	serverURL             string
	tokenAudience         string
	artifactChunkBytes    int64
	artifactReleaseID     string
	artifactSigningKeyEnv string
}

func (a *app) Flags(fs *flag.FlagSet) {
	fs.StringVar(&a.typ, "type", "site", "Whether to deploy `site, service, or artifact`.")
	fs.StringVar(&a.serverURL, "server-url", "https://deploy.astrophena.name", "The `URL` of the deployment server.")
	fs.StringVar(&a.tokenAudience, "token-audience", "astrophena.name", "The `audience` for the OIDC token.")
	fs.Int64Var(&a.artifactChunkBytes, "artifact-chunk-size", defaultArtifactChunkBytes, "Artifact upload chunk `bytes`.")
	fs.StringVar(&a.artifactReleaseID, "artifact-release-id", "", "Artifact release ID. Defaults to current UTC timestamp.")
	fs.StringVar(&a.artifactSigningKeyEnv, "artifact-signing-key-env", "DEPLOY_ARTIFACT_SIGNING_KEY", "Environment variable containing the Ed25519 artifact signing key.")
}

func (a *app) Run(ctx context.Context) error {
	a.serverURL = strings.TrimRight(a.serverURL, "/")
	switch a.typ {
	case "site", "service":
		return a.runArchiveDeployment(ctx)
	case "artifact":
		return a.runArtifactDeployment(ctx)
	default:
		return fmt.Errorf("%w: invalid type, want site, service, or artifact, got %q", cli.ErrInvalidArgs, a.typ)
	}
}

func (a *app) runArchiveDeployment(ctx context.Context) error {
	env := cli.GetEnv(ctx)
	if len(env.Args) != 2 {
		return fmt.Errorf("%w: want service or site and archive path", cli.ErrInvalidArgs)
	}
	target, archive := env.Args[0], env.Args[1]

	token, err := a.githubOIDCToken(ctx)
	if err != nil {
		return err
	}

	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer f.Close()

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, err := mw.CreateFormFile("archive", filepath.Base(archive))
	if err != nil {
		return err
	}
	if _, err := io.Copy(part, f); err != nil {
		return err
	}
	if err := mw.Close(); err != nil {
		return err
	}

	_, err = request.Make[request.IgnoreResponse](ctx, request.Params{
		Method: http.MethodPost,
		URL:    a.serverURL + "/" + a.typ + "/" + target,
		Body:   buf.Bytes(),
		Headers: map[string]string{
			"Content-Type":  mw.FormDataContentType(),
			"User-Agent":    version.UserAgent(),
			"Authorization": "Bearer " + token,
		},
	})
	return err
}

func (a *app) runArtifactDeployment(ctx context.Context) error {
	env := cli.GetEnv(ctx)
	if len(env.Args) < 2 {
		return fmt.Errorf("%w: want artifact target and one or more file paths", cli.ErrInvalidArgs)
	}
	if a.artifactChunkBytes <= 0 || a.artifactChunkBytes > maxArtifactChunkBytes {
		return fmt.Errorf("%w: artifact chunk size must be between 1 and %d bytes", cli.ErrInvalidArgs, maxArtifactChunkBytes)
	}
	target := env.Args[0]
	paths := env.Args[1:]

	token, err := a.githubOIDCToken(ctx)
	if err != nil {
		return err
	}
	privateKey, err := artifactSigningKey(env, a.artifactSigningKeyEnv)
	if err != nil {
		return err
	}

	releaseID := a.artifactReleaseID
	if releaseID == "" {
		releaseID = time.Now().UTC().Format("20060102150405")
	}
	manifest, localPaths, err := buildArtifactManifest(paths, releaseID, privateKey.Public().(ed25519.PublicKey), a.artifactChunkBytes, env)
	if err != nil {
		return err
	}
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		return err
	}
	signature := ed25519.Sign(privateKey, manifestBytes)

	client := &http.Client{Timeout: 30 * time.Minute}
	createResp, err := request.Make[artifactUploadResponse](ctx, request.Params{
		Method: http.MethodPost,
		URL:    artifactUploadURL(a.serverURL, target),
		Body: artifactCreateUploadRequest{
			ReleaseID:   releaseID,
			Manifest:    json.RawMessage(manifestBytes),
			ManifestSig: base64.StdEncoding.EncodeToString(signature),
		},
		Headers:    artifactAuthHeaders(token),
		HTTPClient: client,
	})
	if err != nil {
		return err
	}
	if createResp.UploadID == "" || createResp.UploadToken == "" {
		return errors.New("artifact upload response missing upload_id or upload_token")
	}

	present := presentChunkSet(createResp.Present)
	for _, file := range manifest.Files {
		if err := a.uploadArtifactFileChunks(ctx, client, createResp.UploadToken, target, createResp.UploadID, localPaths[file.Path], file, present[file.Path], env.Stderr); err != nil {
			return err
		}
	}

	_, err = request.Make[request.IgnoreResponse](ctx, request.Params{
		Method:     http.MethodPost,
		URL:        artifactCompleteURL(a.serverURL, target, createResp.UploadID),
		Headers:    artifactAuthHeaders(createResp.UploadToken),
		HTTPClient: client,
	})
	return err
}

func (a *app) githubOIDCToken(ctx context.Context) (string, error) {
	env := cli.GetEnv(ctx)
	requestURL := env.Getenv("ACTIONS_ID_TOKEN_REQUEST_URL")
	requestToken := env.Getenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN")
	if requestURL == "" || requestToken == "" {
		return "", errors.New("ACTIONS_ID_TOKEN_REQUEST_URL and ACTIONS_ID_TOKEN_REQUEST_TOKEN should be set")
	}

	tokenResp, err := request.Make[tokenResponse](ctx, request.Params{
		Method: http.MethodGet,
		URL:    requestURL + "&audience=" + url.QueryEscape(a.tokenAudience),
		Headers: map[string]string{
			"Authorization": "Bearer " + requestToken,
			"User-Agent":    version.UserAgent(),
		},
	})
	if err != nil {
		return "", err
	}
	return tokenResp.Value, nil
}

type artifactCreateUploadRequest struct {
	ReleaseID   string          `json:"release_id"`
	Manifest    json.RawMessage `json:"manifest"`
	ManifestSig string          `json:"manifest_sig"`
}

type artifactManifest struct {
	ReleaseID        string                 `json:"release_id"`
	Files            []artifactManifestFile `json:"files"`
	Build            map[string]string      `json:"build,omitempty"`
	PatchPaths       []string               `json:"patch_paths,omitempty"`
	SigningPublicKey string                 `json:"signing_public_key"`
}

type artifactManifestFile struct {
	Path   string                  `json:"path"`
	Size   int64                   `json:"size"`
	SHA256 string                  `json:"sha256"`
	Chunks []artifactManifestChunk `json:"chunks"`
}

type artifactManifestChunk struct {
	Index  int    `json:"index"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

type artifactUploadResponse struct {
	UploadID             string                       `json:"upload_id"`
	UploadToken          string                       `json:"upload_token"`
	UploadTokenExpiresAt time.Time                    `json:"upload_token_expires_at"`
	Present              map[string][]int             `json:"present_chunks"`
	Files                []artifactUploadResponseFile `json:"files"`
}

type artifactUploadResponseFile struct {
	Path   string `json:"path"`
	Chunks int    `json:"chunks"`
}

func buildArtifactManifest(paths []string, releaseID string, publicKey ed25519.PublicKey, chunkBytes int64, env *cli.Env) (artifactManifest, map[string]string, error) {
	manifest := artifactManifest{
		ReleaseID:        releaseID,
		Build:            artifactBuildMetadata(env),
		SigningPublicKey: base64.StdEncoding.EncodeToString(publicKey),
	}
	localPaths := make(map[string]string)
	seen := make(map[string]bool)
	for _, filePath := range paths {
		name := filepath.Base(filePath)
		if name == "." || name == string(filepath.Separator) || name == "" {
			return artifactManifest{}, nil, fmt.Errorf("invalid artifact file path %q", filePath)
		}
		if seen[name] {
			return artifactManifest{}, nil, fmt.Errorf("duplicate artifact file name %q", name)
		}
		seen[name] = true
		file, err := artifactManifestForFile(filePath, name, chunkBytes)
		if err != nil {
			return artifactManifest{}, nil, err
		}
		manifest.Files = append(manifest.Files, file)
		manifest.PatchPaths = append(manifest.PatchPaths, name)
		localPaths[name] = filePath
	}
	slices.SortFunc(manifest.Files, func(a, b artifactManifestFile) int { return strings.Compare(a.Path, b.Path) })
	slices.Sort(manifest.PatchPaths)
	return manifest, localPaths, nil
}

func artifactManifestForFile(filePath, name string, chunkBytes int64) (artifactManifestFile, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return artifactManifestFile{}, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return artifactManifestFile{}, err
	}
	if !info.Mode().IsRegular() {
		return artifactManifestFile{}, fmt.Errorf("artifact file %q is not a regular file", filePath)
	}
	if info.Size() == 0 {
		return artifactManifestFile{}, fmt.Errorf("artifact file %q is empty", filePath)
	}

	buf := make([]byte, chunkBytes)
	fullHash := sha256.New()
	file := artifactManifestFile{Path: name, Size: info.Size()}
	for index := 0; ; index++ {
		n, readErr := io.ReadFull(f, buf)
		if readErr != nil && !errors.Is(readErr, io.ErrUnexpectedEOF) && !errors.Is(readErr, io.EOF) {
			return artifactManifestFile{}, readErr
		}
		if n == 0 {
			break
		}
		chunkBytes := buf[:n]
		fullHash.Write(chunkBytes)
		chunkHash := sha256.Sum256(chunkBytes)
		file.Chunks = append(file.Chunks, artifactManifestChunk{
			Index:  index,
			Size:   int64(n),
			SHA256: hex.EncodeToString(chunkHash[:]),
		})
		if readErr != nil {
			break
		}
	}
	file.SHA256 = hex.EncodeToString(fullHash.Sum(nil))
	return file, nil
}

func (a *app) uploadArtifactFileChunks(ctx context.Context, client *http.Client, token, target, uploadID, localPath string, file artifactManifestFile, present map[int]bool, stderr io.Writer) error {
	f, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer f.Close()

	var offset int64
	for _, chunk := range file.Chunks {
		if present[chunk.Index] {
			offset += chunk.Size
			continue
		}
		data := make([]byte, int(chunk.Size))
		if _, err := f.ReadAt(data, offset); err != nil {
			return fmt.Errorf("reading chunk %d from %s: %w", chunk.Index, localPath, err)
		}
		if stderr != nil {
			fmt.Fprintf(stderr, "uploading %s chunk %d/%d\n", file.Path, chunk.Index+1, len(file.Chunks))
		}
		if err := a.uploadArtifactChunk(ctx, client, token, target, uploadID, file.Path, chunk, data, stderr); err != nil {
			return err
		}
		offset += chunk.Size
	}
	return nil
}

func (a *app) uploadArtifactChunk(ctx context.Context, client *http.Client, token, target, uploadID, filePath string, chunk artifactManifestChunk, data []byte, stderr io.Writer) error {
	chunkURL := artifactChunkURL(a.serverURL, target, uploadID, filePath, chunk.Index)
	var lastErr error
	for attempt := 1; attempt <= artifactChunkUploadAttempts; attempt++ {
		attemptCtx, cancel := context.WithTimeout(ctx, artifactChunkUploadAttemptTimeout)
		err := uploadArtifactChunkAttempt(attemptCtx, client, token, chunkURL, chunk, data)
		cancel()
		if err == nil {
			return nil
		}
		lastErr = err
		if !isTemporaryArtifactUploadError(err) || attempt == artifactChunkUploadAttempts {
			return err
		}
		if stderr != nil {
			fmt.Fprintf(stderr, "retrying %s chunk %d after upload error: %v\n", filePath, chunk.Index+1, err)
		}
		if !artifactChunkUploadSleep(ctx, artifactChunkUploadRetryDelay) {
			return err
		}
	}
	return lastErr
}

func uploadArtifactChunkAttempt(ctx context.Context, client *http.Client, token, chunkURL string, chunk artifactManifestChunk, data []byte) error {
	uploadCtx, cancelUpload := context.WithCancelCause(ctx)
	defer cancelUpload(nil)

	progress := make(chan struct{}, 1)
	done := make(chan struct{})
	var doneOnce sync.Once
	finishUpload := func() { doneOnce.Do(func() { close(done) }) }
	stallTimeout := artifactChunkUploadStallTimeout
	go monitorArtifactChunkUploadProgress(uploadCtx, cancelUpload, stallTimeout, progress, done)
	defer finishUpload()

	newBody := func() io.ReadCloser {
		return io.NopCloser(&artifactUploadProgressReader{
			r: bytes.NewReader(data),
			onProgress: func() {
				if usesNetworkProgress(client) {
					return
				}
				select {
				case progress <- struct{}{}:
				default:
				}
			},
			onDone: finishUpload,
		})
	}
	req, err := http.NewRequestWithContext(uploadCtx, http.MethodPut, chunkURL, newBody())
	if err != nil {
		return err
	}
	req.ContentLength = int64(len(data))
	req.GetBody = func() (io.ReadCloser, error) { return newBody(), nil }
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("User-Agent", version.UserAgent())
	req.Header.Set("X-Chunk-SHA256", chunk.SHA256)

	attemptClient := *client
	useNetworkProgress(&attemptClient, func() {
		select {
		case progress <- struct{}{}:
		default:
		}
	})
	res, err := attemptClient.Do(req)
	if err != nil {
		if errors.Is(context.Cause(uploadCtx), errArtifactChunkUploadStalled) {
			return errArtifactChunkUploadStalled
		}
		return err
	}
	defer res.Body.Close()

	b, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("%s %q: %w", http.MethodPut, chunkURL, &request.StatusError{
			WantedStatusCode: http.StatusOK,
			StatusCode:       res.StatusCode,
			Headers:          res.Header,
			Body:             b,
		})
	}
	return nil
}

func usesNetworkProgress(client *http.Client) bool {
	if client.Transport == nil {
		return true
	}
	_, ok := client.Transport.(*http.Transport)
	return ok
}

func useNetworkProgress(client *http.Client, onProgress func()) {
	base, ok := client.Transport.(*http.Transport)
	if client.Transport == nil {
		base, _ = http.DefaultTransport.(*http.Transport)
		ok = base != nil
	}
	if !ok {
		return
	}

	t := base.Clone()
	dialContext := t.DialContext
	if dialContext == nil {
		var d net.Dialer
		dialContext = d.DialContext
	}
	t.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		conn, err := dialContext(ctx, network, address)
		if err != nil {
			return nil, err
		}
		return artifactUploadProgressConn{Conn: conn, onProgress: onProgress}, nil
	}
	if t.DialTLSContext != nil {
		dialTLSContext := t.DialTLSContext
		t.DialTLSContext = func(ctx context.Context, network, address string) (net.Conn, error) {
			conn, err := dialTLSContext(ctx, network, address)
			if err != nil {
				return nil, err
			}
			return artifactUploadProgressConn{Conn: conn, onProgress: onProgress}, nil
		}
	}
	client.Transport = t
}

type artifactUploadProgressConn struct {
	net.Conn
	onProgress func()
}

func (c artifactUploadProgressConn) Write(p []byte) (int, error) {
	n, err := c.Conn.Write(p)
	if n > 0 {
		c.onProgress()
	}
	return n, err
}

func monitorArtifactChunkUploadProgress(ctx context.Context, cancel context.CancelCauseFunc, stallTimeout time.Duration, progress <-chan struct{}, done <-chan struct{}) {
	timer := time.NewTimer(stallTimeout)
	defer timer.Stop()
	for {
		select {
		case <-timer.C:
			cancel(errArtifactChunkUploadStalled)
			return
		case <-progress:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(stallTimeout)
		case <-done:
			return
		case <-ctx.Done():
			return
		}
	}
}

type artifactUploadProgressReader struct {
	r          *bytes.Reader
	onProgress func()
	onDone     func()
	done       bool
}

func (r *artifactUploadProgressReader) Read(p []byte) (int, error) {
	n, err := r.r.Read(p)
	if n > 0 {
		r.onProgress()
	}
	if (r.r.Len() == 0 || err == io.EOF) && !r.done {
		r.done = true
		r.onDone()
	}
	return n, err
}

func sleepArtifactChunkUploadRetry(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	}
}

func isTemporaryArtifactUploadError(err error) bool {
	if errors.Is(err, errArtifactChunkUploadStalled) {
		return true
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	for _, target := range []error{
		io.EOF,
		io.ErrUnexpectedEOF,
		syscall.ECONNABORTED,
		syscall.ECONNREFUSED,
		syscall.ECONNRESET,
		syscall.EPIPE,
		syscall.ETIMEDOUT,
	} {
		if errors.Is(err, target) {
			return true
		}
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	var statusErr *request.StatusError
	if errors.As(err, &statusErr) {
		switch statusErr.StatusCode {
		case http.StatusRequestTimeout, http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
			return true
		}
	}
	return false
}

func artifactBuildMetadata(env *cli.Env) map[string]string {
	keys := map[string]string{
		"repository":   "GITHUB_REPOSITORY",
		"ref":          "GITHUB_REF",
		"sha":          "GITHUB_SHA",
		"actor":        "GITHUB_ACTOR",
		"workflow":     "GITHUB_WORKFLOW",
		"workflow_ref": "GITHUB_WORKFLOW_REF",
		"run_id":       "GITHUB_RUN_ID",
		"run_attempt":  "GITHUB_RUN_ATTEMPT",
	}
	build := make(map[string]string)
	for name, envName := range keys {
		if value := env.Getenv(envName); value != "" {
			build[name] = value
		}
	}
	if len(build) == 0 {
		return nil
	}
	return build
}

func artifactSigningKey(env *cli.Env, envName string) (ed25519.PrivateKey, error) {
	text := strings.TrimSpace(env.Getenv(envName))
	if text == "" {
		return nil, fmt.Errorf("%s should contain an Ed25519 private key", envName)
	}
	if block, _ := pem.Decode([]byte(text)); block != nil {
		key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("parse %s PEM private key: %w", envName, err)
		}
		privateKey, ok := key.(ed25519.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("%s PEM private key is %T, want Ed25519", envName, key)
		}
		return privateKey, nil
	}

	decoded, err := decodeArtifactSigningKey(text)
	if err != nil {
		return nil, fmt.Errorf("decode %s: %w", envName, err)
	}
	switch len(decoded) {
	case ed25519.SeedSize:
		return ed25519.NewKeyFromSeed(decoded), nil
	case ed25519.PrivateKeySize:
		return ed25519.PrivateKey(decoded), nil
	default:
		return nil, fmt.Errorf("%s decoded to %d bytes, want %d-byte seed or %d-byte private key", envName, len(decoded), ed25519.SeedSize, ed25519.PrivateKeySize)
	}
}

func decodeArtifactSigningKey(text string) ([]byte, error) {
	if len(text) == ed25519.SeedSize*2 || len(text) == ed25519.PrivateKeySize*2 {
		if decoded, err := hex.DecodeString(text); err == nil {
			return decoded, nil
		}
	}
	text = strings.TrimPrefix(text, "base64:")
	if decoded, err := base64.StdEncoding.DecodeString(text); err == nil {
		return decoded, nil
	}
	if decoded, err := base64.RawStdEncoding.DecodeString(text); err == nil {
		return decoded, nil
	}
	return hex.DecodeString(text)
}

func presentChunkSet(present map[string][]int) map[string]map[int]bool {
	sets := make(map[string]map[int]bool)
	for file, chunks := range present {
		set := make(map[int]bool)
		for _, index := range chunks {
			set[index] = true
		}
		sets[file] = set
	}
	return sets
}

func artifactAuthHeaders(token string) map[string]string {
	return map[string]string{
		"Authorization": "Bearer " + token,
		"User-Agent":    version.UserAgent(),
	}
}

func artifactUploadURL(serverURL, target string) string {
	return serverURL + "/artifact/" + url.PathEscape(target) + "/uploads"
}

func artifactChunkURL(serverURL, target, uploadID, fileName string, index int) string {
	return serverURL + "/artifact/" + url.PathEscape(target) + "/uploads/" + url.PathEscape(uploadID) + "/files/" + url.PathEscape(fileName) + "/chunks/" + fmt.Sprint(index)
}

func artifactCompleteURL(serverURL, target, uploadID string) string {
	return serverURL + "/artifact/" + url.PathEscape(target) + "/uploads/" + url.PathEscape(uploadID) + "/complete"
}
