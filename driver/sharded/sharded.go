package sharded

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/spf13/afero"

	"github.com/nuln/sbox"
)

// DefaultChunkSize is the default chunk size (4MB).
const DefaultChunkSize = 4 * 1024 * 1024

// Auto-register sharded storage driver.
func init() {
	sbox.Register("sharded", func(cfg *sbox.Config) (sbox.StorageEngine, error) {
		chunkSize := int64(DefaultChunkSize)
		if v, ok := cfg.Options["chunkSize"]; ok {
			switch n := v.(type) {
			case int:
				chunkSize = int64(n)
			case int64:
				chunkSize = n
			case float64:
				chunkSize = int64(n)
			}
		}

		basePath := cfg.BasePath
		if basePath == "" {
			basePath = "./data"
		}

		manifestPath := filepath.Join(basePath, "manifest")
		if v, ok := cfg.Options["manifestDir"]; ok {
			if s, ok := v.(string); ok {
				manifestPath = s
			}
		}

		shardsPath := filepath.Join(basePath, "shards")
		if v, ok := cfg.Options["shardsDir"]; ok {
			if s, ok := v.(string); ok {
				shardsPath = s
			}
		}

		// Ensure directories exist
		if err := os.MkdirAll(manifestPath, 0750); err != nil {
			return nil, err
		}
		if err := os.MkdirAll(shardsPath, 0750); err != nil {
			return nil, err
		}

		manifestFs := afero.NewBasePathFs(afero.NewOsFs(), manifestPath)
		shardsFs := afero.NewBasePathFs(afero.NewOsFs(), shardsPath)

		return New(manifestFs, shardsFs, chunkSize), nil
	})
}

// Engine implements sbox.StorageEngine using content-addressed chunked storage.
type Engine struct {
	manifestFs afero.Fs
	shardsFs   afero.Fs
	chunkSize  int64
	bufferPool *sync.Pool
}

// New creates a new sharded Engine.
// manifestFs stores manifest JSON files (mirroring logical paths),
// shardsFs stores chunk blobs (content-addressed via HashPath).
// They can share the same filesystem or be separate (e.g., for cross-user dedup).
func New(manifestFs, shardsFs afero.Fs, chunkSize int64) *Engine {
	if chunkSize <= 0 {
		chunkSize = DefaultChunkSize
	}
	e := &Engine{
		manifestFs: manifestFs,
		shardsFs:   shardsFs,
		chunkSize:  chunkSize,
	}
	e.bufferPool = &sync.Pool{
		New: func() interface{} {
			b := make([]byte, e.chunkSize)
			return &b
		},
	}
	return e
}

// cleanPath normalizes a logical path for manifest storage.
func cleanPath(p string) string {
	clean := filepath.Clean(p)
	clean = filepath.ToSlash(clean)
	clean = strings.TrimPrefix(clean, "/")
	if clean == "" || clean == "." {
		return ""
	}
	return clean
}

// manifestPath returns the manifest file path that mirrors the logical path.
// e.g. "test/hello.txt" → "manifests/test/hello.txt.json"
func (e *Engine) manifestPath(path string) string {
	p := cleanPath(path)
	if p == "" {
		return "manifests"
	}
	return filepath.Join("manifests", p+".json")
}

// manifestDirPath returns the manifest directory path that mirrors the logical path.
// e.g. "test/dirops" → "manifests/test/dirops"
func (e *Engine) manifestDirPath(path string) string {
	p := cleanPath(path)
	if p == "" {
		return "manifests"
	}
	return filepath.Join("manifests", p)
}

func (e *Engine) shardPath(hash string) string {
	return sbox.HashPath(hash)
}

// Stat returns information about a logical file or directory.
func (e *Engine) Stat(ctx context.Context, path string) (*sbox.EntryInfo, error) {
	p := cleanPath(path)
	if p == "" {
		return &sbox.EntryInfo{
			Name:  "/",
			IsDir: true,
			Path:  path,
		}, nil
	}

	// Try as file (load manifest)
	mPath := e.manifestPath(path)
	data, err := afero.ReadFile(e.manifestFs, mPath)
	if err == nil {
		var m sbox.Manifest
		if unmarshalErr := json.Unmarshal(data, &m); unmarshalErr != nil {
			return nil, unmarshalErr
		}
		return &sbox.EntryInfo{
			Name:    filepath.Base(p),
			Size:    m.Size,
			ModTime: m.ModTime,
			IsDir:   false,
			Path:    path,
		}, nil
	}

	// Try as directory
	mDir := e.manifestDirPath(path)
	info, err := e.manifestFs.Stat(mDir)
	if err == nil && info.IsDir() {
		return &sbox.EntryInfo{
			Name:    filepath.Base(p),
			ModTime: info.ModTime(),
			IsDir:   true,
			Path:    path,
		}, nil
	}

	return nil, os.ErrNotExist
}

// Open returns a reader that transparently stitches shards together.
func (e *Engine) Open(ctx context.Context, path string) (sbox.ReadSeekCloser, error) {
	mPath := e.manifestPath(path)
	data, err := afero.ReadFile(e.manifestFs, mPath)
	if err != nil {
		return nil, err
	}
	var m sbox.Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return newShardedReader(e, m), nil
}

// Create creates or overwrites a file for writing.
func (e *Engine) Create(ctx context.Context, path string) (sbox.WriteCloser, error) {
	return e.OpenFile(ctx, path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
}

// OpenFile returns a WriteSeekCloser.
func (e *Engine) OpenFile(ctx context.Context, path string, flag int, perm os.FileMode) (sbox.WriteSeekCloser, error) {
	var buf []byte
	var pb *[]byte
	if pbi, ok := e.bufferPool.Get().(*[]byte); ok && pbi != nil {
		pb = pbi
		buf = (*pb)[:0]
	} else {
		buf = make([]byte, e.chunkSize)[:0]
	}

	writer := &shardedWriter{
		engine: e,
		path:   path,
		buffer: buf,
		pbuf:   pb,
	}

	mPath := e.manifestPath(path)
	exists, _ := afero.Exists(e.manifestFs, mPath)

	// If appending, load existing manifest
	if exists && (flag&os.O_APPEND != 0) && (flag&os.O_TRUNC == 0) {
		data, err := afero.ReadFile(e.manifestFs, mPath)
		if err == nil {
			var m sbox.Manifest
			if err := json.Unmarshal(data, &m); err == nil {
				writer.hashes = m.Chunks
				writer.chunkSizes = m.ChunkSizes
				writer.size = m.Size

				// Ensure ChunkSizes is populated for existing fixed-size files
				if len(writer.chunkSizes) == 0 && len(writer.hashes) > 0 {
					for i := 0; i < len(writer.hashes)-1; i++ {
						writer.chunkSizes = append(writer.chunkSizes, e.chunkSize)
					}
					lastSize := writer.size - int64(len(writer.hashes)-1)*e.chunkSize
					writer.chunkSizes = append(writer.chunkSizes, lastSize)
				}
			}
		}
	} else if flag&os.O_CREATE != 0 {
		// Ensure parent directory exists in manifest fs
		if err := e.manifestFs.MkdirAll(filepath.Dir(mPath), 0755); err != nil {
			return nil, err
		}
	}

	return writer, nil
}

// Remove deletes a file or directory.
func (e *Engine) Remove(ctx context.Context, path string) error {
	mPath := e.manifestPath(path)
	exists, _ := afero.Exists(e.manifestFs, mPath)
	if exists {
		// Only remove the manifest. Shards are content-addressed and may be
		// shared; orphan cleanup should be done separately (GC).
		return e.manifestFs.Remove(mPath)
	}
	mDir := e.manifestDirPath(path)
	return e.manifestFs.RemoveAll(mDir)
}

// Rename moves or renames a file or directory.
func (e *Engine) Rename(ctx context.Context, oldPath, newPath string) error {
	oldM := e.manifestPath(oldPath)
	newM := e.manifestPath(newPath)

	exists, _ := afero.Exists(e.manifestFs, oldM)
	if exists {
		if err := e.manifestFs.MkdirAll(filepath.Dir(newM), 0755); err != nil {
			return err
		}
		return e.manifestFs.Rename(oldM, newM)
	}

	oldD := e.manifestDirPath(oldPath)
	newD := e.manifestDirPath(newPath)
	if err := e.manifestFs.MkdirAll(filepath.Dir(newD), 0755); err != nil {
		return err
	}
	return e.manifestFs.Rename(oldD, newD)
}

// MkdirAll creates a directory (mirrored in manifest filesystem).
func (e *Engine) MkdirAll(ctx context.Context, path string) error {
	mDir := e.manifestDirPath(path)
	return e.manifestFs.MkdirAll(mDir, 0755)
}

// ReadDir returns the contents of a directory.
func (e *Engine) ReadDir(ctx context.Context, path string) ([]*sbox.EntryInfo, error) {
	mDir := e.manifestDirPath(path)
	entries, err := afero.ReadDir(e.manifestFs, mDir)
	if err != nil {
		if os.IsNotExist(err) {
			p := cleanPath(path)
			if p == "" {
				return []*sbox.EntryInfo{}, nil
			}
			return nil, os.ErrNotExist
		}
		return nil, err
	}

	result := make([]*sbox.EntryInfo, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() {
			result = append(result, &sbox.EntryInfo{
				Name:    name,
				ModTime: entry.ModTime(),
				IsDir:   true,
				Path:    filepath.Join(path, name),
			})
		} else if strings.HasSuffix(name, ".json") {
			logicalName := strings.TrimSuffix(name, ".json")
			var size int64
			var modTime time.Time
			mData, err := afero.ReadFile(e.manifestFs, filepath.Join(mDir, name))
			if err == nil {
				var m sbox.Manifest
				if err := json.Unmarshal(mData, &m); err == nil {
					size = m.Size
					modTime = m.ModTime
				}
			}
			result = append(result, &sbox.EntryInfo{
				Name:    logicalName,
				Size:    size,
				ModTime: modTime,
				IsDir:   false,
				Path:    filepath.Join(path, logicalName),
			})
		}
	}
	return result, nil
}

// === Extension: Copier ===

// Copy copies a file by duplicating only its manifest (zero-copy for shards).
func (e *Engine) Copy(ctx context.Context, src, dst string) error {
	srcM := e.manifestPath(src)
	dstM := e.manifestPath(dst)

	data, err := afero.ReadFile(e.manifestFs, srcM)
	if err != nil {
		return err
	}

	if err := e.manifestFs.MkdirAll(filepath.Dir(dstM), 0755); err != nil {
		return err
	}
	return afero.WriteFile(e.manifestFs, dstM, data, 0644)
}

// === Extension: Hasher ===

func (e *Engine) Hash(ctx context.Context, path string, algorithm string) (string, error) {
	if algorithm != "sha256" {
		return "", fmt.Errorf("sbox/sharded: only sha256 is supported")
	}
	r, err := e.Open(ctx, path)
	if err != nil {
		return "", err
	}
	defer func() { _ = r.Close() }()

	h := sha256.New()
	if _, err := copyBuffered(h, r); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// Compile-time interface checks.
var (
	_ sbox.StorageEngine = (*Engine)(nil)
	_ sbox.Copier        = (*Engine)(nil)
	_ sbox.Hasher        = (*Engine)(nil)
)
