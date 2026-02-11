package rclone

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"time"

	"github.com/rclone/rclone/fs"
	"github.com/rclone/rclone/fs/hash"
	"github.com/rclone/rclone/fs/operations"
	rcloneWalk "github.com/rclone/rclone/fs/walk"

	"github.com/nuln/sbox"
)

// Auto-register rclone storage driver.
func init() {
	sbox.Register("rclone", func(cfg *sbox.Config) (sbox.StorageEngine, error) {
		remote := ""
		if v, ok := cfg.Options["remote"]; ok {
			remote, _ = v.(string)
		}
		if remote == "" {
			remote = cfg.BasePath
		}
		if remote == "" {
			return nil, fmt.Errorf("sbox/rclone: remote path is required (set Options[\"remote\"] or BasePath)")
		}
		return New(remote)
	})
}

// Engine implements sbox.StorageEngine using rclone's fs.Fs.
type Engine struct {
	remote fs.Fs
}

// New creates a new rclone Engine from a remote path (e.g., "gdrive:backup").
func New(remotePath string) (*Engine, error) {
	remote, err := fs.NewFs(context.Background(), remotePath)
	if err != nil {
		return nil, err
	}
	return &Engine{remote: remote}, nil
}

func (e *Engine) Stat(ctx context.Context, p string) (*sbox.EntryInfo, error) {
	obj, err := e.remote.NewObject(ctx, p)
	if err != nil {
		// Might be a directory
		entries, errDir := e.remote.List(ctx, p)
		if errDir == nil && len(entries) > 0 {
			return &sbox.EntryInfo{
				Name:  path.Base(p),
				Path:  p,
				IsDir: true,
			}, nil
		}
		return nil, convertError(err)
	}

	return &sbox.EntryInfo{
		Name:    path.Base(obj.Remote()),
		Path:    p,
		Size:    obj.Size(),
		ModTime: obj.ModTime(ctx),
		IsDir:   false,
	}, nil
}

func (e *Engine) Open(ctx context.Context, path string) (sbox.ReadSeekCloser, error) {
	obj, err := e.remote.NewObject(ctx, path)
	if err != nil {
		return nil, convertError(err)
	}

	// Rclone objects don't natively support Seek. Download to a temp file.
	tmp, err := os.CreateTemp("", "sbox-rclone-*")
	if err != nil {
		return nil, err
	}

	rc, err := obj.Open(ctx)
	if err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return nil, err
	}

	if _, err := io.Copy(tmp, rc); err != nil {
		_ = rc.Close()
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return nil, err
	}
	_ = rc.Close()

	// Seek back to start
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return nil, err
	}

	return &tempFileReader{File: tmp}, nil
}

// tempFileReader wraps an os.File and deletes it on Close.
type tempFileReader struct {
	*os.File
}

func (t *tempFileReader) Close() error {
	name := t.File.Name()
	err := t.File.Close()
	_ = os.Remove(name)
	return err
}

func (e *Engine) Create(ctx context.Context, p string) (sbox.WriteCloser, error) {
	return &rcloneWriter{
		engine: e,
		path:   p,
		ctx:    ctx,
	}, nil
}

func (e *Engine) OpenFile(ctx context.Context, p string, flag int, perm os.FileMode) (sbox.WriteSeekCloser, error) {
	w := &rcloneWriteSeeker{
		engine: e,
		path:   p,
		ctx:    ctx,
	}

	// If appending, download existing content first
	if flag&os.O_APPEND != 0 {
		obj, err := e.remote.NewObject(ctx, p)
		if err == nil {
			rc, err := obj.Open(ctx)
			if err == nil {
				existing, _ := io.ReadAll(rc)
				_ = rc.Close()
				w.buf = existing
			}
		}
	}

	return w, nil
}

// rcloneWriter implements WriteCloser for rclone.
type rcloneWriter struct {
	engine *Engine
	path   string
	ctx    context.Context
	buf    []byte
}

func (w *rcloneWriter) Write(p []byte) (n int, err error) {
	w.buf = append(w.buf, p...)
	return len(p), nil
}

func (w *rcloneWriter) Close() error {
	rc := io.NopCloser(io.NewSectionReader(newBytesReaderAt(w.buf), 0, int64(len(w.buf))))
	_, err := operations.Rcat(w.ctx, w.engine.remote, w.path, rc, time.Now(), nil)
	return err
}

// rcloneWriteSeeker implements WriteSeekCloser for rclone.
type rcloneWriteSeeker struct {
	engine *Engine
	path   string
	ctx    context.Context
	buf    []byte
	offset int64
}

func (w *rcloneWriteSeeker) Write(p []byte) (n int, err error) {
	w.buf = append(w.buf, p...)
	w.offset += int64(len(p))
	return len(p), nil
}

func (w *rcloneWriteSeeker) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case io.SeekStart:
		w.offset = offset
	case io.SeekCurrent:
		w.offset += offset
	case io.SeekEnd:
		w.offset = int64(len(w.buf)) + offset
	}
	return w.offset, nil
}

func (w *rcloneWriteSeeker) Close() error {
	rc := io.NopCloser(io.NewSectionReader(newBytesReaderAt(w.buf), 0, int64(len(w.buf))))
	_, err := operations.Rcat(w.ctx, w.engine.remote, w.path, rc, time.Now(), nil)
	return err
}

func (e *Engine) Remove(ctx context.Context, path string) error {
	obj, err := e.remote.NewObject(ctx, path)
	if err != nil {
		// Try as directory
		return operations.Purge(ctx, e.remote, path)
	}
	return obj.Remove(ctx)
}

func (e *Engine) Rename(ctx context.Context, oldPath, newPath string) error {
	return operations.MoveFile(ctx, e.remote, e.remote, newPath, oldPath)
}

func (e *Engine) MkdirAll(ctx context.Context, path string) error {
	return e.remote.Mkdir(ctx, path)
}

func (e *Engine) ReadDir(ctx context.Context, dirPath string) ([]*sbox.EntryInfo, error) {
	entries, err := e.remote.List(ctx, dirPath)
	if err != nil {
		return nil, convertError(err)
	}

	var result []*sbox.EntryInfo
	for _, entry := range entries {
		info := &sbox.EntryInfo{
			Name: path.Base(entry.Remote()),
			Path: filepath.Join(dirPath, path.Base(entry.Remote())),
		}
		if obj, ok := entry.(fs.Object); ok {
			info.Size = obj.Size()
			info.ModTime = obj.ModTime(ctx)
			info.IsDir = false
		} else {
			info.IsDir = true
		}
		result = append(result, info)
	}
	return result, nil
}

// === Extension: StreamReader ===

func (e *Engine) Get(ctx context.Context, path string) (io.ReadCloser, error) {
	obj, err := e.remote.NewObject(ctx, path)
	if err != nil {
		return nil, convertError(err)
	}
	return obj.Open(ctx)
}

// === Extension: RangeReader ===

func (e *Engine) GetRange(ctx context.Context, path string, offset, length int64) (io.ReadCloser, error) {
	obj, err := e.remote.NewObject(ctx, path)
	if err != nil {
		return nil, convertError(err)
	}

	var options []fs.OpenOption
	if length > 0 {
		options = append(options, &fs.RangeOption{Start: offset, End: offset + length - 1})
	} else if offset > 0 {
		options = append(options, &fs.RangeOption{Start: offset, End: -1})
	}

	return obj.Open(ctx, options...)
}

// === Extension: Hasher ===

func (e *Engine) Hash(ctx context.Context, path string, algorithm string) (string, error) {
	obj, err := e.remote.NewObject(ctx, path)
	if err != nil {
		return "", convertError(err)
	}

	var ht hash.Type
	switch algorithm {
	case "md5":
		ht = hash.MD5
	case "sha1":
		ht = hash.SHA1
	case "sha256":
		ht = hash.SHA256
	default:
		return "", fmt.Errorf("sbox/rclone: unsupported hash algorithm: %s", algorithm)
	}

	h, err := obj.Hash(ctx, ht)
	if err != nil {
		if err == hash.ErrUnsupported {
			return "", sbox.ErrNotSupported
		}
		return "", err
	}
	if h == "" {
		return "", sbox.ErrNotSupported
	}
	return h, nil
}

// === Extension: Copier ===

func (e *Engine) Copy(ctx context.Context, src, dst string) error {
	return operations.CopyFile(ctx, e.remote, e.remote, dst, src)
}

// === Extension: SignedURLGenerator ===

func (e *Engine) SignedURL(ctx context.Context, path string, expiry time.Duration) (string, error) {
	do, ok := e.remote.(fs.PublicLinker)
	if !ok {
		return "", fmt.Errorf("sbox/rclone: remote does not support public links")
	}
	return do.PublicLink(ctx, path, fs.Duration(expiry), false)
}

// === Extension: StreamWriter ===

func (e *Engine) Put(ctx context.Context, path string, reader io.Reader) error {
	rc, ok := reader.(io.ReadCloser)
	if !ok {
		rc = io.NopCloser(reader)
	}
	_, err := operations.Rcat(ctx, e.remote, path, rc, time.Now(), nil)
	return err
}

// === Walk helper (used by sbox.Walk but rclone has native support) ===

// WalkNative performs a native rclone walk, which is more efficient than
// the generic sbox.Walk for remote backends.
func (e *Engine) WalkNative(ctx context.Context, p string, fn sbox.WalkFunc) error {
	return rcloneWalk.Walk(ctx, e.remote, p, true, -1, func(walkPath string, entries fs.DirEntries, err error) error {
		if err != nil {
			return fn(walkPath, nil, err)
		}
		for _, entry := range entries {
			info := &sbox.EntryInfo{
				Name: path.Base(entry.Remote()),
				Path: entry.Remote(),
			}
			if obj, ok := entry.(fs.Object); ok {
				info.Size = obj.Size()
				info.ModTime = obj.ModTime(ctx)
				info.IsDir = false
			} else {
				info.IsDir = true
			}
			if err := fn(entry.Remote(), info, nil); err != nil {
				return err
			}
		}
		return nil
	})
}

// Helpers

func convertError(err error) error {
	if err == nil {
		return nil
	}
	if err == fs.ErrorObjectNotFound || err == fs.ErrorDirNotFound {
		return os.ErrNotExist
	}
	return err
}

// bytesReaderAt implements io.ReaderAt for a byte slice.
type bytesReaderAt struct {
	data []byte
}

func newBytesReaderAt(data []byte) *bytesReaderAt {
	return &bytesReaderAt{data: data}
}

func (r *bytesReaderAt) ReadAt(p []byte, off int64) (n int, err error) {
	if off >= int64(len(r.data)) {
		return 0, io.EOF
	}
	n = copy(p, r.data[off:])
	if n < len(p) {
		err = io.EOF
	}
	return
}

// Compile-time interface checks.
var (
	_ sbox.StorageEngine      = (*Engine)(nil)
	_ sbox.StreamReader       = (*Engine)(nil)
	_ sbox.StreamWriter       = (*Engine)(nil)
	_ sbox.RangeReader        = (*Engine)(nil)
	_ sbox.Hasher             = (*Engine)(nil)
	_ sbox.Copier             = (*Engine)(nil)
	_ sbox.SignedURLGenerator = (*Engine)(nil)
)
