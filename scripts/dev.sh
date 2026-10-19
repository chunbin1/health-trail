#!/bin/bash
# Dev mode, run by `make dev`:
#   - the UI on Vite (http://localhost:5173), with hot module reload
#   - the API on 127.0.0.1:8722 with a throwaway data dir, rebuilt and
#     restarted whenever a .go file changes
# 8722, not the default 8721, so it never talks to an installed app that is
# running on the real data. No watcher dependency: it polls file times once a
# second.
set -u
cd "$(dirname "$0")/.."

PORT=8722
DATA=${HEALTH_VAULT_DEV_DATA:-/tmp/health-vault-dev}
BIN=$(mktemp -d)/health-vault
api=
vite=

stop() {
  [ -n "$api" ] && kill "$api" 2>/dev/null && wait "$api" 2>/dev/null
  api=
}

cleanup() {
  stop
  [ -n "$vite" ] && kill "$vite" 2>/dev/null
  rm -rf "$(dirname "$BIN")"
  exit 0
}
trap cleanup INT TERM EXIT

# a fingerprint of every Go source file (tests excluded) plus go.mod
fingerprint() {
  find . -path ./frontend -prune -o -path ./.git -prune -o \
    \( -name '*.go' ! -name '*_test.go' -o -name go.mod \) -print0 |
    xargs -0 stat -f '%m %N' | sort | shasum
}

restart() {
  stop
  echo "[dev] 编译后端…"
  if go build -o "$BIN" .; then
    "$BIN" serve -data "$DATA" -port "$PORT" &
    api=$!
  else
    echo "[dev] 编译失败，改好后保存会自动重试"
  fi
}

(cd frontend && exec ./node_modules/.bin/vite) &
vite=$!

last=$(fingerprint)
restart
while sleep 1; do
  now=$(fingerprint)
  if [ "$now" != "$last" ]; then
    last=$now
    restart
  fi
done
