package sharded

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"time"

	"github.com/spf13/afero"

	"github.com/nuln/sbox"
)

// shardedWriter implements sbox.WriteSeekCloser for sharded storage.
// It accumulates data into chunks, hashes them, and writes unique shards.
type shardedWriter struct {
	engine     *Engine
	path       string
	hashes     []string
	chunkSizes []int64
	size       int64
	buffer     []byte
	pbuf       *[]byte
}

func (w *shardedWriter) Write(p []byte) (n int, err error) {
	total := len(p)
	for len(p) > 0 {
		space := int(w.engine.chunkSize) - len(w.buffer)
		if space > len(p) {
			w.buffer = append(w.buffer, p...)
			p = nil
		} else {
			w.buffer = append(w.buffer, p[:space]...)
			if err := w.flush(); err != nil {
				return 0, err
			}
			p = p[space:]
		}
	}
	w.size += int64(total)
	return total, nil
}

func (w *shardedWriter) flush() error {
	if len(w.buffer) == 0 {
		return nil
	}

	hash := sha256.Sum256(w.buffer)
	hashStr := hex.EncodeToString(hash[:])
	shardPath := w.engine.shardPath(hashStr)

	if err := w.engine.shardsFs.MkdirAll(filepath.Dir(shardPath), 0755); err != nil {
		return err
	}

	// Content-addressed: skip write if shard already exists (dedup)
	exists, _ := afero.Exists(w.engine.shardsFs, shardPath)
	if !exists {
		if err := afero.WriteFile(w.engine.shardsFs, shardPath, w.buffer, 0644); err != nil {
			return err
		}
	}

	w.hashes = append(w.hashes, hashStr)
	w.chunkSizes = append(w.chunkSizes, int64(len(w.buffer)))
	w.buffer = w.buffer[:0]
	return nil
}

func (w *shardedWriter) Seek(offset int64, whence int) (int64, error) {
	// Only support seeking to current end (for append/TUS compatibility)
	if whence == io.SeekStart && offset == w.size {
		return w.size, nil
	}
	if whence == io.SeekStart && offset == 0 && w.size == 0 {
		return 0, nil
	}
	return 0, errors.New("sbox/sharded: seek only supported to current end")
}

func (w *shardedWriter) Close() error {
	if err := w.flush(); err != nil {
		return err
	}

	manifest := sbox.Manifest{
		Chunks:     w.hashes,
		ChunkSizes: w.chunkSizes,
		Size:       w.size,
		ModTime:    time.Now(),
	}

	data, err := json.Marshal(manifest)
	if err != nil {
		return err
	}

	mPath := w.engine.manifestPath(w.path)
	if mkdirErr := w.engine.manifestFs.MkdirAll(filepath.Dir(mPath), 0750); mkdirErr != nil {
		return mkdirErr
	}

	err = afero.WriteFile(w.engine.manifestFs, mPath, data, 0644)

	// Return buffer to pool
	if w.pbuf != nil {
		*w.pbuf = w.buffer[:cap(w.buffer)]
		w.engine.bufferPool.Put(w.pbuf)
		w.pbuf = nil
		w.buffer = nil
	}
	return err
}

// copyBuffered is a helper for hashing.
func copyBuffered(dst io.Writer, src io.Reader) (int64, error) {
	buf := make([]byte, 32*1024)
	var total int64
	for {
		n, err := src.Read(buf)
		if n > 0 {
			nw, ew := dst.Write(buf[:n])
			if nw > 0 {
				total += int64(nw)
			}
			if ew != nil {
				return total, ew
			}
		}
		if err == io.EOF {
			return total, nil
		}
		if err != nil {
			return total, err
		}
	}
}
