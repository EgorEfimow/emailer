.PHONY: build test lint fmt tidy clean run

# Binary name
BINARY_NAME := emailer

# Go parameters
GOCMD := go
GOBUILD := $(GOCMD) build
GOTEST := $(GOCMD) test
GOLINT := golangci-lint
GOFMT := $(GOCMD) fmt
GOIMports := $(GOCMD) imports

# Build the binary
build:
	$(GOBUILD) -o bin/$(BINARY_NAME) ./cmd/emailer

# Run all tests
test:
	$(GOTEST) -v -race ./...

# Run linter
lint:
	$(GOLINT) run ./...

# Format code
fmt:
	$(GOFMT) ./...

# Tidy go modules
tidy:
	$(GOCMD) mod tidy

# Clean build artifacts
clean:
	rm -rf bin/
	rm -f coverage.out

# Run the binary (for development)
run: build
	./bin/$(BINARY_NAME)