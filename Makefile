BIN      := bin/agenttik
PKG      := ./app/cmd/agenttik
UI       := web

# Wails needs to know which webkit2gtk is installed. 4.1 is the current one;
# older distros still ship 4.0.
WEBKIT_TAG := $(shell pkg-config --exists webkit2gtk-4.1 && echo webkit2_41)
DESKTOP_TAGS := desktop production $(WEBKIT_TAG)

.PHONY: all build build-web run run-web test vet fmt clean deps ui ui-dev

all: build

## ui: compile the Vue UI into web/dist, which the binary embeds (needs node)
ui:
	cd $(UI) && npm install --no-audit --no-fund && npm run build

## ui-dev: vite with hot reload on :5173, proxying the API to `make run-web`
ui-dev:
	cd $(UI) && npm install --no-audit --no-fund && npm run dev

## build: desktop app (needs node, libwebkit2gtk-4.1-dev, libgtk-3-dev, gcc)
build: ui
	go build -tags "$(DESKTOP_TAGS)" -o $(BIN) $(PKG)

## build-web: web server only, no cgo and no system dependencies beyond node
build-web: ui
	CGO_ENABLED=0 go build -o $(BIN)-web $(PKG)

run: build
	./$(BIN)

run-web: build-web
	./$(BIN)-web --web

test:
	go test ./...
	cd $(UI) && npm test

vet:
	go vet ./...

fmt:
	gofmt -l -w .

clean:
	rm -rf bin
	find web/dist -mindepth 1 ! -name .gitkeep -delete

## deps: system packages the desktop build needs on Debian/Ubuntu. The UI
## build needs node 20 or newer, which is not installed from here.
deps:
	sudo apt-get install -y build-essential pkg-config libgtk-3-dev libwebkit2gtk-4.1-dev
