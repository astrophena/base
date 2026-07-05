// © 2026 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the ISC
// license that can be found in the LICENSE.md file.

package cdc

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"go.astrophena.name/base/testutil"
)

func TestWalkChunksSmallFile(t *testing.T) {
	data := []byte("hello deployd")
	var chunks [][]byte
	file, err := WalkChunks(context.Background(), bytes.NewReader(data), func(chunk DataChunk) error {
		chunks = append(chunks, append([]byte(nil), chunk.Data...))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	testutil.AssertEqual(t, file.Size, int64(len(data)))
	testutil.AssertEqual(t, file.SHA256, SHA256HexBytes(data))
	testutil.AssertEqual(t, file.Chunks, []ManifestChunk{{Index: 0, Size: int64(len(data)), SHA256: SHA256HexBytes(data)}})
	testutil.AssertEqual(t, chunks, [][]byte{data})
}

func TestDefaultChunkingValidates(t *testing.T) {
	if err := ValidateChunking(DefaultChunking()); err != nil {
		t.Fatal(err)
	}

	bad := DefaultChunking()
	bad.Algorithm = "other"
	if err := ValidateChunking(bad); err == nil {
		t.Fatal("ValidateChunking accepted unsupported algorithm")
	}
}

func TestNewIndexAndValidateIndex(t *testing.T) {
	data := []byte("artifact bytes")
	file, err := WalkChunks(context.Background(), bytes.NewReader(data), nil)
	if err != nil {
		t.Fatal(err)
	}

	index := NewIndex("rootfs.erofs", file)
	if err := ValidateIndex(index, "rootfs.erofs"); err != nil {
		t.Fatal(err)
	}
	testutil.AssertEqual(t, index.Version, Version)
	testutil.AssertEqual(t, index.Algorithm, Algorithm)
	testutil.AssertEqual(t, index.ChunkEncoding, ChunkEncodingRaw)
	testutil.AssertEqual(t, index.Source.SHA256, SHA256HexBytes(data))

	index.Chunks[0].Offset = 1
	if err := ValidateIndex(index, "rootfs.erofs"); err == nil {
		t.Fatal("ValidateIndex accepted non-contiguous chunks")
	}
}

func TestDigestHelpers(t *testing.T) {
	hash := strings.Repeat("a", 64)
	if !IsSHA256Hex(hash) {
		t.Fatalf("IsSHA256Hex(%q) = false", hash)
	}
	if IsSHA256Hex("not-a-sha") {
		t.Fatal("IsSHA256Hex accepted invalid digest")
	}
	testutil.AssertEqual(t, NormalizeSHA256(strings.ToUpper(hash)), hash)
}
