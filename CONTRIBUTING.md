# Contributing

If you're interested in contributing to this project, this is the best place to start. Before contributing to this project, please take a bit of time to read our [Code of Conduct](CODE-OF-CONDUCT.md). Also, note that this project is open-source and licensed under [Apache License 2.0](LICENSE).

## Project Structure

This project is a Go library providing a unified storage abstraction layer.

- [storage.go](storage.go): Main interface definition.
- [types.go](types.go): Core data types and IO interfaces.
- [config.go](config.go): Driver registration and factory.
- [local/](local/), [sharded/](sharded/), [rclone/](rclone/): Storage driver implementations.
- [sboxtest/](sboxtest/): Comprehensive test suite for drivers.

## Development

First prepare the backend environment by downloading all required dependencies:

```bash
go mod download
```

### Running Tests

You can run the full test suite (including all registered drivers) using:

```bash
go test ./... -v
```

## Adding New Drivers

New drivers (e.g., S3, Azure Blob, IPFS) can be added by:
1. Creating a new directory at the project root (e.g., `s3/`).
2. Implementing the `sbox.StorageEngine` interface.
3. Registering the driver in an `init()` function via `sbox.Register`.
4. Adding the driver to `drivers/drivers.go` for convenience.
