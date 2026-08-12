#!/usr/bin/env bash
set -Eeuo pipefail

frontend_changed=0
backend_changed=0
full_deploy=0

while IFS= read -r path; do
  [[ -n "$path" ]] || continue
  case "$path" in
    *.md|docs/*|.github/*|LICENSE)
      ;;
    web/*)
      frontend_changed=1
      ;;
    backend/*)
      backend_changed=1
      ;;
    Makefile|scripts/*|deploy/*|Dockerfile|docker-compose*.yml|docker-compose*.yaml|compose.yml|compose.yaml|nginx.conf|.env.example)
      full_deploy=1
      ;;
    *)
      full_deploy=1
      ;;
  esac
done

if [[ $full_deploy -eq 1 || ( $frontend_changed -eq 1 && $backend_changed -eq 1 ) ]]; then
  echo full
elif [[ $frontend_changed -eq 1 ]]; then
  echo frontend
elif [[ $backend_changed -eq 1 ]]; then
  echo backend
else
  echo none
fi
