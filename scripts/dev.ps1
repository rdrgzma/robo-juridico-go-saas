param([ValidateSet('init','extend','bootstrap','outbox','up','down','test','vet','migrate','status','logs','build')][string]$Action='up')
$ErrorActionPreference='Stop'
Push-Location (Join-Path $PSScriptRoot '..')
try {
    switch ($Action) {
        'init' { docker run --rm --env APP_ENV=development --mount "type=bind,source=$((Get-Location).Path),target=/workspace" -w /workspace golang:1.27.2-bookworm go run ./cmd/devinit }
        'extend' { docker run --rm --env APP_ENV=development --mount "type=bind,source=$((Get-Location).Path),target=/workspace" -w /workspace golang:1.27.2-bookworm go run ./cmd/devinit extend }
        'bootstrap' { docker compose exec -T app go run ./cmd/identity bootstrap }
        'outbox' { docker compose exec -T app go run ./cmd/identity outbox }
        'up' { docker compose up -d --build }
        'down' { docker compose down }
        'test' { docker compose exec -T app go test -race ./... }
        'vet' { docker compose exec -T app go vet ./... }
        'migrate' { docker compose exec -T app go run ./cmd/migrate up }
        'status' { docker compose ps }
        'logs' { docker compose logs -f app }
        'build' { docker build -f docker/production/Dockerfile -t robo-juridico-go-saas:local . }
    }
    if ($LASTEXITCODE -ne 0) {throw "Operação Docker falhou ($LASTEXITCODE)"}
} finally {Pop-Location}
