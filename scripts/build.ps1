$ErrorActionPreference='Stop'
Push-Location (Join-Path $PSScriptRoot '..')
try {
    docker compose exec -T app sh -c 'CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -o /workspace/bin/robo-juridico ./cmd/server'
    if ($LASTEXITCODE -ne 0) {throw 'Build em contêiner falhou'}
} finally {Pop-Location}
