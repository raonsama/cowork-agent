## ─────────────────────────────────────────────────────────────────────────────
## CoworkAgent — Makefile
## Senior Ghost Developer build system
## ─────────────────────────────────────────────────────────────────────────────

BINARY    := cowork
BINPATH   := ./bin
CMD       := ./cmd/cowork
VERSION   := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT    := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_AT  := $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
LDFLAGS   := -s -w \
             -X main.version=$(VERSION) \
             -X main.commit=$(COMMIT) \
             -X main.buildAt=$(BUILD_AT)

# Target platform (override for cross-compile)
GOOS      ?= $(shell go env GOOS)
GOARCH    ?= $(shell go env GOARCH)

# Installation paths
PREFIX    ?= $(HOME)/.local
BINDIR    := $(PREFIX)/bin
LUADIR    := $(HOME)/.config/nvim/lua

## ─────────────────────────────────────────────────────────────────────────────
## Build targets
## ─────────────────────────────────────────────────────────────────────────────

.PHONY: all build build-android clean install uninstall test lint fmt deps

all: build

## Build for the current platform
build:
	@echo "  →  Building $(BINARY) $(VERSION) ($(GOOS)/$(GOARCH))"
	@CGO_ENABLED=1 GOOS=$(GOOS) GOARCH=$(GOARCH) \
		go build -trimpath -ldflags "$(LDFLAGS)" -o $(BINPATH)/$(BINARY) $(CMD)
	@echo "  ✅  $(BINARY) ready"

## Build for Android ARM64 (Termux)
build-android:
	@echo "  →  Building for Android ARM64 (Termux)"
	@CGO_ENABLED=1 \
	 GOOS=android \
	 GOARCH=arm64 \
	 CC=aarch64-linux-android-clang \
	 go build -trimpath -ldflags "$(LDFLAGS)" -o $(BINPATH)/$(BINARY)-android $(CMD)
	@echo "  ✅  $(BINARY)-android ready"

## Build a stripped, compressed binary for Termux (requires upx)
build-termux: build
	@echo "  →  Compressing with upx (for Termux)"
	@upx --best --lzma $(BINPATH)/$(BINARY) 2>/dev/null || echo "  ⚠  upx not found — skipping compression"

## ─────────────────────────────────────────────────────────────────────────────
## Dev targets
## ─────────────────────────────────────────────────────────────────────────────

## Run with hot args (e.g. make run TASK="add unit tests to handler.go")
run: build
	$(BINPATH)/$(BINARY)

run-cowork: build
	$(BINPATH)/$(BINARY) cowork "$(TASK)"

run-index: build
	$(BINPATH)/$(BINARY) index .

## ─────────────────────────────────────────────────────────────────────────────
## Code quality
## ─────────────────────────────────────────────────────────────────────────────

test:
	@echo "  →  Running tests"
	@go test ./... -race -timeout 60s

lint:
	@echo "  →  Linting"
	@which golangci-lint > /dev/null 2>&1 || \
		(echo "  ⚠  golangci-lint not found — run: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest" && exit 1)
	@golangci-lint run ./...

fmt:
	@echo "  →  Formatting"
	@gofmt -w .
	@goimports -w . 2>/dev/null || true

vet:
	@go vet ./...

fix:
	@go fix ./...

## ─────────────────────────────────────────────────────────────────────────────
## Dependencies
## ─────────────────────────────────────────────────────────────────────────────

deps:
	@echo "  →  Tidying modules"
	@go mod tidy
	@go mod download

## ─────────────────────────────────────────────────────────────────────────────
## Install / uninstall
## ─────────────────────────────────────────────────────────────────────────────

install: build
	@echo "  →  Installing to $(BINDIR)/$(BINARY)"
	@mkdir -p $(BINDIR)
	@cp $(BINPATH)/$(BINARY) $(BINDIR)/$(BINARY)
	@chmod +x $(BINDIR)/$(BINARY)
	@echo "  →  Installing LazyVim plugin to $(LUADIR)"
	@mkdir -p $(LUADIR)/cowork-agent
	@cp ./plugin/lazyvim/cowork-agent-module.lua $(LUADIR)/cowork-agent/cowork.lua
	@cp ./plugin/lazyvim/cowork-agent-plugin.lua $(LUADIR)/plugins/cowork.lua
	@echo "  ✅  Installed. Add $(BINDIR) to PATH if needed."

uninstall:
	@echo "  →  Removing $(BINDIR)/$(BINARY)"
	@rm -f $(BINDIR)/$(BINARY)
	@echo "  →  Removing LazyVim plugin"
	@rm -f $(LUADIR)/cowork-agent.lua
	@echo "  ✅  Uninstalled."

## ─────────────────────────────────────────────────────────────────────────────
## Maintenance
## ─────────────────────────────────────────────────────────────────────────────

clean:
	@rm -f $(BINPATH)/$(BINARY) $(BINPATH)/$(BINARY)-android
	@echo "  ✅  Cleaned."

## Print project structure
tree:
	@find . -not -path '*/.git/*' -not -path '*/vendor/*' \
		| sort | sed 's|[^/]*/|  |g'

## Show version info
version:
	@echo "Binary  : $(BINARY)"
	@echo "Version : $(VERSION)"
	@echo "Commit  : $(COMMIT)"
	@echo "Built   : $(BUILD_AT)"
	@echo "Platform: $(GOOS)/$(GOARCH)"

## Help
help:
	@echo ""
	@echo "  CoworkAgent — Build targets"
	@echo ""
	@echo "  make build           Build for current platform"
	@echo "  make build-android   Cross-compile for Android ARM64 (Termux)"
	@echo "  make build-termux    Build + compress with upx"
	@echo "  make run             Build and launch interactive TUI"
	@echo "  make run-cowork TASK='...'  Build and run cowork mode"
	@echo "  make run-index       Build and index current directory"
	@echo "  make install         Install binary + LazyVim plugin"
	@echo "  make uninstall       Remove installed files"
	@echo "  make test            Run test suite"
	@echo "  make lint            Run golangci-lint"
	@echo "  make fmt             Format source code"
	@echo "  make deps            Tidy go modules"
	@echo "  make clean           Remove build artifacts"
	@echo "  make version         Show version info"
	@echo ""
