BINARY_NAME=chloe
VERSION?=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS=-s -w -X main.version=$(VERSION)

.PHONY: build build-all test lint fmt install install-system release release-dry clean deps deps-update help

## Build

build: ## Build for current platform
	go build -ldflags "$(LDFLAGS)" -o $(BINARY_NAME) .

build-all: ## Build for all platforms
	GOOS=linux   GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o build/$(BINARY_NAME)-linux-amd64 .
	GOOS=linux   GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o build/$(BINARY_NAME)-linux-arm64 .
	GOOS=darwin  GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o build/$(BINARY_NAME)-darwin-amd64 .
	GOOS=darwin  GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o build/$(BINARY_NAME)-darwin-arm64 .

## Test & Quality

test: ## Run tests with race detection
	CGO_ENABLED=1 go test -v -race ./...

test-coverage: ## Generate coverage report
	CGO_ENABLED=1 go test -race -coverprofile=coverage.out -covermode=atomic ./...
	go tool cover -func=coverage.out

test-coverage-html: test-coverage ## Open coverage in browser
	go tool cover -html=coverage.out

lint: ## Run linter
	golangci-lint run ./...

fmt: ## Format code
	goimports -w .

## Install

install: build ## Install to ~/.local/bin
	@mkdir -p $(HOME)/.local/bin
	cp $(BINARY_NAME) $(HOME)/.local/bin/$(BINARY_NAME)
	@echo "Installed to $(HOME)/.local/bin/$(BINARY_NAME)"

install-system: build ## Install to /usr/local/bin (requires sudo)
	sudo cp $(BINARY_NAME) /usr/local/bin/$(BINARY_NAME)
	@echo "Installed to /usr/local/bin/$(BINARY_NAME)"

## Release

release: ## Create release binaries
	goreleaser release --clean

release-dry: ## Test release locally (dry run)
	goreleaser release --snapshot --skip=publish --clean

## Dependencies

deps: ## Download and tidy modules
	go mod download
	go mod tidy

deps-update: ## Update all dependencies
	go get -u ./...
	go mod tidy

## Misc

clean: ## Remove build artifacts
	rm -rf $(BINARY_NAME) build/ dist/ coverage.out

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

.DEFAULT_GOAL := help
