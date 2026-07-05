// © 2026 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the ISC
// license that can be found in the LICENSE.md file.

// Package cdc defines deployd's content-defined artifact chunking contract.
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
	// Version is the schema version for deployd CDC sidecar indexes.
	Version = 1
	// Algorithm names deployd's complete CDC contract.
	Algorithm = "deployd-cdc-v1"
	// ChunkEncodingRaw records that chunks are stored as original bytes.
	ChunkEncodingRaw = "raw"
	// IndexSuffix is appended to artifact file names for CDC sidecar indexes.
	IndexSuffix = ".cdc.json"

	// MinSize is the smallest content-defined chunk size.
	MinSize = 512 << 10
	// AvgBits controls the statistical target chunk size: 2^AvgBits bytes.
	AvgBits = 20
	// MaxSize is the forced split size for data without natural CDC boundaries.
	MaxSize = 8 << 20
)

// Polynomial is deployd's fixed Rabin polynomial for content-defined chunking.
const Polynomial = chunker.Pol(0x2feedfaceb00b5)

// Chunking describes the stable CDC parameters signed in artifact manifests and
// recorded in generated sidecar indexes.
type Chunking struct {
	Version       int    `json:"version"`
	Algorithm     string `json:"algorithm"`
	ChunkEncoding string `json:"chunk_encoding"`
	Polynomial    string `json:"polynomial"`
	MinSize       uint   `json:"min_size"`
	AvgBits       int    `json:"avg_bits"`
	MaxSize       uint   `json:"max_size"`
}

// DefaultChunking returns deployd's current CDC compatibility contract.
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

// IsZero reports whether c is the zero value, which legacy artifact manifests
// use to mean fixed-size upload chunks.
func (c Chunking) IsZero() bool {
	return c == Chunking{}
}

// ValidateChunking verifies that c matches deployd's current CDC contract.
func ValidateChunking(c Chunking) error {
	want := DefaultChunking()
	if c.Version != want.Version {
		return fmt.Errorf("unsupported CDC version %d", c.Version)
	}
	if c.Algorithm != want.Algorithm {
		return fmt.Errorf("unsupported CDC algorithm %q", c.Algorithm)
	}
	if c.ChunkEncoding != want.ChunkEncoding {
		return fmt.Errorf("unsupported CDC chunk encoding %q", c.ChunkEncoding)
	}
	if c.Polynomial != want.Polynomial {
		return fmt.Errorf("unsupported CDC polynomial %q", c.Polynomial)
	}
	if c.MinSize != want.MinSize || c.AvgBits != want.AvgBits || c.MaxSize != want.MaxSize {
		return fmt.Errorf("unsupported CDC boundaries min=%d avg_bits=%d max=%d", c.MinSize, c.AvgBits, c.MaxSize)
	}
	return nil
}

// ManifestChunk describes one chunk in a signed artifact manifest.
type ManifestChunk struct {
	Index  int    `json:"index"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

// DataChunk is one content-defined chunk emitted by [WalkChunks].
type DataChunk struct {
	Index  int
	Offset int64
	Size   int64
	SHA256 string
	Data   []byte
}

// File describes the split result for one artifact file.
type File struct {
	Size   int64
	SHA256 string
	Chunks []ManifestChunk
}

// WalkChunks splits r with deployd's CDC contract, calls fn for each raw chunk,
// and returns the final file metadata. The Data field passed to fn is valid only
// until fn returns.
func WalkChunks(ctx context.Context, r io.Reader, fn func(DataChunk) error) (File, error) {
	fileHash := sha256.New()
	c := chunker.NewWithBoundaries(io.TeeReader(r, fileHash), Polynomial, MinSize, MaxSize)
	c.SetAverageBits(AvgBits)

	buf := make([]byte, 0, MaxSize)
	var file File
	var offset int64
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

		chunkSum := sha256.Sum256(chunk.Data)
		chunkID := hex.EncodeToString(chunkSum[:])
		chunkSize := int64(len(chunk.Data))
		dataChunk := DataChunk{
			Index:  index,
			Offset: offset,
			Size:   chunkSize,
			SHA256: chunkID,
			Data:   chunk.Data,
		}
		if fn != nil {
			if err := fn(dataChunk); err != nil {
				return File{}, err
			}
		}
		file.Chunks = append(file.Chunks, ManifestChunk{
			Index:  index,
			Size:   chunkSize,
			SHA256: chunkID,
		})
		offset += chunkSize
		buf = chunk.Data[:0]
	}
	file.Size = offset
	file.SHA256 = hex.EncodeToString(fileHash.Sum(nil))
	return file, nil
}

// Source describes the final file produced by a CDC index.
type Source struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

// Index is the release-local JSON sidecar that describes how to reconstruct one
// artifact file by concatenating raw content-defined chunks.
type Index struct {
	Version       int          `json:"version"`
	Algorithm     string       `json:"algorithm"`
	ChunkEncoding string       `json:"chunk_encoding"`
	Source        Source       `json:"source"`
	Chunking      Chunking     `json:"chunking"`
	Chunks        []IndexChunk `json:"chunks"`
}

// IndexChunk identifies one byte range of a reconstructed output file.
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
			SHA256: NormalizeSHA256(file.SHA256),
		},
		Chunking: DefaultChunking(),
		Chunks:   make([]IndexChunk, 0, len(file.Chunks)),
	}
	var offset int64
	for _, chunk := range file.Chunks {
		index.Chunks = append(index.Chunks, IndexChunk{
			Offset: offset,
			Size:   chunk.Size,
			SHA256: NormalizeSHA256(chunk.SHA256),
		})
		offset += chunk.Size
	}
	return index
}

// ValidateIndex checks that index matches the CDC contract and reconstructs the
// requested file as one contiguous byte stream.
func ValidateIndex(index Index, fileName string) error {
	if index.Version != Version {
		return fmt.Errorf("unsupported CDC index version %d", index.Version)
	}
	if index.Algorithm != Algorithm {
		return fmt.Errorf("unsupported CDC algorithm %q", index.Algorithm)
	}
	if index.ChunkEncoding != ChunkEncodingRaw {
		return fmt.Errorf("unsupported CDC chunk encoding %q", index.ChunkEncoding)
	}
	if err := ValidateChunking(index.Chunking); err != nil {
		return err
	}
	if index.Source.Path != fileName {
		return fmt.Errorf("CDC index source path %q does not match requested file %q", index.Source.Path, fileName)
	}
	if index.Source.Size < 0 || !IsSHA256Hex(index.Source.SHA256) {
		return fmt.Errorf("CDC index has invalid source metadata for %q", fileName)
	}
	if len(index.Chunks) == 0 && index.Source.Size != 0 {
		return fmt.Errorf("CDC index for %q has no chunks", fileName)
	}
	var offset int64
	for _, chunk := range index.Chunks {
		if chunk.Offset != offset {
			return fmt.Errorf("CDC index for %q has non-contiguous chunk at offset %d", fileName, chunk.Offset)
		}
		if chunk.Size <= 0 || !IsSHA256Hex(chunk.SHA256) {
			return fmt.Errorf("CDC index for %q has invalid chunk metadata", fileName)
		}
		offset += chunk.Size
	}
	if offset != index.Source.Size {
		return fmt.Errorf("CDC index for %q has size %d, want %d", fileName, offset, index.Source.Size)
	}
	return nil
}

// IndexName returns the release-local sidecar name for fileName.
func IndexName(fileName string) string {
	return fileName + IndexSuffix
}

// IsSHA256Hex reports whether s is a SHA-256 hex digest.
func IsSHA256Hex(s string) bool {
	if len(s) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}

// NormalizeSHA256 returns s as a lowercase SHA-256 digest.
func NormalizeSHA256(s string) string {
	return strings.ToLower(s)
}

// SHA256HexBytes returns the lowercase SHA-256 digest for content.
func SHA256HexBytes(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}
