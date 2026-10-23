VERSION ?= 1.0.0
APP_NAME := coreip-inspector
BIN_DIR := bin
DIST_DIR := dist

.PHONY: all build build-linux test test-race package-deb package-rpm clean run

all: test build

build:
	@mkdir -p $(BIN_DIR)
	@echo "==> Building native binary for host ($(shell go env GOOS)/$(shell go env GOARCH))..."
	go build -ldflags="-s -w" -o $(BIN_DIR)/$(APP_NAME) ./cmd/inspector

build-linux:
	@mkdir -p $(BIN_DIR)
	@echo "==> Cross-compiling for Linux amd64..."
	GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o $(BIN_DIR)/$(APP_NAME)-linux-amd64 ./cmd/inspector

test:
	@echo "==> Running unit tests..."
	go test -v ./...

test-race:
	@echo "==> Running tests with race detector..."
	go test -race -v ./...

package-deb: build-linux
	@echo "==> Packaging Debian/Ubuntu package (.deb)..."
	go run ./cmd/package-deb

package-rpm: build-linux
	@echo "==> Packaging CentOS/RHEL package (.rpm)..."
	./packaging/rpm/build-rpm.sh

run: build
	@echo "==> Running $(APP_NAME) locally..."
	./$(BIN_DIR)/$(APP_NAME) -config configs/config.yaml

clean:
	@echo "==> Cleaning build artifacts..."
	rm -rf $(BIN_DIR) $(DIST_DIR)

