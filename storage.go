package sbox

import (
	"context"
	"os"
)

// StorageEngine defines the unified interface for all storage backends.
// All driver implementations must satisfy this interface.
type StorageEngine interface {
	// Stat returns metadata about a file or directory.
	Stat(ctx context.Context, path string) (*EntryInfo, error)

	// Open opens a file for reading. The returned ReadSeekCloser supports
	// Seek, making it suitable for use with http.ServeContent.
	Open(ctx context.Context, path string) (ReadSeekCloser, error)

	// Create creates or overwrites a file for writing.
	Create(ctx context.Context, path string) (WriteCloser, error)

	// OpenFile opens a file with specific flags (e.g. os.O_APPEND).
	OpenFile(ctx context.Context, path string, flag int, perm os.FileMode) (WriteSeekCloser, error)

	// Remove deletes a file or directory (and all children).
	Remove(ctx context.Context, path string) error

	// Rename moves or renames a file or directory.
	Rename(ctx context.Context, oldPath, newPath string) error

	// MkdirAll creates a directory and all necessary parents.
	MkdirAll(ctx context.Context, path string) error

	// ReadDir returns the contents of a directory.
	ReadDir(ctx context.Context, path string) ([]*EntryInfo, error)
}
