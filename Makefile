SHELL := /bin/bash

REMOTE ?= origin
BRANCH ?= custom/main
REF ?=
SERVICE ?= story-creation

.PHONY: check check-deploy-scope deploy rollback install-service restart status logs

check:
	./scripts/test-classify-deploy-scope.sh
	cd backend && go test ./...
	cd web && bun install --frozen-lockfile && bun run test && bun run build

check-deploy-scope:
	./scripts/test-classify-deploy-scope.sh

deploy:
	REMOTE="$(REMOTE)" BRANCH="$(BRANCH)" DEPLOY_REF="$(REF)" SERVICE="$(SERVICE)" ./scripts/deploy-bare-metal.sh

rollback:
	@test -n "$(REF)" || { echo "Usage: make rollback REF=<commit-sha>" >&2; exit 2; }
	REMOTE="$(REMOTE)" BRANCH="$(BRANCH)" DEPLOY_REF="$(REF)" SERVICE="$(SERVICE)" ./scripts/deploy-bare-metal.sh

install-service:
	SERVICE="$(SERVICE)" ./scripts/install-systemd.sh

restart:
	systemctl restart "$(SERVICE)"

status:
	SERVICE="$(SERVICE)" ./scripts/service-control.sh status

logs:
	SERVICE="$(SERVICE)" ./scripts/service-control.sh logs
