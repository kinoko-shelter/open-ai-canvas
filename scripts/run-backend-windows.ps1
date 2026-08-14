$ErrorActionPreference = "Stop"

$root = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$backend = Join-Path $root "backend"
$envFile = Join-Path $root ".env"

if (-not (Test-Path -LiteralPath $envFile)) {
    throw "Missing environment file: $envFile"
}

Get-Content -LiteralPath $envFile | ForEach-Object {
    $line = $_.Trim()
    if ($line -and -not $line.StartsWith("#") -and $line -match "^([^=]+)=(.*)$") {
        $name = $matches[1].Trim()
        $value = $matches[2].Trim()
        [Environment]::SetEnvironmentVariable($name, $value, "Process")
    }
}

# The checked-in deployment .env uses Linux paths. Keep those values for
# services, but use Windows paths for the Go tool and local backend data.
$localRoot = Join-Path $root ".local"
$env:CANVAS_BACKEND_DATA_DIR = Join-Path $localRoot "project-workbench-debug"
$env:GOCACHE = Join-Path $localRoot "cache\go-build"
$env:GOMODCACHE = Join-Path $localRoot "cache\go-mod"

New-Item -ItemType Directory -Force -Path `
    $env:CANVAS_BACKEND_DATA_DIR, $env:GOCACHE, $env:GOMODCACHE | Out-Null

Push-Location $backend
try {
    go run -buildvcs=false ./cmd/server
}
finally {
    Pop-Location
}
