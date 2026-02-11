package sharded_test

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/spf13/afero"

	"github.com/nuln/sbox/sboxtest"
	"github.com/nuln/sbox/sharded"
)

func newTestEngine() *sharded.Engine {
	manifestFs := afero.NewMemMapFs()
	shardsFs := afero.NewMemMapFs()
	return sharded.New(manifestFs, shardsFs, sharded.DefaultChunkSize)
}

func TestShardedEngine(t *testing.T) {
	engine := newTestEngine()
	sboxtest.StorageTestSuite(t, engine)
}

func TestShardedEngine_Deduplication(t *testing.T) {
	// Shared shards filesystem
	shardsFs := afero.NewMemMapFs()

	// Two separate users with their own manifest filesystems
	userAFs := afero.NewMemMapFs()
	userBFs := afero.NewMemMapFs()

	engineA := sharded.New(userAFs, shardsFs, sharded.DefaultChunkSize)
	engineB := sharded.New(userBFs, shardsFs, sharded.DefaultChunkSize)

	content := "this is shared content that should be deduplicated"
	path := "/test.txt"
	ctx := context.Background()

	// User A uploads
	wA, err := engineA.Create(ctx, path)
	if err != nil {
		t.Fatalf("Create A: %v", err)
	}
	_, _ = io.Copy(wA, strings.NewReader(content))
	_ = wA.Close()

	// User B uploads the same content
	wB, err := engineB.Create(ctx, path)
	if err != nil {
		t.Fatalf("Create B: %v", err)
	}
	_, _ = io.Copy(wB, strings.NewReader(content))
	_ = wB.Close()

	// Verify both users can read the file
	rA, err := engineA.Open(ctx, path)
	if err != nil {
		t.Fatalf("Open A: %v", err)
	}
	dataA, _ := io.ReadAll(rA)
	_ = rA.Close()
	if string(dataA) != content {
		t.Errorf("User A content = %q, want %q", string(dataA), content)
	}

	rB, err := engineB.Open(ctx, path)
	if err != nil {
		t.Fatalf("Open B: %v", err)
	}
	dataB, _ := io.ReadAll(rB)
	_ = rB.Close()
	if string(dataB) != content {
		t.Errorf("User B content = %q, want %q", string(dataB), content)
	}

	// Verify dedup: count shard files (should be exactly 1 since content < chunkSize)
	shardCount := 0
	countShards(t, shardsFs, "", &shardCount)
	if shardCount != 1 {
		t.Errorf("Expected 1 shard (dedup), got %d", shardCount)
	}

	// Verify independence: User A deletes, User B still has it
	_ = engineA.Remove(ctx, path)
	_, err = engineA.Stat(ctx, path)
	if err == nil {
		t.Error("User A: Stat after Remove should return error")
	}

	rB2, err := engineB.Open(ctx, path)
	if err != nil {
		t.Fatalf("User B Open after A deleted: %v", err)
	}
	dataB2, _ := io.ReadAll(rB2)
	_ = rB2.Close()
	if string(dataB2) != content {
		t.Errorf("User B after A delete = %q, want %q", string(dataB2), content)
	}
}

func countShards(t *testing.T, fs afero.Fs, dir string, count *int) {
	t.Helper()
	entries, err := afero.ReadDir(fs, dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		path := dir + "/" + e.Name()
		if dir == "" {
			path = e.Name()
		}
		if e.IsDir() {
			countShards(t, fs, path, count)
		} else {
			*count++
		}
	}
}
