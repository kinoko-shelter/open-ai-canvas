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
    if [[ -L "$PROJECT_DIR/.local/current" ]]; then
      current_release="$(readlink -f "$PROJECT_DIR/.local/current")"
      printf 'frontend commit: '
      if [[ -s "$current_release/web-commit" ]]; then
        tr -d '\r\n' < "$current_release/web-commit"
        printf '\n'
      else
        echo unknown
      fi
      printf 'backend commit: '
      if [[ -s "$current_release/backend-commit" ]]; then
        tr -d '\r\n' < "$current_release/backend-commit"
        printf '\n'
      else
        echo unknown
      fi
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
