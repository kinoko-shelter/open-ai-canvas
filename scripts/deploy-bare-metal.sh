#!/usr/bin/env bash
set -Eeuo pipefail

PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
REMOTE="${REMOTE:-origin}"
BRANCH="${BRANCH:-custom/main}"
DEPLOY_REF="${DEPLOY_REF:-}"
SERVICE="${SERVICE:-story-creation}"
LOCAL_DIR="$PROJECT_DIR/.local"
RELEASES_DIR="$LOCAL_DIR/releases"
CURRENT_LINK="$LOCAL_DIR/current"
ENV_FILE="$LOCAL_DIR/server.env"
LOCK_FILE="$LOCAL_DIR/deploy.lock"
UNIT_TARGET="/etc/systemd/system/${SERVICE}.service"
BACKEND_HEALTH_URL="${BACKEND_HEALTH_URL:-http://127.0.0.1:8080/api/health}"
APP_HEALTH_URL="${APP_HEALTH_URL:-http://127.0.0.1:3000/api/health}"
APP_HOME_URL="${APP_HOME_URL:-http://127.0.0.1:3000/}"

if [[ ${EUID} -ne 0 ]]; then
  echo "Run this command as root: sudo make deploy" >&2
  exit 1
fi
for command_name in git bun go curl systemctl flock tar; do
  command -v "$command_name" >/dev/null || { echo "Missing required command: $command_name" >&2; exit 1; }
done
[[ -f "$ENV_FILE" ]] || { echo "Missing $ENV_FILE" >&2; exit 1; }
set -a
# shellcheck disable=SC1090
source "$ENV_FILE"
set +a

mkdir -p "$LOCAL_DIR" "$RELEASES_DIR"
exec 9>"$LOCK_FILE"
flock -n 9 || { echo "Another deployment is already running" >&2; exit 1; }

cd "$PROJECT_DIR"
[[ "$(git branch --show-current)" == "$BRANCH" ]] || { echo "Server checkout must be on $BRANCH" >&2; exit 1; }
git diff --quiet && git diff --cached --quiet || { echo "Tracked files have local changes; deployment stopped" >&2; exit 1; }

echo "Fetching $REMOTE/$BRANCH"
git fetch "$REMOTE" "$BRANCH"
if [[ -n "$DEPLOY_REF" ]]; then
  TARGET_COMMIT="$(git rev-parse --verify "${DEPLOY_REF}^{commit}")"
else
  TARGET_COMMIT="$(git rev-parse --verify "$REMOTE/$BRANCH^{commit}")"
fi
git merge-base --is-ancestor "$TARGET_COMMIT" "$REMOTE/$BRANCH" || { echo "Deployment ref must belong to $REMOTE/$BRANCH" >&2; exit 1; }
if [[ -z "$DEPLOY_REF" ]]; then
  git merge --ff-only "$TARGET_COMMIT"
fi
SHORT_COMMIT="$(git rev-parse --short=12 "$TARGET_COMMIT")"
BUILD_DIR="$(mktemp -d "$LOCAL_DIR/build.${SHORT_COMMIT}.XXXXXX")"
SOURCE_DIR="$BUILD_DIR/source"
RELEASE_DIR="$RELEASES_DIR/${SHORT_COMMIT}-$(date -u +%Y%m%dT%H%M%SZ)"
OLD_CURRENT=""
LEGACY_TMUX=0
SWITCHED=0
WEB_MIGRATED=0
FAILURE_NOTIFIED=0
UNIT_HAD_PREVIOUS=0
UNIT_CHANGED=0
UNIT_BACKUP="$BUILD_DIR/systemd-unit.backup"

json_escape() {
  local value="$1"
  value="${value//\\/\\\\}"
  value="${value//\"/\\\"}"
  value="${value//$'\n'/\\n}"
  printf '%s' "$value"
}

notify_feishu() {
  local message="$1"
  [[ -n "${FEISHU_DEPLOY_WEBHOOK:-}" ]] || return 0
  local payload
  payload="$(printf '{\"msg_type\":\"text\",\"content\":{\"text\":\"%s\"}}' "$(json_escape "$message")")"
  if ! curl -fsS --max-time 10 -H 'Content-Type: application/json' --data-binary "$payload" "$FEISHU_DEPLOY_WEBHOOK" >/dev/null; then
    echo "Warning: Feishu deployment notification failed" >&2
  fi
}

cleanup() {
  rm -rf "$BUILD_DIR"
}

cleanup_and_report() {
  local exit_code=$?
  cleanup
  if [[ $exit_code -ne 0 && $FAILURE_NOTIFIED -eq 0 ]]; then
    notify_feishu "[故事创作] 部署失败\n主机: $(hostname)\n分支: $BRANCH\n提交: $SHORT_COMMIT\n线上版本未切换"
  fi
  return "$exit_code"
}

rollback_current_deploy() {
  local exit_code=$?
  trap - ERR INT TERM
  if [[ $UNIT_CHANGED -eq 1 ]]; then
    if [[ $UNIT_HAD_PREVIOUS -eq 1 ]]; then
      install -m 0644 "$UNIT_BACKUP" "$UNIT_TARGET"
    else
      systemctl stop "$SERVICE" || true
      rm -f "$UNIT_TARGET"
    fi
    systemctl daemon-reload || true
  fi
  if [[ $SWITCHED -eq 1 ]]; then
    echo "Deployment failed; restoring the previous application"
    if [[ -n "$OLD_CURRENT" && -d "$OLD_CURRENT" ]]; then
      ln -sfn "$OLD_CURRENT" "$LOCAL_DIR/current.rollback"
      mv -Tf "$LOCAL_DIR/current.rollback" "$CURRENT_LINK"
      systemctl restart "$SERVICE" || true
    elif [[ $LEGACY_TMUX -eq 1 ]]; then
      systemctl stop "$SERVICE" || true
      tmux new-session -d -s open-ai-canvas-backend "cd '$PROJECT_DIR/backend' && set -a && . '$ENV_FILE' && set +a && go run ./cmd/server 2>&1 | tee -a '$LOCAL_DIR/logs/backend.log'" || true
    else
      rm -f "$CURRENT_LINK"
    fi
  fi
  if [[ $WEB_MIGRATED -eq 1 && ( -e "$PROJECT_DIR/web/dist.pre-systemd" || -L "$PROJECT_DIR/web/dist.pre-systemd" ) ]]; then
    rm -f "$PROJECT_DIR/web/dist"
    mv "$PROJECT_DIR/web/dist.pre-systemd" "$PROJECT_DIR/web/dist"
  fi
  rm -rf "$RELEASE_DIR"
  cleanup
  FAILURE_NOTIFIED=1
  notify_feishu "[故事创作] 部署失败，已执行回退\n主机: $(hostname)\n分支: $BRANCH\n提交: $SHORT_COMMIT"
  exit "$exit_code"
}
trap rollback_current_deploy ERR INT TERM
trap cleanup_and_report EXIT

notify_feishu "[故事创作] 开始部署\n主机: $(hostname)\n分支: $BRANCH\n提交: $SHORT_COMMIT"

mkdir -p "$SOURCE_DIR" "$RELEASE_DIR"
git archive "$TARGET_COMMIT" | tar -x -C "$SOURCE_DIR"

echo "Building frontend at $SHORT_COMMIT"
(
  cd "$SOURCE_DIR/web"
  bun install --frozen-lockfile
  bun run build
)
[[ -s "$SOURCE_DIR/web/dist/index.html" ]] || { echo "Frontend build did not produce index.html" >&2; exit 1; }

echo "Building backend at $SHORT_COMMIT"
(
  cd "$SOURCE_DIR/backend"
  CGO_ENABLED=1 go build -trimpath -o "$RELEASE_DIR/backend" ./cmd/server
)
[[ -x "$RELEASE_DIR/backend" ]] || { echo "Backend build did not produce an executable" >&2; exit 1; }
mv "$SOURCE_DIR/web/dist" "$RELEASE_DIR/web-dist"
printf '%s\n' "$TARGET_COMMIT" > "$RELEASE_DIR/commit"

if [[ -L "$CURRENT_LINK" ]]; then
  OLD_CURRENT="$(readlink -f "$CURRENT_LINK")"
fi
if command -v tmux >/dev/null && tmux has-session -t open-ai-canvas-backend 2>/dev/null; then
  LEGACY_TMUX=1
fi

UNIT_SOURCE="$SOURCE_DIR/deploy/systemd/story-creation.service"
if [[ ! -f "$UNIT_SOURCE" ]]; then
  UNIT_SOURCE="$PROJECT_DIR/deploy/systemd/story-creation.service"
fi
[[ -f "$UNIT_SOURCE" ]] || { echo "Missing systemd unit" >&2; exit 1; }
if [[ -f "$UNIT_TARGET" ]]; then
  cp "$UNIT_TARGET" "$UNIT_BACKUP"
  UNIT_HAD_PREVIOUS=1
fi
install -m 0644 "$UNIT_SOURCE" "$UNIT_TARGET"
UNIT_CHANGED=1
systemctl daemon-reload
systemctl enable "$SERVICE" >/dev/null

EXPECTED_WEB_TARGET="../.local/current/web-dist"
CURRENT_WEB_TARGET=""
if [[ -L "$PROJECT_DIR/web/dist" ]]; then
  CURRENT_WEB_TARGET="$(readlink "$PROJECT_DIR/web/dist")"
fi
if [[ "$CURRENT_WEB_TARGET" != "$EXPECTED_WEB_TARGET" ]]; then
  rm -rf "$PROJECT_DIR/web/dist.pre-systemd"
  if [[ -e "$PROJECT_DIR/web/dist" || -L "$PROJECT_DIR/web/dist" ]]; then
    mv "$PROJECT_DIR/web/dist" "$PROJECT_DIR/web/dist.pre-systemd"
  fi
  ln -s "$EXPECTED_WEB_TARGET" "$PROJECT_DIR/web/dist"
  WEB_MIGRATED=1
fi

ln -sfn "$RELEASE_DIR" "$LOCAL_DIR/current.next"
mv -Tf "$LOCAL_DIR/current.next" "$CURRENT_LINK"
SWITCHED=1

if [[ $LEGACY_TMUX -eq 1 ]]; then
  tmux kill-session -t open-ai-canvas-backend
fi
echo "Restarting $SERVICE; active worker tasks may drain before the old process exits"
systemctl restart "$SERVICE"

for _ in $(seq 1 60); do
  if systemctl is-active --quiet "$SERVICE" && curl -fsS "$BACKEND_HEALTH_URL" >/dev/null; then
    break
  fi
  sleep 1
done
systemctl is-active --quiet "$SERVICE"
curl -fsS "$BACKEND_HEALTH_URL" >/dev/null
curl -fsS "$APP_HEALTH_URL" >/dev/null
curl -fsS "$APP_HOME_URL" >/dev/null

printf '%s\n' "$TARGET_COMMIT" > "$LOCAL_DIR/deployed-commit"
printf '%s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" > "$LOCAL_DIR/deployed-at"

WEB_MIGRATED=0
SWITCHED=0
UNIT_CHANGED=0
trap - ERR INT TERM

find "$RELEASES_DIR" -mindepth 1 -maxdepth 1 -type d ! -path "$RELEASE_DIR" -exec rm -rf {} + || true
rm -rf "$PROJECT_DIR/web/dist.pre-systemd" || true

echo "Deployment succeeded"
echo "Commit: $TARGET_COMMIT"
echo "Service: $SERVICE"
notify_feishu "[故事创作] 部署成功\n主机: $(hostname)\n分支: $BRANCH\n提交: $SHORT_COMMIT\n服务: $SERVICE"
