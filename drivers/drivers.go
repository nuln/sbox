// Package drivers is a convenience package that registers all built-in
// storage drivers. Import it with a blank identifier to make all drivers
// available:
//
//	import _ "github.com/nuln/sbox/drivers"
package drivers

import (
	"github.com/nuln/sbox"
	_ "github.com/nuln/sbox/driver/local"
	_ "github.com/nuln/sbox/driver/rclone"
	_ "github.com/nuln/sbox/driver/sharded"
)

// Init ensures all built-in drivers are registered.
// This is called automatically by importing the package.
func Init() {}

// List returns a list of all registered storage drivers.
func List() []string {
	return sbox.List()
}
