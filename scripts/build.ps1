$ErrorActionPreference = 'Stop'
$previousCGO = $env:CGO_ENABLED
$previousOS = $env:GOOS
$previousArch = $env:GOARCH
Push-Location (Join-Path $PSScriptRoot '..')
try {
    $env:CGO_ENABLED = '0'
    $env:GOOS = 'linux'
    $env:GOARCH = 'amd64'
    go build -trimpath -o bin/robo-juridico ./cmd/server
    if ($LASTEXITCODE -ne 0) { throw 'Build falhou' }
} finally {
    $env:GOOS = $previousOS
    $env:GOARCH = $previousArch
    $env:CGO_ENABLED = $previousCGO
    Pop-Location
}
