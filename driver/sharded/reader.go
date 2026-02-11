package sharded

import (
	"errors"
	"io"

	"github.com/nuln/sbox"
)

// shardedReader implements sbox.ReadSeekCloser by transparently stitching
// shards together. It supports seeking to any offset within the logical file.
type shardedReader struct {
	engine   *Engine
	manifest sbox.Manifest
	offset   int64
}

func newShardedReader(e *Engine, m sbox.Manifest) *shardedReader {
	return &shardedReader{
		engine:   e,
		manifest: m,
		offset:   0,
	}
}

func (r *shardedReader) Read(p []byte) (n int, err error) { //nolint:gocyclo
	if r.offset >= r.manifest.Size {
		return 0, io.EOF
	}

	totalRead := 0
	for len(p) > 0 && r.offset < r.manifest.Size {
		var chunkIdx int
		var chunkOffset int64

		// Support variable-sized chunks
		if len(r.manifest.ChunkSizes) > 0 {
			current := int64(0)
			chunkIdx = -1
			for i, sz := range r.manifest.ChunkSizes {
				if r.offset < current+sz {
					chunkIdx = i
					chunkOffset = r.offset - current
					break
				}
				current += sz
			}
			if chunkIdx == -1 {
				return totalRead, io.ErrUnexpectedEOF
			}
		} else {
			chunkIdx = int(r.offset / r.engine.chunkSize)
			chunkOffset = r.offset % r.engine.chunkSize
		}

		if chunkIdx >= len(r.manifest.Chunks) {
			return totalRead, io.ErrUnexpectedEOF
		}

		hash := r.manifest.Chunks[chunkIdx]
		shardPath := r.engine.shardPath(hash)

		f, err := r.engine.shardsFs.Open(shardPath)
		if err != nil {
			return totalRead, err
		}

		if _, err := f.Seek(chunkOffset, io.SeekStart); err != nil {
			_ = f.Close()
			return totalRead, err
		}

		// Calculate how much can be read from this chunk
		var remainingInChunk int64
		if len(r.manifest.ChunkSizes) > 0 {
			remainingInChunk = r.manifest.ChunkSizes[chunkIdx] - chunkOffset
		} else {
			remainingInChunk = r.engine.chunkSize - chunkOffset
		}

		toRead := int(remainingInChunk)
		if toRead > len(p) {
			toRead = len(p)
		}

		read, readErr := f.Read(p[:toRead])
		_ = f.Close()

		if read > 0 {
			totalRead += read
			r.offset += int64(read)
			p = p[read:]
		}

		if readErr != nil && readErr != io.EOF {
			return totalRead, readErr
		}

		if read == 0 && readErr == io.EOF {
			break
		}
	}

	return totalRead, nil
}

func (r *shardedReader) Seek(offset int64, whence int) (int64, error) {
	var newOffset int64
	switch whence {
	case io.SeekStart:
		newOffset = offset
	case io.SeekCurrent:
		newOffset = r.offset + offset
	case io.SeekEnd:
		newOffset = r.manifest.Size + offset
	default:
		return 0, errors.New("sbox/sharded: invalid whence")
	}

	if newOffset < 0 || newOffset > r.manifest.Size {
		return 0, errors.New("sbox/sharded: seek offset out of range")
	}

	r.offset = newOffset
	return r.offset, nil
}

func (r *shardedReader) Close() error {
	return nil
}
