package sbox

import (
	"context"
	"path/filepath"
)

// WalkFunc is the callback for Walk. It is called for each file or directory
// visited. If it returns filepath.SkipDir for a directory, Walk skips that
// directory's contents.
type WalkFunc func(path string, info *EntryInfo, err error) error

// Walk walks the file tree rooted at root, calling fn for each file or
// directory in the tree, including root. It works with any StorageEngine.
func Walk(ctx context.Context, engine StorageEngine, root string, fn WalkFunc) error {
	info, err := engine.Stat(ctx, root)
	if err != nil {
		err = fn(root, nil, err)
	} else {
		err = walkDir(ctx, engine, root, info, fn)
	}
	if err == filepath.SkipDir {
		return nil
	}
	return err
}

func walkDir(ctx context.Context, engine StorageEngine, path string, info *EntryInfo, fn WalkFunc) error {
	if !info.IsDir {
		return fn(path, info, nil)
	}

	err := fn(path, info, nil)
	if err != nil {
		if err == filepath.SkipDir {
			return nil
		}
		return err
	}

	entries, err := engine.ReadDir(ctx, path)
	if err != nil {
		err = fn(path, nil, err)
		if err != nil {
			if err == filepath.SkipDir {
				return nil
			}
			return err
		}
	}

	for _, entry := range entries {
		err = walkDir(ctx, engine, entry.Path, entry, fn)
		if err != nil {
			if err == filepath.SkipDir {
				return nil
			}
			return err
		}
	}
	return nil
}
