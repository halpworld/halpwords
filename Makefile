# Halpwords build targets. Run `make help` for a list.

APP     := halpwords
PKG     := ./cmd/halpwords
DIST    := dist
# The version comes from the latest git tag, such as v1.0.0.
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
# VERSION_NUM is the version as plain numbers, for app bundles and .exe
# files: v1.2.3-4-gabcdef becomes 1.2.3, and anything else 0.0.0.
VERSION_NUM := $(shell echo "$(VERSION)" | sed -E -n 's/^v?([0-9]+\.[0-9]+\.[0-9]+).*/\1/p')
ifeq ($(VERSION_NUM),)
VERSION_NUM := 0.0.0
endif
LDFLAGS := -s -w -X github.com/halpworld/halpwords/internal/game.Version=$(VERSION)
GOBUILD := go build -trimpath -ldflags "$(LDFLAGS)"
# Windows builds are GUI programs, with no console window.
WINBUILD := go build -trimpath -ldflags "$(LDFLAGS) -H=windowsgui"
RELEASE := $(DIST)/release
# MACOS_SIGN_IDENTITY signs the Mac app. "-" signs it just for this Mac;
# a "Developer ID Application: ..." identity signs it for everyone.
MACOS_SIGN_IDENTITY ?= -

.PHONY: help run test vet check sounds balance icon build build-mac build-mac-intel build-mac-universal \
	bundle-mac winres build-windows build-windows-arm64 build-linux build-web serve-web \
	release-mac release-windows release-linux release-web clean

help: ## Show this help
	@grep -E '^[a-z0-9-]+:.*## ' $(MAKEFILE_LIST) | awk -F':.*## ' '{printf "  %-20s %s\n", $$1, $$2}'

run: ## Run the game
	go run $(PKG)

test: ## Run unit tests
	go test ./...

vet: ## Run go vet and check formatting
	go vet ./...
	@test -z "$$(gofmt -l .)" || (gofmt -l . && echo "run gofmt -w ." && exit 1)

check: vet test ## Run every check CI runs

sounds: ## Write every sound effect and piece of music to dist/sounds as WAV files
	go run ./tools/sfxdump -o $(DIST)/sounds

balance: ## Play the dungeon with typing bots and print the balance tables
	go run ./tools/balance

icon: ## Draw the icon: PNGs, a macOS .icns and a Windows .ico in dist/icon
	go run ./tools/icon -o $(DIST)/icon

build: ## Build for this computer
	$(GOBUILD) -o $(DIST)/$(APP) $(PKG)

build-mac: ## Build for Apple Silicon Macs (works from any OS)
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 $(GOBUILD) -o $(DIST)/mac-arm64/$(APP) $(PKG)

build-mac-intel: ## Build for Intel Macs (works from any OS)
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 $(GOBUILD) -o $(DIST)/mac-amd64/$(APP) $(PKG)

build-mac-universal: build-mac build-mac-intel ## Build one binary for every Mac (needs macOS for lipo)
	mkdir -p $(DIST)/mac-universal
	lipo -create -output $(DIST)/mac-universal/$(APP) $(DIST)/mac-arm64/$(APP) $(DIST)/mac-amd64/$(APP)

bundle-mac: build-mac-universal icon ## Build Halpwords.app for every Mac (needs macOS)
	rm -rf $(DIST)/Halpwords.app
	mkdir -p $(DIST)/Halpwords.app/Contents/MacOS $(DIST)/Halpwords.app/Contents/Resources
	cp $(DIST)/mac-universal/$(APP) $(DIST)/Halpwords.app/Contents/MacOS/$(APP)
	cp $(DIST)/icon/halpwords.icns $(DIST)/Halpwords.app/Contents/Resources/
	sed -e 's/@VERSION@/$(VERSION_NUM)/g' build/macos/Info.plist > $(DIST)/Halpwords.app/Contents/Info.plist
	codesign --force --deep --options runtime $(if $(filter -,$(MACOS_SIGN_IDENTITY)),,--timestamp) \
		--sign "$(MACOS_SIGN_IDENTITY)" $(DIST)/Halpwords.app
	@echo "Built $(DIST)/Halpwords.app"

# winres puts the icon and version into Windows builds. It needs the
# network the first time, to fetch go-winres.
winres: icon
	cd cmd/halpwords && go run github.com/tc-hib/go-winres@v0.3.3 simply \
		--arch amd64,arm64 --icon ../../$(DIST)/icon/halpwords.ico --manifest gui \
		--product-name Halpwords --file-description Halpwords --original-filename $(APP).exe \
		--copyright "The Halpwords authors" --product-version $(VERSION_NUM).0 --file-version $(VERSION_NUM).0

build-windows: winres ## Build for 64-bit Windows (works from any OS)
	GOOS=windows GOARCH=amd64 $(WINBUILD) -o $(DIST)/windows-amd64/$(APP).exe $(PKG)

build-windows-arm64: winres ## Build for Windows on Arm (works from any OS)
	GOOS=windows GOARCH=arm64 $(WINBUILD) -o $(DIST)/windows-arm64/$(APP).exe $(PKG)

build-linux: ## Build for Linux (run on Linux: needs cgo and X11/GL dev packages)
	GOOS=linux GOARCH=amd64 $(GOBUILD) -o $(DIST)/linux-amd64/$(APP) $(PKG)

build-web: icon ## Build the WebAssembly version into dist/web
	mkdir -p $(DIST)/web
	GOOS=js GOARCH=wasm $(GOBUILD) -o $(DIST)/web/$(APP).wasm $(PKG)
	cp "$$(go env GOROOT)/lib/wasm/wasm_exec.js" $(DIST)/web/
	cp $(DIST)/icon/favicon.png $(DIST)/icon/halpwords.png $(DIST)/web/
	sed -e 's/@VERSION@/$(VERSION)/g' -e "s/@WASM_SIZE@/$$(wc -c < $(DIST)/web/$(APP).wasm | tr -d ' ')/g" \
		build/web/index.html > $(DIST)/web/index.html
	@echo "Built $(DIST)/web. Try it with: make serve-web"

serve-web: build-web ## Build the web version and serve it at http://localhost:8000
	python3 -m http.server -d $(DIST)/web 8000

# The release archives. Each target packs builds that already exist.
release-mac: bundle-mac ## Zip Halpwords.app into dist/release (needs macOS)
	mkdir -p $(RELEASE)
	cd $(DIST) && ditto -c -k --keepParent Halpwords.app release/Halpwords-$(VERSION)-macos.zip

release-windows: build-windows build-windows-arm64 ## Zip the Windows builds into dist/release
	mkdir -p $(RELEASE)
	cp LICENSE $(DIST)/windows-amd64/LICENSE.txt && cp LICENSE $(DIST)/windows-arm64/LICENSE.txt
	cd $(DIST)/windows-amd64 && zip -q -r ../release/Halpwords-$(VERSION)-windows-amd64.zip .
	cd $(DIST)/windows-arm64 && zip -q -r ../release/Halpwords-$(VERSION)-windows-arm64.zip .

release-linux: build-linux icon ## Pack the Linux build into dist/release (run on Linux)
	mkdir -p $(RELEASE)
	cp LICENSE $(DIST)/icon/halpwords-256.png $(DIST)/linux-amd64/
	tar -C $(DIST)/linux-amd64 -czf $(RELEASE)/Halpwords-$(VERSION)-linux-amd64.tar.gz .

release-web: build-web ## Zip the web version into dist/release
	mkdir -p $(RELEASE)
	cd $(DIST)/web && zip -q -r ../release/Halpwords-$(VERSION)-web.zip .

clean: ## Remove build output
	rm -rf $(DIST) cmd/halpwords/rsrc_windows_*.syso
