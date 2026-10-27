# health-trail build
#
#   make            build the UI and the binary (./health-trail)
#   make app        bundle 健康足迹.app (ad-hoc signed)
#   make test       Go tests + UI typecheck
#   make appstore-test  健康足迹（App Store 测试版）.app: the App Store sandbox, its own bundle id and data
#   make dmg        dist/HealthTrail-<version>.dmg for other Macs: universal, Developer ID signed
#   make notarize   make dmg, then have Apple notarize it so it opens without a warning,
#                   and zip the app for the in-app update (dist/HealthTrail-<version>.zip)
#   make dev        UI on Vite :5173 (hot reload) + API on :8722 on the shared test data,
#                   rebuilt and restarted when a .go file changes (scripts/dev.sh)
#   make test-window  the app's window on the shared test data, port 8732
#   make dev-real   the same on your real data, backed up first (never for tests)

export PATH := /opt/homebrew/bin:$(PATH)
# the Wails/AppKit objects target macOS 13; link for the same
export CGO_CFLAGS  := -mmacosx-version-min=13.0
export CGO_LDFLAGS := -mmacosx-version-min=13.0
export MACOSX_DEPLOYMENT_TARGET := 13.0

APP     := 健康足迹.app
TEST_APP := 健康足迹（App Store 测试版）.app
# a tag v1.2.0 gives 1.2.0
VERSION := $(patsubst v%,%,$(shell git describe --tags --always 2>/dev/null || echo 0.1.0))
# your own signing identity goes in local.mk (git-ignored; see local.mk.example).
# Without one, DEV_ID=- signs ad hoc: fine for trying the steps, not for sharing.
# CI finds the identity in the certificate it imports.
-include local.mk
DEV_ID  ?= -
# The shared test data: made-up records, kept across restarts, used by make dev,
# make test-window and every coding session that tries a change by hand. Never
# the real data in ~/Library/Application Support/HealthVault (CLAUDE.md).
TEST_DATA ?= $(HOME)/Library/Application Support/HealthTrail-Test
# saved once with: xcrun notarytool store-credentials health-trail --team-id <your team id>;
# CI passes an App Store Connect API key instead (--key … --key-id … --issuer …)
NOTARY_AUTH ?= --keychain-profile health-trail
DIST    := dist
DMG     := $(DIST)/HealthTrail-$(VERSION).dmg
# what the app downloads to update itself (internal/update)
ZIP     := $(DIST)/HealthTrail-$(VERSION).zip

.PHONY: all ui bin app appstore-test dmg notarize test test-window dev dev-real clean

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

# A local stand-in for the App Store build: the same sandbox
# (build/appstore.entitlements), ad-hoc signed, with a bundle id of its own so
# its data container, permissions and calendars stay apart from the real app.
appstore-test: bin
	rm -rf "$(TEST_APP)"
	mkdir -p "$(TEST_APP)/Contents/MacOS" "$(TEST_APP)/Contents/Resources"
	cp health-trail "$(TEST_APP)/Contents/MacOS/health-trail"
	sed -e 's/__VERSION__/$(VERSION)/g' -e 's#<string>com.chunbin1.healthtrail</string>#<string>com.chunbin1.healthtrail.appstoretest</string>#' \
		-e 's#<string>健康足迹</string>#<string>健康足迹测试版</string>#g' build/Info.plist > "$(TEST_APP)/Contents/Info.plist"
	cp build/AppIcon.icns "$(TEST_APP)/Contents/Resources/"
	codesign --force --options runtime --entitlements build/appstore.entitlements --sign - "$(TEST_APP)"
	@echo "built $(TEST_APP) — data: ~/Library/Containers/com.chunbin1.healthtrail.appstoretest"

# one binary for Apple silicon and Intel; the hardened runtime and a secure
# timestamp are what notarizing asks for
dmg: ui
	rm -rf "$(DIST)" && mkdir -p "$(DIST)/root/$(APP)/Contents/MacOS" "$(DIST)/root/$(APP)/Contents/Resources"
	GOARCH=arm64 go build -ldflags "-s -w -X main.version=$(VERSION)" -o "$(DIST)/health-trail-arm64" .
	GOARCH=amd64 CGO_ENABLED=1 CC="clang -arch x86_64" go build -ldflags "-s -w -X main.version=$(VERSION)" -o "$(DIST)/health-trail-amd64" .
	lipo -create -output "$(DIST)/root/$(APP)/Contents/MacOS/health-trail" "$(DIST)/health-trail-arm64" "$(DIST)/health-trail-amd64"
	sed 's/__VERSION__/$(VERSION)/g' build/Info.plist > "$(DIST)/root/$(APP)/Contents/Info.plist"
	cp build/AppIcon.icns "$(DIST)/root/$(APP)/Contents/Resources/"
	codesign --force --options runtime --timestamp$(if $(filter -,$(DEV_ID)),=none) \
		--entitlements build/developerid.entitlements --sign "$(DEV_ID)" "$(DIST)/root/$(APP)"
	codesign --verify --strict "$(DIST)/root/$(APP)"
	ln -s /Applications "$(DIST)/root/Applications"
	hdiutil create -volname 健康足迹 -srcfolder "$(DIST)/root" -fs HFS+ -format UDZO -ov "$(DMG)"
	codesign --force --sign "$(DEV_ID)" "$(DMG)"
	@echo "built $(DMG)"

# stapling fetches the ticket from Apple and sometimes loses the connection
# (v1.4.1: "The network connection was lost"): try a few times
STAPLE = staple() { for i in 1 2 3 4 5; do xcrun stapler staple "$$1" && return; [ $$i = 5 ] || sleep 20; done; return 1; }; staple

notarize: dmg
	xcrun notarytool submit "$(DMG)" $(NOTARY_AUTH) --wait
	$(STAPLE) "$(DMG)"
	spctl --assess --type open --context context:primary-signature -v "$(DMG)"
	# the in-app update installs the same app, with its ticket stapled on
	$(STAPLE) "$(DIST)/root/$(APP)"
	spctl --assess --type execute -v "$(DIST)/root/$(APP)"
	ditto -c -k --keepParent "$(DIST)/root/$(APP)" "$(ZIP)"

test: frontend/node_modules
	go vet ./...
	go test ./...
	python3 -I scripts/test_ai_review_gate.py
	cd frontend && npx tsc -b

# the binary embeds frontend/dist, so it has to exist once before the first build
# a test window on the shared test data; it is not the real app, so it sends
# no system notifications and syncs to 「健康足迹（测试）」 calendars
test-window: bin
	./health-trail -data "$(TEST_DATA)" -port 8732

dev: frontend/node_modules
	@test -d frontend/dist || $(MAKE) ui
	@./scripts/dev.sh

dev-real: frontend/node_modules
	@test -d frontend/dist || $(MAKE) ui
	@./scripts/dev.sh --real

clean:
	rm -rf health-trail "$(APP)" "$(TEST_APP)" "$(DIST)" frontend/dist
