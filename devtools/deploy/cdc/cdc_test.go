// © 2026 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the ISC
// license that can be found in the LICENSE.md file.

package cdc

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	"go.astrophena.name/base/testutil"
)

func TestSplitSmallFile(t *testing.T) {
	data := []byte("hello deployd")
	sum := sha256.Sum256(data)
	wantHash := hex.EncodeToString(sum[:])
	file, err := Split(context.Background(), bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}

	testutil.AssertEqual(t, file.Size, int64(len(data)))
	testutil.AssertEqual(t, file.SHA256, wantHash)
	testutil.AssertEqual(t, file.Chunks, []Chunk{{Index: 0, Size: int64(len(data)), SHA256: wantHash}})
}

func TestDefaultChunkingValidates(t *testing.T) {
	if err := DefaultChunking().Validate(); err != nil {
		t.Fatal(err)
	}

	bad := DefaultChunking()
	bad.Algorithm = "other"
	if err := bad.Validate(); err == nil {
		t.Fatal("Validate accepted unsupported algorithm")
	}
}

func TestIndexValidate(t *testing.T) {
	data := []byte("artifact bytes")
	sum := sha256.Sum256(data)
	file, err := Split(context.Background(), bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	file.SHA256 = strings.ToUpper(file.SHA256)
	file.Chunks[0].SHA256 = strings.ToUpper(file.Chunks[0].SHA256)

	index := NewIndex("rootfs.erofs", file)
	if err := index.Validate("rootfs.erofs"); err != nil {
		t.Fatal(err)
	}
	testutil.AssertEqual(t, index.Version, Version)
	testutil.AssertEqual(t, index.Algorithm, Algorithm)
	testutil.AssertEqual(t, index.ChunkEncoding, ChunkEncodingRaw)
	testutil.AssertEqual(t, index.Source.SHA256, hex.EncodeToString(sum[:]))
	testutil.AssertEqual(t, index.Chunks[0].SHA256, hex.EncodeToString(sum[:]))

	index.Chunks[0].Offset = 1
	if err := index.Validate("rootfs.erofs"); err == nil {
		t.Fatal("Validate accepted non-contiguous chunks")
	}
}

func TestDigestHelpers(t *testing.T) {
	hash := strings.Repeat("a", 64)
	if !ValidDigest(hash) {
		t.Fatalf("ValidDigest(%q) = false", hash)
	}
	if ValidDigest("not-a-sha") {
		t.Fatal("ValidDigest accepted invalid digest")
	}
	if !ValidDigest(strings.ToUpper(hash)) {
		t.Fatal("ValidDigest rejected uppercase hex")
	}
}
