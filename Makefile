BIN      := bin/agenttik
PKG      := ./app/cmd/agenttik
UI       := web
E2E      := e2e
DIST     := dist

# Wails needs to know which webkit2gtk is installed. 4.1 is the current one;
# older distros still ship 4.0.
WEBKIT_TAG := $(shell pkg-config --exists webkit2gtk-4.1 && echo webkit2_41)
DESKTOP_TAGS := desktop production $(WEBKIT_TAG)

# VERSION is what --version reports. A vX.Y.Z tag on HEAD gives its X.Y.Z,
# dropping any suffix the tag carries; anything else gives the short commit.
# Override it with `make build VERSION=1.2.3`.
VERSION ?= $(shell git describe --tags --exact-match 2>/dev/null | \
	sed -n 's/^v\([0-9][0-9]*\.[0-9][0-9]*\.[0-9][0-9]*\).*/\1/p')
ifeq ($(strip $(VERSION)),)
VERSION := $(shell git rev-parse --short=12 HEAD 2>/dev/null || echo dev)
endif
VERSION_LDFLAGS := -X main.version=$(VERSION)

.PHONY: all build build-web build-windows-amd64 build-macos-arm64 run run-web test test-startup e2e vet fmt clean deps hooks ui ui-dev

all: build

## ui: compile the Vue UI into web/dist, which the binary embeds (needs node)
ui:
	cd $(UI) && npm install --no-audit --no-fund && npm run build

## ui-dev: vite with hot reload on :5173, proxying the API to `make run-web`
ui-dev:
	cd $(UI) && npm install --no-audit --no-fund && npm run dev

## build: desktop app (needs node, libwebkit2gtk-4.1-dev, libgtk-3-dev, gcc)
build: ui
	go build -ldflags "$(VERSION_LDFLAGS)" -tags "$(DESKTOP_TAGS)" -o $(BIN) $(PKG)

## build-web: web server only, no cgo and no system dependencies beyond node
build-web: ui
	CGO_ENABLED=0 go build -ldflags "$(VERSION_LDFLAGS)" -o $(BIN)-web $(PKG)

## build-windows-amd64: Windows x64 desktop binary, with the embedded UI
build-windows-amd64: ui
	mkdir -p $(DIST)
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "-H=windowsgui $(VERSION_LDFLAGS)" -tags "desktop production" -o $(DIST)/agenttik_windows_amd64.exe $(PKG)

## build-macos-arm64: macOS ARM64 desktop binary (run this target on macOS)
## Wails calls UTType for the file dialog filters, so the linker needs
## UniformTypeIdentifiers. The wails CLI adds it; plain `go build` does not.
build-macos-arm64: ui
	mkdir -p $(DIST)
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=1 CGO_LDFLAGS="-framework UniformTypeIdentifiers" go build -trimpath -ldflags "$(VERSION_LDFLAGS)" -tags "desktop production" -o $(DIST)/agenttik_darwin_arm64 $(PKG)

run: build
	./$(BIN)

run-web: build-web
	./$(BIN)-web --web

test:
	go test ./...
	cd $(UI) && npm test

## e2e: browser tests against a real server. The first run downloads a
## chromium into ~/.cache/ms-playwright.
e2e: build-web
	cd $(E2E) && npm install --no-audit --no-fund && npx playwright install chromium && npx playwright test

## test-startup: hard one-second browser budget against the built web binary.
## Run after build-web and installing e2e dependencies plus Chromium.
test-startup:
	cd $(E2E) && npx playwright test tests/startup.spec.js --workers=1

vet:
	go vet ./...

fmt:
	gofmt -l -w .

clean:
	rm -rf bin $(DIST) $(E2E)/test-results $(E2E)/playwright-report
	find web/dist -mindepth 1 ! -name .gitkeep -delete

## hooks: point git at .githooks, which rejects Co-Authored-By trailers.
## Config is per clone, so every clone runs this once.
hooks:
	git config core.hooksPath .githooks

## deps: system packages the desktop build needs on Debian/Ubuntu. The UI
## build needs node 20 or newer, which is not installed from here.
deps:
	sudo apt-get install -y build-essential pkg-config libgtk-3-dev libwebkit2gtk-4.1-dev
