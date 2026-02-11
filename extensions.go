package sbox

import (
	"context"
	"io"
	"time"
)

// StreamReader supports streaming read without Seek (suitable for remote backends).
// Use type assertion to check: if sr, ok := engine.(sbox.StreamReader); ok { ... }
type StreamReader interface {
	Get(ctx context.Context, path string) (io.ReadCloser, error)
}

// StreamWriter supports streaming write from a reader.
type StreamWriter interface {
	Put(ctx context.Context, path string, reader io.Reader) error
}

// RangeReader supports reading a specific byte range of a file.
type RangeReader interface {
	// GetRange returns a ReadCloser for a specific byte range.
	// offset: starting byte, length: number of bytes (-1 for until EOF).
	GetRange(ctx context.Context, path string, offset, length int64) (io.ReadCloser, error)
}

// Hasher supports calculating file hashes.
type Hasher interface {
	Hash(ctx context.Context, path string, algorithm string) (string, error)
}

// Copier supports file/directory copy. Some backends can implement this
// as a zero-copy or server-side operation.
type Copier interface {
	Copy(ctx context.Context, src, dst string) error
}

// SignedURLGenerator generates temporary access URLs (e.g., S3 presigned URLs).
type SignedURLGenerator interface {
	SignedURL(ctx context.Context, path string, expiry time.Duration) (string, error)
}
