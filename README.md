# sbox

A unified storage abstraction library for Go, providing a generic interface for multiple storage backends including local filesystem, content-addressed sharded storage, and any rclone-supported remotes.

## Features

- **Unified Interface**: File-system style API (`Open`, `Create`, `OpenFile`, `Stat`, `Remove`, `Rename`, `MkdirAll`, `ReadDir`).
- **Plug-and-play Drivers**: Support for local filesystem (afero-based), sharded CAS storage, and rclone.
- **Support for Seek/Append**: Native `ReadSeekCloser` and `WriteSeekCloser` support for efficient file operations.
- **Content-Addressed Storage**: Built-in sharded engine with deduplication and variable chunk size support.
- **Extensible Architecture**: Easy to implement new drivers and optional extension interfaces (`Hasher`, `Copier`, `SignedURLGenerator`, etc.).
- **Comprehensive Test Suite**: Includes a reusable test suite for validating custom storage implementations.

## Installation

```bash
go get github.com/nuln/sbox
```

## Quick Start

### 1. Initialize Drivers

Import the `drivers` package with a blank identifier to register all bundled drivers:

```go
import (
    "github.com/nuln/sbox"
    _ "github.com/nuln/sbox/drivers" // Register local, sharded, and rclone
)
```

Alternatively, you can call `drivers.Init()` explicitly:

```go
import "github.com/nuln/sbox/drivers"

func init() {
    drivers.Init()
}
```

### 2. Open a Storage Engine

```go
func main() {
    // Example: Local Filesystem
    cfg := &sbox.Config{
        Type:     "local",
        BasePath: "./data",
    }

    engine, err := sbox.Open(cfg)
    if err != nil {
        panic(err)
    }
}
```

### 3. Basic Operations

```go
ctx := context.Background()

// Create a file
w, _ := engine.Create(ctx, "hello.txt")
w.Write([]byte("hello world"))
w.Close()

// Read a file (supports Seek)
r, _ := engine.Open(ctx, "hello.txt")
defer r.Close()
data, _ := io.ReadAll(r)

// Seek to offset
r.Seek(6, io.SeekStart)
partial, _ := io.ReadAll(r)  // "world"

// Append to a file
aw, _ := engine.OpenFile(ctx, "hello.txt", os.O_WRONLY|os.O_APPEND, 0644)
aw.Write([]byte(" extension"))
aw.Close()
```

## Drivers Configuration

### 1. Local (local)

Simple file-system backend using `afero.OsFs`.

- `BasePath`: Root directory for storage.

### 2. Sharded CAS (sharded)

Content-addressed storage with deduplication.

- `BasePath`: Default root for both manifest and shards.
- `Options`:
    - `chunkSize` (int): Size of each chunk in bytes (default: 4MB).
    - `manifestDir` (string): Specific directory for manifest files.
    - `shardsDir` (string): Specific directory for shard blobs.

### 3. Rclone (rclone)

Supports 40+ backends. Note that you must import the specific rclone backend driver.

- `Options`:
    - `remote`: Rclone remote path (e.g., `:s3,provider=AWS,...:mybucket`).

```go
import (
    _ "github.com/nuln/sbox/rclone"
    _ "github.com/rclone/rclone/backend/s3"
)
```

## Development

The project includes a `Makefile` for standard development tasks:

```bash
make all      # Run fmt, tidy, lint and test
make test     # Run all tests
make lint     # Run static analysis
make coverage # Generate coverage report
```

## License

Apache License 2.0
