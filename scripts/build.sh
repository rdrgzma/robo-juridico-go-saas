#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
docker compose exec -T app sh -c 'CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -o /workspace/bin/robo-juridico ./cmd/server'
