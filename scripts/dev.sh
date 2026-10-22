#!/bin/bash
# Dev mode, run by `make dev`:
#   - the UI on Vite (http://localhost:5173), with hot module reload
#   - the API on 127.0.0.1:8722 with a throwaway data dir, rebuilt and
#     restarted whenever a .go file changes
# 8722, not the default 8721, so it doesn't collide with an installed app.
# No watcher dependency: it polls file times once a second.
#
# `make dev-real` (--real) runs the same thing on the real data directory, for
# the user's own debugging. It backs the data up first: dev code migrates and
# writes that database. Coding agents never run it (CLAUDE.md).
set -u
cd "$(dirname "$0")/.."

export HEALTH_TRAIL_DEV_PORT=${HEALTH_TRAIL_DEV_PORT:-8722}
PORT=$HEALTH_TRAIL_DEV_PORT
# A second dev run would leave its API unbound while its Vite proxies to the
# first one, showing the other run's data. Refuse to start instead.
if busy=$(lsof -nP -iTCP:"$PORT" -sTCP:LISTEN 2>/dev/null | tail -n +2) && [ -n "$busy" ]; then
  echo "[dev] 端口 $PORT 已被占用，可能另一个 make dev / make dev-real 还开着。先在那个终端按 Ctrl+C 再试："
  echo "$busy"
  exit 1
fi
BIN=$(mktemp -d)/health-trail
if [ "${1:-}" = --real ]; then
  DATA=${HEALTH_TRAIL_DATA:-${HEALTH_VAULT_DATA:-$HOME/Library/Application Support/HealthVault}}
  if [ -f "$DATA/health.db" ]; then
    backups="$DATA-dev-backups"
    dest="$backups/$(date +%Y%m%d-%H%M%S)"
    mkdir -p "$dest" || exit 1
    # .backup takes a consistent copy even while the app is writing
    sqlite3 "$DATA/health.db" ".backup '$dest/health.db'" || { echo "[dev] 备份失败，未启动"; exit 1; }
    [ -d "$DATA/files" ] && cp -R "$DATA/files" "$dest/"
    echo "[dev] 已备份真实数据到 $dest"
    # keep the latest 10
    ls -1d "$backups"/*/ | sort -r | tail -n +11 | while read -r old; do rm -rf "$old"; done
  fi
  echo "[dev] 使用真实数据：$DATA"
else
  DATA=${HEALTH_TRAIL_DEV_DATA:-/tmp/health-trail-dev}
fi
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
