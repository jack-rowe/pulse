BINARY_NAME=pulse
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_FLAGS=-ldflags "-s -w -X main.version=$(VERSION)"

.PHONY: build run test lint clean release docker init

## Build for current platform
build:
	go build $(BUILD_FLAGS) -o $(BINARY_NAME) .

## Run locally
run: build
	./$(BINARY_NAME) --config config.yaml

## Generate default config
init: build
	./$(BINARY_NAME) --init

## Validate config
validate: build
	./$(BINARY_NAME) --validate --config config.yaml

## Run tests
test:
	go test ./... -v -race

## Lint (requires golangci-lint)
lint:
	golangci-lint run ./...

## Clean build artifacts
clean:
	rm -f $(BINARY_NAME)
	rm -f $(BINARY_NAME)-*

## Cross-compile for all platforms
release: clean
	GOOS=linux   GOARCH=amd64 go build $(BUILD_FLAGS) -o $(BINARY_NAME)-linux-amd64 .
	GOOS=linux   GOARCH=arm64 go build $(BUILD_FLAGS) -o $(BINARY_NAME)-linux-arm64 .
	GOOS=darwin  GOARCH=amd64 go build $(BUILD_FLAGS) -o $(BINARY_NAME)-darwin-amd64 .
	GOOS=darwin  GOARCH=arm64 go build $(BUILD_FLAGS) -o $(BINARY_NAME)-darwin-arm64 .
	GOOS=windows GOARCH=amd64 go build $(BUILD_FLAGS) -o $(BINARY_NAME)-windows-amd64.exe .

## Build Docker image
docker:
	docker build -t pulse:$(VERSION) .

## Show help
help:
	@grep -E '^## ' Makefile | sed 's/## //'
