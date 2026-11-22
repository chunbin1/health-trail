#!/usr/bin/env bash
set -euo pipefail

# Build HealthTrailCore.xcframework for iOS (device + simulator) using gomobile.
# Usage: scripts/build_ios_core.sh [--build-ui]

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT_DIR"

if [[ -z "${DEVELOPER_DIR:-}" ]]; then
  CURRENT_DEV_DIR="$(xcode-select -p 2>/dev/null || true)"
  if [[ "$CURRENT_DEV_DIR" != *Xcode*.app* && -d "/Applications/Xcode.app/Contents/Developer" ]]; then
    export DEVELOPER_DIR="/Applications/Xcode.app/Contents/Developer"
  fi
fi
GOPATH_BIN="$(go env GOPATH)/bin"
export PATH="$GOPATH_BIN:/opt/homebrew/bin:$PATH"

BUILD_UI=0
for arg in "$@"; do
  case "$arg" in
    --build-ui) BUILD_UI=1 ;;
    *) echo "Unknown argument: $arg" >&2; exit 1 ;;
  esac
done

if [[ "$BUILD_UI" == "1" || ! -f "$ROOT_DIR/frontend/dist/index.html" ]]; then
  echo "==> Building frontend UI..."
  (cd "$ROOT_DIR/frontend" && npm run build)
fi

if ! command -v gomobile >/dev/null 2>&1; then
  echo "==> Installing gomobile and gobind..."
  go install golang.org/x/mobile/cmd/gomobile@latest
  go install golang.org/x/mobile/cmd/gobind@latest
fi

VERSION="$(git describe --tags --always 2>/dev/null | sed 's/^v//' || echo "1.0.0")"
OUT_DIR="$ROOT_DIR/ios/Frameworks"
OUT_XCFRAMEWORK="$OUT_DIR/HealthTrailCore.xcframework"
mkdir -p "$OUT_DIR"
rm -rf "$OUT_XCFRAMEWORK"

echo "==> Building HealthTrailCore.xcframework (version: $VERSION, target: ios,iossimulator)..."
gomobile bind \
  -v \
  -target=ios,iossimulator \
  -iosversion=16.0 \
  -ldflags "-s -w -X github.com/lee/health-trail/mobile.Version=$VERSION" \
  -o "$OUT_XCFRAMEWORK" \
  ./mobile

echo "==> Built: $OUT_XCFRAMEWORK"
ls -la "$OUT_XCFRAMEWORK"
