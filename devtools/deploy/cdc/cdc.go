// © 2026 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the ISC
// license that can be found in the LICENSE.md file.

// Package cdc splits deployd artifacts into reusable chunks and describes them
// in signed manifests and sidecar indexes.
package cdc

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/restic/chunker"
)

const (
	// Version is the sidecar index version.
	Version = 1
	// Algorithm names the chunking format.
	Algorithm = "deployd-cdc-v1"
	// ChunkEncodingRaw means chunks contain the original bytes.
	ChunkEncodingRaw = "raw"
	// IndexSuffix is appended to artifact file names for sidecar indexes.
	IndexSuffix = ".cdc.json"

	// MinSize is the smallest content-defined chunk size.
	MinSize = 512 << 10
	// AvgBits controls the statistical target chunk size: 2^AvgBits bytes.
	AvgBits = 20
	// MaxSize is the forced split size for data without natural CDC boundaries.
	MaxSize = 8 << 20
)

// Polynomial is the Rabin polynomial used to split files.
const Polynomial = chunker.Pol(0x2feedfaceb00b5)

// Chunking records the parameters signed in manifests and stored in indexes.
type Chunking struct {
	Version       int    `json:"version"`
	Algorithm     string `json:"algorithm"`
	ChunkEncoding string `json:"chunk_encoding"`
	Polynomial    string `json:"polynomial"`
	MinSize       uint   `json:"min_size"`
	AvgBits       int    `json:"avg_bits"`
	MaxSize       uint   `json:"max_size"`
}

// DefaultChunking returns the supported chunking parameters.
func DefaultChunking() Chunking {
	return Chunking{
		Version:       Version,
		Algorithm:     Algorithm,
		ChunkEncoding: ChunkEncodingRaw,
		Polynomial:    Polynomial.String(),
		MinSize:       MinSize,
		AvgBits:       AvgBits,
		MaxSize:       MaxSize,
	}
}

// Validate reports whether c matches the supported chunking parameters.
func (c Chunking) Validate() error {
	if c != DefaultChunking() {
		return errors.New("unsupported chunking parameters")
	}
	return nil
}

// Chunk describes one chunk in a signed manifest.
type Chunk struct {
	Index  int    `json:"index"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

// File describes the chunks and digest of a file.
type File struct {
	Size   int64
	SHA256 string
	Chunks []Chunk
}

// Split reads r and returns its chunk sizes and SHA-256 digests.
func Split(ctx context.Context, r io.Reader) (File, error) {
	hash := sha256.New()
	c := chunker.New(
		io.TeeReader(r, hash),
		Polynomial,
		chunker.WithBoundaries(MinSize, MaxSize),
		chunker.WithAverageBits(AvgBits),
	)

	buf := make([]byte, 0, MaxSize)
	var file File
	for index := 0; ; index++ {
		select {
		case <-ctx.Done():
			return File{}, ctx.Err()
		default:
		}

		chunk, err := c.Next(buf)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return File{}, err
		}

		sum := sha256.Sum256(chunk.Data)
		size := int64(len(chunk.Data))
		file.Chunks = append(file.Chunks, Chunk{
			Index:  index,
			Size:   size,
			SHA256: hex.EncodeToString(sum[:]),
		})
		file.Size += size
		buf = chunk.Data[:0]
	}
	file.SHA256 = hex.EncodeToString(hash.Sum(nil))
	return file, nil
}

// Source describes the file reconstructed from an index.
type Source struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

// Index describes a file as a sequence of raw chunks in a JSON sidecar.
type Index struct {
	Version       int          `json:"version"`
	Algorithm     string       `json:"algorithm"`
	ChunkEncoding string       `json:"chunk_encoding"`
	Source        Source       `json:"source"`
	Chunking      Chunking     `json:"chunking"`
	Chunks        []IndexChunk `json:"chunks"`
}

// IndexChunk identifies one byte range of the reconstructed file.
type IndexChunk struct {
	Offset int64  `json:"offset"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

// NewIndex builds a sidecar index from manifest chunk metadata.
func NewIndex(path string, file File) Index {
	index := Index{
		Version:       Version,
		Algorithm:     Algorithm,
		ChunkEncoding: ChunkEncodingRaw,
		Source: Source{
			Path:   path,
			Size:   file.Size,
			SHA256: strings.ToLower(file.SHA256),
		},
		Chunking: DefaultChunking(),
		Chunks:   make([]IndexChunk, len(file.Chunks)),
	}
	var offset int64
	for i, chunk := range file.Chunks {
		index.Chunks[i] = IndexChunk{
			Offset: offset,
			Size:   chunk.Size,
			SHA256: strings.ToLower(chunk.SHA256),
		}
		offset += chunk.Size
	}
	return index
}

// Validate checks the index format and its contiguous byte ranges for fileName.
func (index Index) Validate(fileName string) error {
	if index.Version != Version {
		return fmt.Errorf("unsupported index version %d", index.Version)
	}
	if index.Algorithm != Algorithm {
		return fmt.Errorf("unsupported algorithm %q", index.Algorithm)
	}
	if index.ChunkEncoding != ChunkEncodingRaw {
		return fmt.Errorf("unsupported chunk encoding %q", index.ChunkEncoding)
	}
	if err := index.Chunking.Validate(); err != nil {
		return err
	}
	if index.Source.Path != fileName {
		return fmt.Errorf("index source path %q does not match file %q", index.Source.Path, fileName)
	}
	if index.Source.Size < 0 || !ValidDigest(index.Source.SHA256) {
		return fmt.Errorf("invalid index source for %q", fileName)
	}
	if len(index.Chunks) == 0 && index.Source.Size != 0 {
		return fmt.Errorf("index for %q has no chunks", fileName)
	}
	var offset int64
	for _, chunk := range index.Chunks {
		if chunk.Offset != offset {
			return fmt.Errorf("index for %q has non-contiguous chunk at offset %d", fileName, chunk.Offset)
		}
		if chunk.Size <= 0 || !ValidDigest(chunk.SHA256) {
			return fmt.Errorf("invalid index chunk for %q", fileName)
		}
		offset += chunk.Size
	}
	if offset != index.Source.Size {
		return fmt.Errorf("index for %q has size %d, want %d", fileName, offset, index.Source.Size)
	}
	return nil
}

// ValidDigest reports whether s is a SHA-256 hex digest.
func ValidDigest(s string) bool {
	if len(s) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}
