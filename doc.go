// Package sbox provides a unified storage abstraction layer for Go.
//
// It defines a generic [StorageEngine] interface that can be backed by
// different storage backends through a driver registration mechanism.
//
// # Supported Drivers
//
//   - local   — Local filesystem via afero (import _ "github.com/nuln/sbox/local")
//   - sharded — Content-addressed chunked storage (import _ "github.com/nuln/sbox/sharded")
//   - rclone  — Any rclone-supported remote (import _ "github.com/nuln/sbox/rclone")
//
// # Quick Start
//
//	import (
//	    "github.com/nuln/sbox"
//	    _ "github.com/nuln/sbox/local"
//	)
//
//	engine, err := sbox.Open(&sbox.Config{Type: "local", BasePath: "./data"})
//
// # Import All Drivers
//
//	import _ "github.com/nuln/sbox/drivers"
package sbox
