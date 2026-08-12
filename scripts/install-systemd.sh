#!/usr/bin/env bash
set -Eeuo pipefail

PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SERVICE="${SERVICE:-story-creation}"
UNIT_SOURCE="${UNIT_SOURCE:-$PROJECT_DIR/deploy/systemd/story-creation.service}"
UNIT_TARGET="/etc/systemd/system/${SERVICE}.service"

if [[ ${EUID} -ne 0 ]]; then
  echo "Run this command as root: sudo make install-service" >&2
  exit 1
fi
command -v systemctl >/dev/null || { echo "systemd is required" >&2; exit 1; }
[[ -f "$PROJECT_DIR/.local/server.env" ]] || { echo "Missing $PROJECT_DIR/.local/server.env" >&2; exit 1; }
[[ -x "$PROJECT_DIR/.local/current/backend" ]] || { echo "Deploy the application before installing the service" >&2; exit 1; }
[[ -f "$UNIT_SOURCE" ]] || { echo "Missing systemd unit: $UNIT_SOURCE" >&2; exit 1; }

install -m 0644 "$UNIT_SOURCE" "$UNIT_TARGET"
systemctl daemon-reload
systemctl enable --now "$SERVICE"
systemctl is-active --quiet "$SERVICE"
