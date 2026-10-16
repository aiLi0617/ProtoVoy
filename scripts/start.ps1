[CmdletBinding()]
param(
    [ValidateSet("Dev", "Release")]
    [string]$Mode = "Dev",

    [switch]$SkipInstall
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

function Get-RequiredCommand {
    param([Parameter(Mandatory)][string]$Name)

    $command = Get-Command $Name -ErrorAction SilentlyContinue
    if (-not $command) {
        throw "Required command '$Name' was not found in PATH. See README.md#development for prerequisites."
    }

    return $command.Source
}

function Invoke-CheckedCommand {
    param(
        [Parameter(Mandatory)][string]$FilePath,
        [Parameter(Mandatory)][string[]]$ArgumentList
    )

    Write-Host "> $FilePath $($ArgumentList -join ' ')" -ForegroundColor Cyan
    & $FilePath @ArgumentList
    if ($LASTEXITCODE -ne 0) {
        throw "Command failed with exit code ${LASTEXITCODE}: $FilePath $($ArgumentList -join ' ')"
    }
}

function Get-CheckedCommandOutput {
    param(
        [Parameter(Mandatory)][string]$FilePath,
        [Parameter(Mandatory)][string[]]$ArgumentList
    )

    $output = & $FilePath @ArgumentList
    if ($LASTEXITCODE -ne 0) {
        throw "Command failed with exit code ${LASTEXITCODE}: $FilePath $($ArgumentList -join ' ')"
    }

    return ($output -join [Environment]::NewLine).Trim()
}

$repositoryRoot = Split-Path -Parent $PSScriptRoot

# rustup installs here by default. Add it only to this process when a new shell
# has not picked up rustup's PATH update yet.
$defaultCargoBin = Join-Path $env:USERPROFILE ".cargo\bin"
if (-not (Get-Command "cargo" -ErrorAction SilentlyContinue) -and
    (Test-Path -LiteralPath (Join-Path $defaultCargoBin "cargo.exe") -PathType Leaf)) {
    $env:PATH = "$defaultCargoBin$([IO.Path]::PathSeparator)$env:PATH"
}

$node = Get-RequiredCommand "node"
$pnpm = Get-RequiredCommand "pnpm"
$cargo = Get-RequiredCommand "cargo"
$rustc = Get-RequiredCommand "rustc"

$nodeVersion = (Get-CheckedCommandOutput $node @("--version")).TrimStart("v")
$nodeMajor = [int](($nodeVersion -split '\.')[0])
if ($nodeMajor -lt 22) {
    throw "Node.js 22 or newer is required; found $nodeVersion."
}

$pnpmVersion = Get-CheckedCommandOutput $pnpm @("--version")
$rustVersion = Get-CheckedCommandOutput $rustc @("--version")

Write-Host "ApiVoy bootstrap" -ForegroundColor Green
Write-Host "  Mode:  $Mode"
Write-Host "  Node:  $nodeVersion"
Write-Host "  pnpm:  $pnpmVersion"
Write-Host "  Rust:  $rustVersion"

Push-Location $repositoryRoot
try {
    if (-not $SkipInstall) {
        Invoke-CheckedCommand $pnpm @("install", "--frozen-lockfile")
    }

    if ($Mode -eq "Dev") {
        Write-Host "Compiling and starting the Tauri desktop app..." -ForegroundColor Green
        Write-Host "Keep this terminal open; press Ctrl+C to stop the development process."
        Invoke-CheckedCommand $pnpm @("dev:desktop")
        return
    }

    Write-Host "Building the release desktop app and Local Agent sidecar..." -ForegroundColor Green
    Invoke-CheckedCommand $pnpm @("build:desktop")

    $metadataJson = & $cargo metadata --format-version 1 --no-deps
    if ($LASTEXITCODE -ne 0) {
        throw "Unable to locate Cargo's target directory."
    }
    $metadata = ($metadataJson -join [Environment]::NewLine) | ConvertFrom-Json
    $executableName = if ($env:OS -eq "Windows_NT") { "apivoy.exe" } else { "apivoy" }
    $releaseDirectory = Join-Path $metadata.target_directory "release"
    $executable = Join-Path $releaseDirectory $executableName

    if (-not (Test-Path -LiteralPath $executable -PathType Leaf)) {
        throw "Release build completed, but the executable was not found at: $executable"
    }

    Write-Host "Starting $executable" -ForegroundColor Green
    Start-Process -FilePath $executable -WorkingDirectory $repositoryRoot
}
finally {
    Pop-Location
}
