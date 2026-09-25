# Tests, then builds both portable programs into dist\:
#   dist\ninekeypad-windows\ninekeypad.exe   and   dist\ninekeypad-linux\ninekeypad
# Each folder is complete: copy it anywhere (USB stick, second disk) and start the program.
$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
$go   = Join-Path $PSScriptRoot 'go.ps1'
Set-Location $root

& $go fmt ./...   # never call gofmt.exe directly: it writes usage counters to %APPDATA%
& $go vet ./...
if ($LASTEXITCODE) { throw 'go vet failed' }
& $go test ./...
if ($LASTEXITCODE) { throw 'tests failed' }

$targets = @(
    @{ os = 'windows'; out = 'dist\ninekeypad-windows\ninekeypad.exe' },
    @{ os = 'linux';   out = 'dist\ninekeypad-linux\ninekeypad' }
)
foreach ($t in $targets) {
    $env:GOOS = $t.os; $env:GOARCH = 'amd64'
    & $go build -trimpath -ldflags '-s -w' -o $t.out ./cmd/ninekeypad
    if ($LASTEXITCODE) { throw "build for $($t.os) failed" }
    $dir = Split-Path -Parent $t.out
    Copy-Item (Join-Path $root "docs\README-$($t.os).txt") (Join-Path $dir 'README.txt') -Force
    Copy-Item (Join-Path $root 'LICENSE') (Join-Path $dir 'LICENSE.txt') -Force
    $size = [math]::Round((Get-Item $t.out).Length / 1MB, 1)
    "built $($t.out)  ($size MB)"
}
Remove-Item env:GOOS, env:GOARCH
