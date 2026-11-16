<#
.SYNOPSIS
    Builds pacenotch for Windows into dist/: the CLI, the GUI, or both.

.DESCRIPTION
    The PowerShell counterpart of scripts/build-cli.sh and scripts/build-gui.sh, with the
    same flags and output names as the CI builds:

      dist/pacenotch_windows_<arch>.exe      CLI only, pure Go (no cgo)
      dist/pacenotch-gui_windows_<arch>.exe  tray icon and window (cgo: needs gcc in PATH)

    One executable can't cleanly be both a console and a GUI app, so there are two.
    Works in Windows PowerShell 5.1 and PowerShell 7.

.PARAMETER Target
    cli, gui or all (default).

.PARAMETER Version
    The version baked into the binaries. Default: $env:VERSION, else `git describe`,
    else "dev".

.EXAMPLE
    .\scripts\build-windows.ps1

.EXAMPLE
    .\scripts\build-windows.ps1 -Target cli -Version v1.2.3
#>
[CmdletBinding()]
param(
    [ValidateSet('cli', 'gui', 'all')]
    [string]$Target = 'all',
    [string]$Version
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

# Runs a native command with some environment variables set, restoring them afterwards,
# and fails the script when the command fails.
function Invoke-WithEnv([hashtable]$Vars, [scriptblock]$Command) {
    $saved = @{}
    foreach ($name in $Vars.Keys) {
        $saved[$name] = [Environment]::GetEnvironmentVariable($name, 'Process')
        [Environment]::SetEnvironmentVariable($name, $Vars[$name], 'Process')
    }
    try {
        & $Command
        if ($LASTEXITCODE -ne 0) { throw "command failed with exit code $LASTEXITCODE" }
    } finally {
        foreach ($name in $saved.Keys) {
            [Environment]::SetEnvironmentVariable($name, $saved[$name], 'Process')
        }
    }
}

function Assert-Command([string]$Name, [string]$Hint) {
    if (-not (Get-Command $Name -ErrorAction SilentlyContinue)) {
        throw "build-windows: '$Name' not found in PATH. $Hint"
    }
}

$root = Split-Path -Parent $PSScriptRoot
Push-Location $root
try {
    Assert-Command 'go' 'Install Go 1.27 or newer from https://go.dev/dl/.'

    if (-not $Version) { $Version = $env:VERSION }
    if (-not $Version -and (Get-Command 'git' -ErrorAction SilentlyContinue)) {
        $Version = git describe --tags --always --dirty 2>$null
        if ($LASTEXITCODE -ne 0) { $Version = $null }
    }
    if (-not $Version) { $Version = 'dev' }
    $ldflags = "-s -w -X main.version=$Version"
    $arch = go env GOARCH
    if ($LASTEXITCODE -ne 0 -or -not $arch) { throw 'build-windows: `go env GOARCH` failed.' }
    New-Item -ItemType Directory -Force -Path 'dist' | Out-Null

    if ($Target -in 'cli', 'all') {
        $out = "dist/pacenotch_windows_$arch.exe"
        Invoke-WithEnv @{ CGO_ENABLED = '0'; GOOS = 'windows'; GOARCH = $arch } {
            go build -trimpath -ldflags $ldflags -o $out ./cmd/pacenotch
        }
        Write-Host "built $out"
    }

    if ($Target -in 'gui', 'all') {
        if ([Environment]::OSVersion.Platform -ne 'Win32NT') {
            throw 'build-windows: the GUI build uses cgo, run it on Windows (the CLI cross-compiles anywhere).'
        }
        Assert-Command 'gcc' 'The GUI build needs a C compiler, for example MinGW-w64 (https://www.mingw-w64.org/) or MSYS2.'
        $out = "dist/pacenotch-gui_windows_$arch.exe"
        # -H=windowsgui: no console window when the GUI starts
        Invoke-WithEnv @{ CGO_ENABLED = '1' } {
            go build -tags gui -trimpath -ldflags "$ldflags -H=windowsgui" -o $out ./cmd/pacenotch-gui
        }
        Write-Host "built $out"
    }
} finally {
    Pop-Location
}
