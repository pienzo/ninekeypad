# Runs the project-local Go toolchain with every folder Go would write to redirected into .tools\,
# so building never writes to C:. Usage:  scripts\go.ps1 build ./...   (any go arguments)
$ErrorActionPreference = 'Stop'
$root  = Split-Path -Parent $PSScriptRoot
$tools = Join-Path $root '.tools'
$go    = Join-Path $tools 'go\bin\go.exe'
if (-not (Test-Path $go)) { throw "Go not found at $go - see README.md" }

$dirs = @{
    GOPATH       = 'gopath'
    GOMODCACHE   = 'gopath\pkg\mod'
    GOCACHE      = 'cache\go-build'
    GOTMPDIR     = 'tmp'
    TMP          = 'tmp'
    TEMP         = 'tmp'
    APPDATA      = 'appdata'       # Go telemetry and the go env file live under %APPDATA%\go
    LOCALAPPDATA = 'localappdata'
}
foreach ($name in $dirs.Keys) {
    $path = Join-Path $tools $dirs[$name]
    New-Item -ItemType Directory -Force $path | Out-Null
    Set-Item "env:$name" $path
}
$env:GOTOOLCHAIN = 'local'      # never download another toolchain
$env:GOTELEMETRY = 'off'
$env:CGO_ENABLED = '0'          # pure Go: static binaries, Linux build made on Windows
$env:GOFLAGS     = '-modcacherw'
$env:PATH        = (Join-Path $tools 'go\bin') + ';' + $env:PATH

& $go @args
exit $LASTEXITCODE
