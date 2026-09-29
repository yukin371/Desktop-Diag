# Compile and run the struct-layout ground-truth probe with MSVC + Windows SDK.
# See README.md for why this tool exists.
#
# Usage:
#   powershell -NoProfile -ExecutionPolicy Bypass -File tools\layout-probe\run.ps1
#   powershell -NoProfile -ExecutionPolicy Bypass -File tools\layout-probe\run.ps1 -KeepOutput

[CmdletBinding()]
param(
    # Write the probe output to this file (relative paths resolve against the repo root).
    [string]$OutFile = 'tools\layout-probe\layout-probe-output.txt',

    # Also copy the compiled .exe/.obj into the repo instead of a temp dir.
    [switch]$KeepBinaries
)

$ErrorActionPreference = 'Stop'

$repoRoot = Split-Path -Parent (Split-Path -Parent $PSScriptRoot)
$srcDir   = $PSScriptRoot
$srcFile  = Join-Path $srcDir 'layout_probe.c'

if (-not (Test-Path $srcFile)) {
    throw "Probe source not found: $srcFile"
}

# ---- locate Visual Studio -------------------------------------------------
$vswhere = Join-Path ${env:ProgramFiles(x86)} 'Microsoft Visual Studio\Installer\vswhere.exe'
if (-not (Test-Path $vswhere)) {
    throw "vswhere.exe not found at '$vswhere'. Install Visual Studio with the C++ workload, or fall back to gcc (see README.md)."
}

# NOTE: vswhere writes to stdout; with $ErrorActionPreference='Stop' a native
# command writing to stderr would become a terminating error, so relax it here.
$prevEAP = $ErrorActionPreference
$ErrorActionPreference = 'Continue'
$vsPath = & $vswhere -latest -products * -requires Microsoft.VisualStudio.Component.VC.Tools.x86.x64 -property installationPath
$vsExit = $LASTEXITCODE
$ErrorActionPreference = $prevEAP

if ($vsExit -ne 0 -or [string]::IsNullOrWhiteSpace($vsPath)) {
    throw "vswhere failed (exit $vsExit) or no VS installation with the C++ toolset was found."
}
$vsPath = $vsPath.Trim()

$vcvars = Join-Path $vsPath 'VC\Auxiliary\Build\vcvars64.bat'
if (-not (Test-Path $vcvars)) {
    throw "vcvars64.bat not found at '$vcvars'."
}
Write-Host "Visual Studio : $vsPath"

# ---- prepare a scratch dir ------------------------------------------------
if ($KeepBinaries) {
    $work = Join-Path $srcDir 'bin'
    if (-not (Test-Path $work)) { New-Item -ItemType Directory -Path $work | Out-Null }
} else {
    $work = Join-Path ([System.IO.Path]::GetTempPath()) ("layout-probe-" + [guid]::NewGuid().ToString('N').Substring(0, 8))
    New-Item -ItemType Directory -Path $work | Out-Null
}
Write-Host "Work dir      : $work"

$exe = Join-Path $work 'layout_probe.exe'

# ---- build a batch driver -------------------------------------------------
# vcvars64.bat is a batch file; it can only be sourced from cmd.exe, so we
# generate a driver rather than trying to import the environment into PowerShell.
$bat = Join-Path $work 'drive.bat'
$batLines = @(
    '@echo off'
    "call `"$vcvars`" >nul 2>&1"
    'if errorlevel 1 ( echo VCVARS_FAILED & exit /b 1 )'
    "cd /d `"$work`""
    # /utf-8: the probe source contains non-ASCII comments; without it MSVC
    # decodes using the ANSI code page and emits C4819.
    "cl /nologo /W3 /O2 /utf-8 /Fe:`"$exe`" `"$srcFile`" iphlpapi.lib ws2_32.lib"
    'if errorlevel 1 ( echo COMPILE_FAILED & exit /b 2 )'
    'echo ==== RUN ===='
    "`"$exe`""
)
Set-Content -Path $bat -Value $batLines -Encoding ASCII

# ---- run ------------------------------------------------------------------
$prevEAP = $ErrorActionPreference
$ErrorActionPreference = 'Continue'
$output = & cmd.exe /c $bat 2>&1
$exit = $LASTEXITCODE
$ErrorActionPreference = $prevEAP

$output | ForEach-Object { Write-Host $_ }

if ($exit -ne 0) {
    throw "layout probe failed with exit code $exit (see output above)."
}

if ($OutFile) {
    if (-not [System.IO.Path]::IsPathRooted($OutFile)) {
        $OutFile = Join-Path $repoRoot $OutFile
    }
    $dir = Split-Path -Parent $OutFile
    if ($dir -and -not (Test-Path $dir)) { New-Item -ItemType Directory -Path $dir | Out-Null }
    Set-Content -Path $OutFile -Value $output -Encoding UTF8
    Write-Host ""
    Write-Host "Output written to: $OutFile"
}

if (-not $KeepBinaries) {
    Remove-Item -Recurse -Force $work -ErrorAction SilentlyContinue
}

Write-Host ""
Write-Host "Now compare the values above against the expectation tables in:"
Write-Host "  internal\winapi\iphlpapi_types_test.go"
