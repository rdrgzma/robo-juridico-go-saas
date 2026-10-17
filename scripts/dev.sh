#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
case "${1:-up}" in
  init) docker run --rm -e APP_ENV=development --mount "type=bind,source=$PWD,target=/workspace" -w /workspace golang:1.27.2-bookworm go run ./cmd/devinit ;;
  extend) docker run --rm -e APP_ENV=development --mount "type=bind,source=$PWD,target=/workspace" -w /workspace golang:1.27.2-bookworm go run ./cmd/devinit extend ;;
  bootstrap) docker compose exec -T app go run ./cmd/identity bootstrap ;;
  outbox) docker compose exec -T app go run ./cmd/identity outbox ;;
  up) docker compose up -d --build ;;
  down) docker compose down ;;
  test) docker compose exec -T app go test -race ./... ;;
  vet) docker compose exec -T app go vet ./... ;;
  migrate) docker compose exec -T app go run ./cmd/migrate up ;;
  status) docker compose ps ;;
  logs) docker compose logs -f app ;;
  build) docker build -f docker/production/Dockerfile -t robo-juridico-go-saas:local . ;;
  *) echo 'Use init|extend|bootstrap|outbox|up|down|test|vet|migrate|status|logs|build' >&2; exit 2 ;;
esac
