package sbox

import (
	"fmt"
	"sort"
	"sync"
)

// Config holds the storage engine configuration.
type Config struct {
	// Type is the driver name: "local", "sharded", "rclone", etc.
	Type string `json:"type" yaml:"type"`

	// BasePath is the root directory for file-based storage engines.
	BasePath string `json:"basePath,omitempty" yaml:"basePath,omitempty"`

	// Options holds driver-specific configuration.
	Options map[string]any `json:"options,omitempty" yaml:"options,omitempty"`
}

// Factory is a function that creates a [StorageEngine] from a [Config].
type Factory func(cfg *Config) (StorageEngine, error)

var (
	mu        sync.RWMutex
	factories = make(map[string]Factory)
)

// Register makes a storage driver available by the provided name.
// This is typically called from the driver package's init() function.
// It panics if called twice with the same name.
func Register(name string, factory Factory) {
	mu.Lock()
	defer mu.Unlock()

	if _, exists := factories[name]; exists {
		panic(fmt.Sprintf("sbox: driver %q already registered", name))
	}
	factories[name] = factory
}

// Drivers returns a sorted list of all registered driver names.
func Drivers() []string {
	mu.RLock()
	defer mu.RUnlock()

	names := make([]string, 0, len(factories))
	for name := range factories {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// List is an alias for [Drivers].
func List() []string {
	return Drivers()
}

// Open creates a new [StorageEngine] using the registered driver specified in cfg.Type.
func Open(cfg *Config) (StorageEngine, error) {
	if cfg == nil {
		return nil, fmt.Errorf("sbox: config must not be nil")
	}

	mu.RLock()
	factory, ok := factories[cfg.Type]
	mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("sbox: unknown driver %q (forgotten import?)", cfg.Type)
	}

	return factory(cfg)
}

// MustOpen is like [Open] but panics on error.
func MustOpen(cfg *Config) StorageEngine {
	engine, err := Open(cfg)
	if err != nil {
		panic(err)
	}
	return engine
}
