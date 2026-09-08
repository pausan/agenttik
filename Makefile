BIN      := bin/agenttik
PKG      := ./cmd/agenttik

# Wails needs to know which webkit2gtk is installed. 4.1 is the current one;
# older distros still ship 4.0.
WEBKIT_TAG := $(shell pkg-config --exists webkit2gtk-4.1 && echo webkit2_41)
DESKTOP_TAGS := desktop production $(WEBKIT_TAG)

.PHONY: all build build-web run run-web test vet fmt clean deps

all: build

## build: desktop app (needs libwebkit2gtk-4.1-dev, libgtk-3-dev, gcc)
build:
	go build -tags "$(DESKTOP_TAGS)" -o $(BIN) $(PKG)

## build-web: web server only, no cgo and no system dependencies
build-web:
	CGO_ENABLED=0 go build -o $(BIN)-web $(PKG)

run: build
	./$(BIN)

run-web: build-web
	./$(BIN)-web --web

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -l -w .

clean:
	rm -rf bin

## deps: system packages the desktop build needs on Debian/Ubuntu
deps:
	sudo apt-get install -y build-essential pkg-config libgtk-3-dev libwebkit2gtk-4.1-dev
