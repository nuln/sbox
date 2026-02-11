package local

import (
	"context"
	"crypto/md5" //nolint:gosec // md5 is intentionally supported
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/afero"

	"github.com/nuln/sbox"
)

// Auto-register local storage driver.
func init() {
	sbox.Register("local", func(cfg *sbox.Config) (sbox.StorageEngine, error) {
		return New(cfg.BasePath)
	})
}

// Engine implements sbox.StorageEngine for the local filesystem.
type Engine struct {
	fs   afero.Fs
	root string
}

// New creates a new local storage Engine with the given root directory.
func New(root string) (*Engine, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(absRoot, 0750); err != nil {
		return nil, err
	}
	return &Engine{
		fs:   afero.NewBasePathFs(afero.NewOsFs(), absRoot),
		root: absRoot,
	}, nil
}

// NewWithFs creates a local Engine backed by a custom afero.Fs.
// This is useful for testing with afero.MemMapFs.
func NewWithFs(fs afero.Fs) *Engine {
	return &Engine{fs: fs, root: "."}
}

func (e *Engine) Stat(ctx context.Context, path string) (*sbox.EntryInfo, error) {
	info, err := e.fs.Stat(path)
	if err != nil {
		return nil, err
	}
	return &sbox.EntryInfo{
		Name:    info.Name(),
		Size:    info.Size(),
		ModTime: info.ModTime(),
		Mode:    info.Mode(),
		IsDir:   info.IsDir(),
		Path:    path,
	}, nil
}

func (e *Engine) Open(ctx context.Context, path string) (sbox.ReadSeekCloser, error) {
	f, err := e.fs.Open(path)
	if err != nil {
		return nil, err
	}
	// afero.File implements ReadSeekCloser
	rsc, ok := f.(sbox.ReadSeekCloser)
	if !ok {
		_ = f.Close()
		return nil, fmt.Errorf("sbox/local: file does not support seek")
	}
	return rsc, nil
}

func (e *Engine) Create(ctx context.Context, path string) (sbox.WriteCloser, error) {
	if err := e.fs.MkdirAll(filepath.Dir(path), 0750); err != nil {
		return nil, err
	}
	return e.fs.Create(path)
}

func (e *Engine) OpenFile(ctx context.Context, path string, flag int, perm os.FileMode) (sbox.WriteSeekCloser, error) {
	if err := e.fs.MkdirAll(filepath.Dir(path), 0750); err != nil {
		return nil, err
	}
	f, err := e.fs.OpenFile(path, flag, perm)
	if err != nil {
		return nil, err
	}
	wsc, ok := f.(sbox.WriteSeekCloser)
	if !ok {
		_ = f.Close()
		return nil, fmt.Errorf("sbox/local: file does not support write+seek")
	}
	return wsc, nil
}

func (e *Engine) Remove(ctx context.Context, path string) error {
	return e.fs.RemoveAll(path)
}

func (e *Engine) Rename(ctx context.Context, oldPath, newPath string) error {
	if err := e.fs.MkdirAll(filepath.Dir(newPath), 0750); err != nil {
		return err
	}
	return e.fs.Rename(oldPath, newPath)
}

func (e *Engine) MkdirAll(ctx context.Context, path string) error {
	return e.fs.MkdirAll(path, 0750)
}

func (e *Engine) ReadDir(ctx context.Context, path string) ([]*sbox.EntryInfo, error) {
	f, err := e.fs.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	infos, err := f.Readdir(-1)
	if err != nil {
		return nil, err
	}

	result := make([]*sbox.EntryInfo, 0, len(infos))
	for _, info := range infos {
		result = append(result, &sbox.EntryInfo{
			Name:    info.Name(),
			Size:    info.Size(),
			ModTime: info.ModTime(),
			Mode:    info.Mode(),
			IsDir:   info.IsDir(),
			Path:    filepath.Join(path, info.Name()),
		})
	}
	return result, nil
}

// === Extension: Copier ===

func (e *Engine) Copy(ctx context.Context, src, dst string) error {
	srcInfo, err := e.fs.Stat(src)
	if err != nil {
		return err
	}
	if srcInfo.IsDir() {
		return e.copyDir(src, dst)
	}
	return e.copyFile(src, dst)
}

func (e *Engine) copyFile(src, dst string) error {
	if err := e.fs.MkdirAll(filepath.Dir(dst), 0750); err != nil {
		return err
	}
	sf, err := e.fs.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = sf.Close() }()

	df, err := e.fs.Create(dst)
	if err != nil {
		return err
	}
	defer func() { _ = df.Close() }()

	_, err = io.Copy(df, sf)
	return err
}

func (e *Engine) copyDir(src, dst string) error {
	if err := e.fs.MkdirAll(dst, 0750); err != nil {
		return err
	}
	entries, err := afero.ReadDir(e.fs, src)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())
		if entry.IsDir() {
			if err := e.copyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if err := e.copyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}
	return nil
}

// === Extension: Hasher ===

func (e *Engine) Hash(ctx context.Context, path string, algorithm string) (string, error) {
	f, err := e.fs.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()

	var h interface {
		io.Writer
		Sum([]byte) []byte
	}

	switch algorithm {
	case "md5":
		h = md5.New() //nolint:gosec // md5 intentionally supported
	case "sha256":
		h = sha256.New()
	default:
		return "", fmt.Errorf("sbox/local: unsupported hash algorithm: %s", algorithm)
	}

	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// === Extension: StreamReader ===

func (e *Engine) Get(ctx context.Context, path string) (io.ReadCloser, error) {
	return e.fs.Open(path)
}

// === Extension: StreamWriter ===

func (e *Engine) Put(ctx context.Context, path string, reader io.Reader) error {
	if err := e.fs.MkdirAll(filepath.Dir(path), 0750); err != nil {
		return err
	}
	f, err := e.fs.Create(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	_, err = io.Copy(f, reader)
	return err
}

// Compile-time interface checks.
var (
	_ sbox.StorageEngine = (*Engine)(nil)
	_ sbox.Copier        = (*Engine)(nil)
	_ sbox.Hasher        = (*Engine)(nil)
	_ sbox.StreamReader  = (*Engine)(nil)
	_ sbox.StreamWriter  = (*Engine)(nil)
)
