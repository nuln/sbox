.DEFAULT_GOAL := help

## all: Run fmt, tidy, lint and test
.PHONY: all
all: fmt tidy lint test

## help: Show this help message
.PHONY: help
help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## //' | column -t -s ':'

## build: Build the project
.PHONY: build
build:
	go build ./...

## test: Run all tests
.PHONY: test
test:
	go test ./... -v -race

## lint: Run golangci-lint
.PHONY: lint
lint:
	golangci-lint run ./...

## fmt: Format code
.PHONY: fmt
fmt:
	gofmt -s -w .
	goimports -local github.com/nuln/sbox -w . || true

## tidy: Tidy go modules
.PHONY: tidy
tidy:
	go mod tidy

## coverage: Generate test coverage report
.PHONY: coverage
coverage:
	go test -coverprofile=coverage.txt -covermode=atomic ./...
	go tool cover -html=coverage.txt -o coverage.html

## clean: Clean build artifacts
.PHONY: clean
clean:
	rm -f coverage.txt coverage.html
	go clean -testcache
