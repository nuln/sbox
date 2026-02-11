package sbox

import (
	"io"
	"os"
	"time"
)

// EntryInfo describes a file or directory in a storage engine.
type EntryInfo struct {
	Name     string            `json:"name"`
	Size     int64             `json:"size"`
	ModTime  time.Time         `json:"modTime"`
	Mode     os.FileMode       `json:"mode"`
	IsDir    bool              `json:"isDir"`
	Path     string            `json:"path"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// ToFileInfo converts EntryInfo to a standard os.FileInfo.
func (e *EntryInfo) ToFileInfo() os.FileInfo {
	return &entryFileInfoWrap{e}
}

type entryFileInfoWrap struct {
	e *EntryInfo
}

func (w *entryFileInfoWrap) Name() string       { return w.e.Name }
func (w *entryFileInfoWrap) Size() int64        { return w.e.Size }
func (w *entryFileInfoWrap) Mode() os.FileMode  { return w.e.Mode }
func (w *entryFileInfoWrap) ModTime() time.Time { return w.e.ModTime }
func (w *entryFileInfoWrap) IsDir() bool        { return w.e.IsDir }
func (w *entryFileInfoWrap) Sys() interface{}   { return nil }

// ReadSeekCloser groups Read, Seek, and Close.
type ReadSeekCloser = io.ReadSeekCloser

// WriteCloser groups Write and Close.
type WriteCloser = io.WriteCloser

// WriteSeekCloser groups Write, Seek, and Close.
type WriteSeekCloser interface {
	io.Writer
	io.Seeker
	io.Closer
}

// Manifest represents the metadata of a chunked/sharded file.
type Manifest struct {
	Chunks     []string  `json:"chunks"`               // Chunk hashes
	ChunkSizes []int64   `json:"chunkSizes,omitempty"` // Per-chunk sizes (for variable-sized chunks)
	Size       int64     `json:"size"`
	ModTime    time.Time `json:"modTime"`
}
