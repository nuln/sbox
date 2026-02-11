package rclone_test

import (
	"context"
	"fmt"
	"net"
	"os"
	"testing"

	_ "github.com/rclone/rclone/backend/local"
	_ "github.com/rclone/rclone/backend/webdav"
	_ "github.com/rclone/rclone/cmd/serve"
	_ "github.com/rclone/rclone/cmd/serve/webdav"
	"github.com/rclone/rclone/fs/rc"

	"github.com/nuln/sbox"
	"github.com/nuln/sbox/sboxtest"
)

func TestRcloneEngine_WebDAV(t *testing.T) {
	// 1. Setup local directory to serve via WebDAV
	tempDir, err := os.MkdirTemp("", "sbox-rclone-webdav-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	// 2. Find a free port
	l, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("Failed to listen: %v", err)
	}
	addr := l.Addr().String()
	_ = l.Close()

	// 3. Start rclone serve webdav programmatically
	ctx := context.Background()
	startCall := rc.Calls.Get("serve/start")
	if startCall == nil {
		t.Fatal("serve/start RC not found - make sure github.com/rclone/rclone/cmd/serve is imported")
	}

	out, err := startCall.Fn(ctx, rc.Params{
		"type": "webdav",
		"fs":   tempDir,
		"addr": addr,
	})
	if err != nil {
		t.Fatalf("Failed to start rclone webdav: %v", err)
	}
	serverID, ok := out["id"].(string)
	if !ok {
		t.Fatal("serve/start did not return id string")
	}
	serverAddr, ok := out["addr"].(string)
	if !ok {
		t.Fatal("serve/start did not return addr string")
	}

	defer func() {
		stopCall := rc.Calls.Get("serve/stop")
		if stopCall != nil {
			_, _ = stopCall.Fn(ctx, rc.Params{"id": serverID})
		}
	}()

	// 4. Initialize sbox rclone engine
	// Remote format: :webdav,url='http://addr':
	remotePath := fmt.Sprintf(":webdav,url='http://%s':", serverAddr)
	cfg := &sbox.Config{
		Type: "rclone",
		Options: map[string]any{
			"remote": remotePath,
		},
	}

	engine, err := sbox.Open(cfg)
	if err != nil {
		t.Fatalf("Failed to open rclone engine: %v", err)
	}

	// 5. Run the universal storage test suite
	sboxtest.StorageTestSuite(t, engine)
}
