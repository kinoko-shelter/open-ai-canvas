#!/usr/bin/env bash
set -Eeuo pipefail

PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SERVICE="${SERVICE:-story-creation}"
ACTION="${1:-status}"

case "$ACTION" in
  status)
    printf 'deployed commit: '
    if [[ -f "$PROJECT_DIR/.local/deployed-commit" ]]; then
      tr -d '\n' < "$PROJECT_DIR/.local/deployed-commit"
      printf '\n'
    else
      echo unknown
    fi
    systemctl --no-pager --full status "$SERVICE" || true
    printf 'backend health: '
    curl -fsS http://127.0.0.1:8080/api/health && printf '\n'
    printf 'public health: '
    curl -fsS http://127.0.0.1:3000/api/health && printf '\n'
    ;;
  logs)
    exec journalctl -u "$SERVICE" -f -n 200
    ;;
  *)
    echo "Usage: $0 {status|logs}" >&2
    exit 2
    ;;
esac
