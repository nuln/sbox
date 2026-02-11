package sboxtest

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/nuln/sbox"
)

// StorageTestSuite runs a comprehensive set of tests against a StorageEngine
// implementation. Call this in your driver tests to verify correctness:
//
//	func TestLocalStorage(t *testing.T) {
//	    engine := setupEngine(t)
//	    sboxtest.StorageTestSuite(t, engine)
//	}
func StorageTestSuite(t *testing.T, engine sbox.StorageEngine) { //nolint:gocyclo
	t.Helper()
	ctx := context.Background()

	t.Run("Create_Open_Stat_Remove", func(t *testing.T) {
		path := "test/hello.txt"
		content := "hello world"

		// Create
		w, err := engine.Create(ctx, path)
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if _, writeErr := io.WriteString(w, content); writeErr != nil {
			t.Fatalf("Write: %v", writeErr)
		}
		if closeErr := w.Close(); closeErr != nil {
			t.Fatalf("Close writer: %v", closeErr)
		}

		// Stat
		info, err := engine.Stat(ctx, path)
		if err != nil {
			t.Fatalf("Stat: %v", err)
		}
		if info.Name != "hello.txt" {
			t.Errorf("Name = %q, want %q", info.Name, "hello.txt")
		}
		if info.Size != int64(len(content)) {
			t.Errorf("Size = %d, want %d", info.Size, len(content))
		}
		if info.IsDir {
			t.Error("IsDir = true, want false")
		}

		// Open + Read
		r, err := engine.Open(ctx, path)
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		data, err := io.ReadAll(r)
		if err != nil {
			t.Fatalf("ReadAll: %v", err)
		}
		_ = r.Close()
		if string(data) != content {
			t.Errorf("content = %q, want %q", string(data), content)
		}

		// Seek
		r2, err := engine.Open(ctx, path)
		if err != nil {
			t.Fatalf("Open for seek: %v", err)
		}
		if _, seekErr := r2.Seek(6, io.SeekStart); seekErr != nil {
			t.Fatalf("Seek: %v", seekErr)
		}
		partial, _ := io.ReadAll(r2)
		_ = r2.Close()
		if string(partial) != "world" {
			t.Errorf("after seek = %q, want %q", string(partial), "world")
		}

		// Remove
		if removeErr := engine.Remove(ctx, path); removeErr != nil {
			t.Fatalf("Remove: %v", removeErr)
		}
		_, err = engine.Stat(ctx, path)
		if err == nil {
			t.Error("Stat after Remove: expected error, got nil")
		}
	})

	t.Run("MkdirAll_ReadDir", func(t *testing.T) {
		dir := "test/dirops"
		if err := engine.MkdirAll(ctx, dir); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}

		// Create files
		for _, name := range []string{"a.txt", "b.txt"} {
			w, err := engine.Create(ctx, dir+"/"+name)
			if err != nil {
				t.Fatalf("Create %s: %v", name, err)
			}
			_, _ = io.WriteString(w, name)
			_ = w.Close()
		}

		// ReadDir
		entries, err := engine.ReadDir(ctx, dir)
		if err != nil {
			t.Fatalf("ReadDir: %v", err)
		}
		if len(entries) != 2 {
			t.Errorf("ReadDir: got %d entries, want 2", len(entries))
		}

		// Cleanup
		_ = engine.Remove(ctx, "test")
	})

	t.Run("Rename", func(t *testing.T) {
		src := "rename_src.txt"
		dst := "rename_dst.txt"

		w, _ := engine.Create(ctx, src)
		_, _ = io.WriteString(w, "data")
		_ = w.Close()

		if err := engine.Rename(ctx, src, dst); err != nil {
			t.Fatalf("Rename: %v", err)
		}

		// src should not exist
		_, err := engine.Stat(ctx, src)
		if err == nil {
			t.Error("Stat src after Rename: expected error")
		}
		// dst should exist
		info, err := engine.Stat(ctx, dst)
		if err != nil {
			t.Fatalf("Stat dst: %v", err)
		}
		if info.Size != 4 {
			t.Errorf("dst size = %d, want 4", info.Size)
		}

		_ = engine.Remove(ctx, dst)
	})

	t.Run("OpenFile_Append", func(t *testing.T) {
		path := "append_test.txt"

		// Create initial file
		w, _ := engine.Create(ctx, path)
		_, _ = io.WriteString(w, "hello")
		_ = w.Close()

		// Append
		aw, err := engine.OpenFile(ctx, path, os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			t.Fatalf("OpenFile append: %v", err)
		}
		_, _ = io.WriteString(aw, " world")
		_ = aw.Close()

		// Verify
		r, _ := engine.Open(ctx, path)
		data, _ := io.ReadAll(r)
		_ = r.Close()

		if string(data) != "hello world" {
			t.Errorf("after append = %q, want %q", string(data), "hello world")
		}

		_ = engine.Remove(ctx, path)
	})

	t.Run("Walk", func(t *testing.T) {
		// Create structure
		_ = engine.MkdirAll(ctx, "walk/sub")
		w1, _ := engine.Create(ctx, "walk/f1.txt")
		_, _ = io.WriteString(w1, "1")
		_ = w1.Close()
		w2, _ := engine.Create(ctx, "walk/sub/f2.txt")
		_, _ = io.WriteString(w2, "2")
		_ = w2.Close()

		var files []string
		err := sbox.Walk(ctx, engine, "walk", func(path string, info *sbox.EntryInfo, err error) error {
			if err != nil {
				return err
			}
			if !info.IsDir {
				files = append(files, info.Name)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("Walk: %v", err)
		}

		if len(files) != 2 {
			t.Errorf("Walk found %d files, want 2: %v", len(files), files)
		}

		_ = engine.Remove(ctx, "walk")
	})

	// Test extensions if supported
	if copier, ok := engine.(sbox.Copier); ok {
		t.Run("Copier", func(t *testing.T) {
			src := "copy_src.txt"
			dst := "copy_dst.txt"

			w, _ := engine.Create(ctx, src)
			_, _ = io.WriteString(w, "copy me")
			_ = w.Close()

			if err := copier.Copy(ctx, src, dst); err != nil {
				if err == sbox.ErrNotSupported {
					t.Skip("Copy not supported by this backend")
				}
				t.Fatalf("Copy: %v", err)
			}

			r, _ := engine.Open(ctx, dst)
			data, _ := io.ReadAll(r)
			_ = r.Close()
			if string(data) != "copy me" {
				t.Errorf("Copy content = %q, want %q", string(data), "copy me")
			}

			_ = engine.Remove(ctx, src)
			_ = engine.Remove(ctx, dst)
		})
	}

	if hasher, ok := engine.(sbox.Hasher); ok {
		t.Run("Hasher", func(t *testing.T) {
			path := "hash_test.txt"
			w, _ := engine.Create(ctx, path)
			_, _ = io.WriteString(w, "hash me")
			_ = w.Close()

			hash, err := hasher.Hash(ctx, path, "sha256")
			if err == sbox.ErrNotSupported {
				t.Skip("Hash not supported by this backend")
			}
			if err != nil {
				t.Fatalf("Hash: %v", err)
			}
			if hash == "" {
				t.Error("Hash returned empty string")
			}

			// Verify deterministic
			hash2, _ := hasher.Hash(ctx, path, "sha256")
			if hash != hash2 {
				t.Errorf("Hash not deterministic: %q != %q", hash, hash2)
			}

			_ = engine.Remove(ctx, path)
		})
	}

	if sr, ok := engine.(sbox.StreamReader); ok {
		t.Run("StreamReader", func(t *testing.T) {
			path := "stream_test.txt"
			w, _ := engine.Create(ctx, path)
			_, _ = io.WriteString(w, "stream data")
			_ = w.Close()

			rc, err := sr.Get(ctx, path)
			if err != nil {
				t.Fatalf("Get: %v", err)
			}
			data, _ := io.ReadAll(rc)
			_ = rc.Close()
			if !strings.Contains(string(data), "stream data") {
				t.Errorf("Get content = %q, want containing %q", string(data), "stream data")
			}

			_ = engine.Remove(ctx, path)
		})
	}
}
