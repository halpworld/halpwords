# Halpwords build targets. Run `make help` for a list.

APP     := halpwords
PKG     := ./cmd/halpwords
DIST    := dist
VERSION ?= 0.1.0

.PHONY: help run test vet sounds build build-mac build-mac-intel bundle-mac build-windows build-linux build-web clean

help: ## Show this help
	@grep -E '^[a-z-]+:.*## ' $(MAKEFILE_LIST) | awk -F':.*## ' '{printf "  %-16s %s\n", $$1, $$2}'

run: ## Run the game
	go run $(PKG)

test: ## Run unit tests
	go test ./...

vet: ## Run go vet and check formatting
	go vet ./...
	@test -z "$$(gofmt -l .)" || (gofmt -l . && echo "run gofmt -w ." && exit 1)

sounds: ## Write every sound effect to dist/sounds as WAV files
	go run ./tools/sfxdump -o $(DIST)/sounds

build: ## Build for this computer
	go build -o $(DIST)/$(APP) $(PKG)

build-mac: ## Build for Apple Silicon Macs (works from any OS)
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -o $(DIST)/mac-arm64/$(APP) $(PKG)

build-mac-intel: ## Build for Intel Macs (works from any OS)
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -o $(DIST)/mac-amd64/$(APP) $(PKG)

bundle-mac: build-mac ## Build Halpwords.app for Apple Silicon
	rm -rf $(DIST)/Halpwords.app
	mkdir -p $(DIST)/Halpwords.app/Contents/MacOS
	cp $(DIST)/mac-arm64/$(APP) $(DIST)/Halpwords.app/Contents/MacOS/$(APP)
	sed 's/@VERSION@/$(VERSION)/g' build/macos/Info.plist > $(DIST)/Halpwords.app/Contents/Info.plist
	@echo "Built $(DIST)/Halpwords.app"

build-windows: ## Build for 64-bit Windows (works from any OS)
	GOOS=windows GOARCH=amd64 go build -ldflags "-H=windowsgui" -o $(DIST)/windows-amd64/$(APP).exe $(PKG)

build-linux: ## Build for Linux (run on Linux: needs cgo and X11/GL dev packages)
	GOOS=linux GOARCH=amd64 go build -o $(DIST)/linux-amd64/$(APP) $(PKG)

build-web: ## Build the WebAssembly version into dist/web
	mkdir -p $(DIST)/web
	GOOS=js GOARCH=wasm go build -o $(DIST)/web/$(APP).wasm $(PKG)
	cp "$$(go env GOROOT)/lib/wasm/wasm_exec.js" $(DIST)/web/
	cp build/web/index.html $(DIST)/web/
	@echo "Serve dist/web with any static web server, e.g. python3 -m http.server -d dist/web"

clean: ## Remove build output
	rm -rf $(DIST)
