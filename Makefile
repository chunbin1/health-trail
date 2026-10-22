# health-trail build
#
#   make            build the UI and the binary (./health-trail)
#   make app        bundle 健康足迹.app (ad-hoc signed)
#   make test       Go tests + UI typecheck
#   make dev        UI on Vite :5173 (hot reload) + API on :8722 with a throwaway data dir,
#                   rebuilt and restarted when a .go file changes (scripts/dev.sh)
#   make dev-real   the same on your real data, backed up first (never for tests)

export PATH := /opt/homebrew/bin:$(PATH)
# the Wails/AppKit objects target macOS 13; link for the same
export CGO_CFLAGS  := -mmacosx-version-min=13.0
export CGO_LDFLAGS := -mmacosx-version-min=13.0
export MACOSX_DEPLOYMENT_TARGET := 13.0

APP     := 健康足迹.app
VERSION := $(shell git describe --tags --always 2>/dev/null || echo 0.1.0)

.PHONY: all ui bin app test dev dev-real clean

all: bin

frontend/node_modules: frontend/package.json
	cd frontend && npm ci && touch node_modules

ui: frontend/node_modules
	cd frontend && npm run build

bin: ui
	go build -ldflags "-s -w -X main.version=$(VERSION)" -o health-trail .

app: bin
	rm -rf "$(APP)"
	mkdir -p "$(APP)/Contents/MacOS" "$(APP)/Contents/Resources"
	cp health-trail "$(APP)/Contents/MacOS/health-trail"
	sed 's/__VERSION__/$(VERSION)/g' build/Info.plist > "$(APP)/Contents/Info.plist"
	cp build/AppIcon.icns "$(APP)/Contents/Resources/AppIcon.icns" 2>/dev/null || true
	codesign --force --deep --sign - "$(APP)"
	@echo "built $(APP) — drag it to /Applications"

test: frontend/node_modules
	go vet ./...
	go test ./...
	cd frontend && npx tsc -b

# the binary embeds frontend/dist, so it has to exist once before the first build
dev: frontend/node_modules
	@test -d frontend/dist || $(MAKE) ui
	@./scripts/dev.sh

dev-real: frontend/node_modules
	@test -d frontend/dist || $(MAKE) ui
	@./scripts/dev.sh --real

clean:
	rm -rf health-trail "$(APP)" frontend/dist
