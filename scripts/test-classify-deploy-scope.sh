#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CLASSIFIER="$SCRIPT_DIR/classify-deploy-scope.sh"

assert_scope() {
  local expected="$1"
  shift
  local actual
  actual="$(printf '%s\n' "$@" | "$CLASSIFIER")"
  if [[ "$actual" != "$expected" ]]; then
    echo "Expected '$expected', got '$actual' for: $*" >&2
    exit 1
  fi
}

assert_scope none ""
assert_scope none README.md web/README.md backend/README.md docs/deployment.md .github/workflows/check.yml LICENSE
assert_scope frontend web/src/App.tsx
assert_scope backend backend/internal/server.go
assert_scope full web/src/App.tsx backend/internal/server.go
assert_scope full Makefile
assert_scope full scripts/deploy-bare-metal.sh
assert_scope full deploy/systemd/story-creation.service
assert_scope full Dockerfile docker-compose.yaml compose.yml nginx.conf
assert_scope full bun.lock

echo "Deployment scope classifier tests passed"
