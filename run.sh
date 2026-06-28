#!/usr/bin/env bash
# Chat2API — quick start (localhost:8080, Go)

set -euo pipefail

ROOT="$(cd "$(dirname "$0")" && pwd)"
cd "$ROOT"

if [ -f .env ]; then
  set -a
  # shellcheck disable=SC1091
  source .env
  set +a
  echo "[run.sh] Loaded .env"
fi

export HOST="${HOST:-localhost}"
export PORT="${PORT:-8080}"

if [ -z "${CHATGPT_ACCESS_TOKEN:-}" ]; then
  echo "[run.sh] Warning: CHATGPT_ACCESS_TOKEN is not set."
  echo "         Copy .env.example to .env and add your token."
fi

echo "[run.sh] Starting Chat2API at http://${HOST}:${PORT}"

# ponytail: common user-local Go install paths
for d in "$HOME/go/bin" /usr/local/go/bin; do
  if [ -x "$d/go" ]; then
    export PATH="$d:$PATH"
    break
  fi
done

if command -v go >/dev/null 2>&1; then
  exec go run ./cmd/chat2api
fi

if [ -x ./bin/chat2api ]; then
  exec ./bin/chat2api
fi

echo "[run.sh] Go not found. Build with: go build -o bin/chat2api ./cmd/chat2api"
exit 1