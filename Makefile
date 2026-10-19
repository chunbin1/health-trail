# health-vault build
#
#   make            build the UI and the binary (./health-vault)
#   make app        bundle 健康档案.app (ad-hoc signed)
#   make test       Go tests + UI typecheck
#   make dev        API on :8721 with a throwaway data dir; run `npm run dev` in frontend/ for the UI

export PATH := /opt/homebrew/bin:$(PATH)
# the Wails/AppKit objects target macOS 13; link for the same
export CGO_CFLAGS  := -mmacosx-version-min=13.0
export CGO_LDFLAGS := -mmacosx-version-min=13.0
export MACOSX_DEPLOYMENT_TARGET := 13.0

APP     := 健康档案.app
VERSION := $(shell git describe --tags --always 2>/dev/null || echo 0.1.0)

.PHONY: all ui bin app test dev clean

all: bin

frontend/node_modules: frontend/package.json
	cd frontend && npm ci && touch node_modules

ui: frontend/node_modules
	cd frontend && npm run build

bin: ui
	go build -ldflags "-s -w -X main.version=$(VERSION)" -o health-vault .

app: bin
	rm -rf "$(APP)"
	mkdir -p "$(APP)/Contents/MacOS" "$(APP)/Contents/Resources"
	cp health-vault "$(APP)/Contents/MacOS/health-vault"
	sed 's/__VERSION__/$(VERSION)/g' build/Info.plist > "$(APP)/Contents/Info.plist"
	cp build/AppIcon.icns "$(APP)/Contents/Resources/AppIcon.icns" 2>/dev/null || true
	codesign --force --deep --sign - "$(APP)"
	@echo "built $(APP) — drag it to /Applications"

test: frontend/node_modules
	go vet ./...
	go test ./...
	cd frontend && npx tsc -b

dev:
	go run . serve -data /tmp/health-vault-dev

clean:
	rm -rf health-vault "$(APP)" frontend/dist
