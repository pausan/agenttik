BIN      := bin/agenttik
PKG      := ./app/cmd/agenttik
UI       := web
E2E      := e2e
MOBILE   := mobileapp
DIST     := dist

# Wails needs to know which webkit2gtk is installed. 4.1 is the current one;
# older distros still ship 4.0.
WEBKIT_TAG := $(shell pkg-config --exists webkit2gtk-4.1 && echo webkit2_41)
DESKTOP_TAGS := desktop production $(WEBKIT_TAG)

# Wails calls UTType for the file dialog filters, so on macOS the linker needs
# UniformTypeIdentifiers. The wails CLI adds it; plain `go build` does not.
ifeq ($(shell uname -s),Darwin)
export CGO_LDFLAGS += -framework UniformTypeIdentifiers
endif

# VERSION is what --version reports. A vMAJOR/vX.Y.Z tag on HEAD gives its X.Y.Z,
# dropping any suffix the tag carries; anything else gives the short commit.
# Override it with `make build VERSION=1.2.3`.
VERSION ?= $(shell git describe --tags --exact-match 2>/dev/null | \
	sed -n 's|.*/||; s/^v\([0-9][0-9]*\.[0-9][0-9]*\.[0-9][0-9]*\).*/\1/p')
ifeq ($(strip $(VERSION)),)
VERSION := $(shell git rev-parse --short=12 HEAD 2>/dev/null || echo dev)
endif
VERSION_LDFLAGS := -X main.version=$(VERSION)

# VPX_LIB is the screen encoder library the binary embeds, built from source by
# scripts/build-vpx.sh (needs git, curl, a C compiler and nasm). See specs/096-screen.md.
VPX_LIB := $(firstword $(wildcard app/internal/screen/vpx/lib/$(shell go env GOOS)_$(shell go env GOARCH)/*agenttikvpx*))

.PHONY: all build build-web android ios vpx build-windows-amd64 build-macos-arm64 run run-web test test-startup e2e vet fmt clean deps hooks ui ui-dev

all: build

.PHONY: speech
## speech: build the native Whisper helper; speech weights download on first use
speech:
	bash scripts/build-speech.sh

## ui: compile the Vue UI into web/dist, which the binary embeds (needs node)
ui:
	cd $(UI) && npm install --no-audit --no-fund && npm run build

## ui-dev: vite with hot reload on :5173, proxying the API to `make run-web`
ui-dev:
	cd $(UI) && npm install --no-audit --no-fund && npm run dev

## vpx: build the embedded screen encoder when missing or its shim changes
vpx: $(VPX_LIB)
ifeq ($(VPX_LIB),)
	scripts/build-vpx.sh
else
$(VPX_LIB): app/internal/screen/vpx/shim/agenttik_vpx.c scripts/build-vpx.sh
	scripts/build-vpx.sh
endif

## build: desktop app (needs node, libwebkit2gtk-4.1-dev, libgtk-3-dev, gcc)
build: ui vpx speech
	go build -ldflags "$(VERSION_LDFLAGS)" -tags "$(DESKTOP_TAGS)" -o $(BIN) $(PKG)
ifeq ($(shell uname -s),Darwin)
	bash scripts/package-macos.sh $(BIN) $(BIN).app "$(VERSION)" $(BIN)_darwin.app.zip $(BIN)_darwin.dmg
endif

## android: Android APK in bin/, signed with this machine's debug key (needs node, JDK 21, Android SDK)
android:
	cd $(MOBILE) && npm ci --no-audit --no-fund && npm test && npx cap sync android
	cd $(MOBILE)/android && ./gradlew assembleRelease -PversionName="$(VERSION)"
	mkdir -p bin && cp $(MOBILE)/android/app/build/outputs/apk/release/app-release.apk $(BIN)_android.apk

## ios: unsigned iOS Simulator build in bin/ (needs macOS and Xcode). Device
## builds need signing: run `npx cap open ios` in mobileapp and use Xcode.
ios:
	cd $(MOBILE) && npm ci --no-audit --no-fund && npm test && npx cap sync ios
	xcodebuild -project $(MOBILE)/ios/App/App.xcodeproj -scheme App -configuration Release \
		-sdk iphonesimulator -destination 'generic/platform=iOS Simulator' \
		-derivedDataPath $(MOBILE)/ios/App/build MARKETING_VERSION="$(VERSION)" build
	mkdir -p bin && rm -rf $(BIN)_ios-simulator.app && \
		cp -R $(MOBILE)/ios/App/build/Build/Products/Release-iphonesimulator/App.app $(BIN)_ios-simulator.app

## build-web: web server without cgo; builds the native speech helper with CMake
build-web: ui vpx speech
	CGO_ENABLED=0 go build -ldflags "$(VERSION_LDFLAGS)" -o $(BIN)-web $(PKG)

## build-windows-amd64: Windows x64 desktop binary and ZIP (needs Python 3.11+, MinGW and nasm)
build-windows-amd64: ui
	bash scripts/build-speech.sh windows amd64
	test app/internal/screen/vpx/lib/windows_amd64/agenttikvpx.dll -nt app/internal/screen/vpx/shim/agenttik_vpx.c || scripts/build-vpx.sh windows amd64
	mkdir -p $(DIST)
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "-H=windowsgui $(VERSION_LDFLAGS)" -tags "desktop production" -o $(DIST)/agenttik_windows_amd64.exe $(PKG)
	python3 scripts/package-windows.py $(DIST)/agenttik_windows_amd64.exe

## build-macos-arm64: macOS ARM64 binary, signed .app, ZIP and DMG (run on macOS)
build-macos-arm64: ui
	bash scripts/build-speech.sh darwin arm64
	test app/internal/screen/vpx/lib/darwin_arm64/libagenttikvpx.dylib -nt app/internal/screen/vpx/shim/agenttik_vpx.c || scripts/build-vpx.sh darwin arm64
	mkdir -p $(DIST)
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=1 go build -trimpath -ldflags "$(VERSION_LDFLAGS)" -tags "desktop production" -o $(DIST)/agenttik_darwin_arm64 $(PKG)
	bash scripts/package-macos.sh $(DIST)/agenttik_darwin_arm64 $(DIST)/agenttik.app "$(VERSION)" $(DIST)/agenttik_darwin_arm64.app.zip $(DIST)/agenttik_darwin_arm64.dmg

run: build
	./$(BIN)

run-web: build-web
	./$(BIN)-web --web

test:
	go test ./...
	cd $(UI) && npm test

## test-desktop-remote: native Wails/WebKit password/code login regression (needs Xvfb)
.PHONY: test-desktop-remote
test-desktop-remote:
	xvfb-run -a go test -tags "$(DESKTOP_TAGS)" $(PKG) -run '^TestDesktopRemoteLogin$$' -count=1

## test-desktop-images: native Wails/WebKit image upload regression (needs Xvfb)
.PHONY: test-desktop-images
test-desktop-images:
	xvfb-run -a go test -tags "$(DESKTOP_TAGS)" $(PKG) -run '^TestDesktopImageUpload$$' -count=1

## test-desktop-speech: native microphone API availability, without using hardware
.PHONY: test-desktop-speech
test-desktop-speech:
	xvfb-run -a go test -tags "$(DESKTOP_TAGS)" $(PKG) -run '^TestDesktopMicrophoneAPI$$' -count=1

## test-desktop-html: native Wails/WebKit sandboxed HTML asset regression (needs Xvfb)
.PHONY: test-desktop-html
test-desktop-html:
	xvfb-run -a go test -tags "$(DESKTOP_TAGS)" $(PKG) -run '^TestDesktopHTMLPreview$$' -count=1

## test-desktop-video: native Wails/WebKit video playback through the media bridge (needs Xvfb, GStreamer, ffmpeg)
.PHONY: test-desktop-video
test-desktop-video:
	xvfb-run -a go test -tags "$(DESKTOP_TAGS)" $(PKG) -run '^TestDesktopVideo$$' -count=1

## test-desktop-terminal: native Wails/WebKit terminal input regression (needs Xvfb)
.PHONY: test-desktop-terminal
test-desktop-terminal:
	xvfb-run -a go test -tags "$(DESKTOP_TAGS)" $(PKG) -run '^TestDesktopTerminalInput$$' -count=1

## test-desktop-screen: native Wails/WebKit Screen tab drawing and input (needs Xvfb)
.PHONY: test-desktop-screen
test-desktop-screen:
	xvfb-run -a go test -tags "$(DESKTOP_TAGS)" $(PKG) -run '^TestDesktopScreen$$' -count=1 -v

## test-desktop-scrollbars: native scrollbar paint regression (needs Xvfb, xdotool, ImageMagick)
.PHONY: test-desktop-scrollbars
test-desktop-scrollbars:
	xvfb-run -a go test -tags "$(DESKTOP_TAGS)" $(PKG) -run '^TestDesktopScrollbarLayers$$' -count=1

## e2e: browser tests against a real server. The first run downloads a
## chromium into ~/.cache/ms-playwright. Tests with time budgets run after the
## rest, one at a time (see e2e/playwright.config.js).
e2e: build-web
	cd $(E2E) && npm install --no-audit --no-fund && npx playwright install chromium && \
		{ npx playwright test --project=chromium; status=$$?; npx playwright test --project=timed && exit $$status; }

## test-startup: hard 1.5-second browser budget against the built web binary.
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
	sudo apt-get install -y build-essential cmake pkg-config libgtk-3-dev libwebkit2gtk-4.1-dev
