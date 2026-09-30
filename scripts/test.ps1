# Runs phase-3 format, vet, coverage and optional race checks with one complete Go toolchain.
# No persistent PATH or toolchain settings are modified.
[CmdletBinding()]
param([switch]$Race, [string]$CoverageFile = "dist/coverage.out")

$ErrorActionPreference = "Stop"
$diagRepoRoot = Split-Path -Parent $PSScriptRoot
$diagOriginalPath = $env:PATH
$diagOriginalToolchain = $env:GOTOOLCHAIN

# Assert-NativeSuccess preserves the real exit code rather than relying on filtered test output.
function Assert-NativeSuccess([string]$Step) {
    if ($LASTEXITCODE -ne 0) { throw "$Step failed with exit code $LASTEXITCODE" }
}

Push-Location $diagRepoRoot
try {
    $env:GOTOOLCHAIN = "go1.26.0"
    $diagGoRoot = (& go env GOROOT | Out-String).Trim()
    Assert-NativeSuccess "Select Go 1.26.0 (preinstall for offline use)"
    $env:PATH = (Join-Path $diagGoRoot "bin") + ";" + $diagOriginalPath
    $env:GOTOOLCHAIN = "local"
    $diagVersion = (& go env GOVERSION | Out-String).Trim()
    Assert-NativeSuccess "Read toolchain version"
    if ($diagVersion -ne "go1.26.0") { throw "Expected go1.26.0, got $diagVersion" }

    $diagFormatting = (& gofmt -l internal cmd | Out-String).Trim()
    Assert-NativeSuccess "gofmt"
    if ($diagFormatting) { throw "Unformatted Go files:`n$diagFormatting" }
    & go vet ./...
    Assert-NativeSuccess "go vet"

    $diagProfileDir = Split-Path -Parent $CoverageFile
    if ($diagProfileDir) { New-Item -ItemType Directory -Force -Path $diagProfileDir | Out-Null }
    # Sequential package execution reduces transient Windows scanner locks on new test executables.
    & go test -p 1 ./... -count=1 "-coverprofile=$CoverageFile"
    Assert-NativeSuccess "go test"
    $diagCoverage = @(& go tool cover "-func=$CoverageFile")
    Assert-NativeSuccess "go tool cover"
    $diagCoverage | ForEach-Object { Write-Verbose $_ }
    $diagCoverage | Where-Object { $_ -match '^total:' } | ForEach-Object { Write-Host $_ }

    # Weighted package/overall statements are computed from raw blocks, never package averages.
    $diagTotals = @{}
    Get-Content -LiteralPath $CoverageFile | Select-Object -Skip 1 | ForEach-Object {
        if ($_ -match '^(\S+)\s+(\d+)\s+(\d+)$') {
            $diagSource = ($matches[1] -split ':')[0]
            $diagStatements = [int]$matches[2]
            $diagCovered = 0
            if ([int]$matches[3] -gt 0) { $diagCovered = $diagStatements }
            $diagPackage = ($diagSource -replace '/[^/]+$', '')
            foreach ($diagKey in @("total", $diagPackage)) {
                if (-not $diagTotals.ContainsKey($diagKey)) { $diagTotals[$diagKey] = @(0, 0) }
                $diagTotals[$diagKey][0] += $diagStatements
                $diagTotals[$diagKey][1] += $diagCovered
            }
        }
    }
    $diagPrefix = "github.com/yukin371/desktop-diag/internal/"
    $diagGates = @(
        [pscustomobject]@{ Key = "total"; Minimum = 70 },
        [pscustomobject]@{ Key = ($diagPrefix + "detect"); Minimum = 90 },
        [pscustomobject]@{ Key = ($diagPrefix + "report"); Minimum = 75 }
    )
    foreach ($diagGate in $diagGates) {
        $diagCounts = $diagTotals[$diagGate.Key]
        if (-not $diagCounts -or $diagCounts[0] -eq 0) { throw "Missing coverage for $($diagGate.Key)" }
        $diagPercent = 100.0 * $diagCounts[1] / $diagCounts[0]
        if ($diagPercent -lt $diagGate.Minimum) { throw "Coverage $($diagGate.Key): $diagPercent below $($diagGate.Minimum)" }
    }
    foreach ($diagFunction in @("Classify", "ClassifyNetwork")) {
        $diagLine = $diagCoverage | Where-Object { $_ -match "\s$diagFunction\s+100\.0%$" }
        if (-not $diagLine) { throw "$diagFunction requires 100% statement coverage" }
    }
    # Explicit pure-function scope: transformations and formatting; excludes IO and orchestration.
    $diagPure = @("convertAdapter", "mergeAdapterInventory", "ifTypeName", "operStatusName", "classifyScope", "prefixToMask", "detectVirtual", "cleanString", "dropBlanks", "splitGateways", "orNone", "displayOr", "normalizeOSName", "formatOSVersion", "archDisplay", "skippedProbe", "probeSummary")
    foreach ($diagFunction in $diagPure) {
        $diagLine = @($diagCoverage | Where-Object { $_ -match "/collect/.*\s$diagFunction\s+([0-9.]+)%$" })
        if ($diagLine.Count -ne 1 -or $diagLine[0] -notmatch '\s([0-9.]+)%$') { throw "Missing pure function $diagFunction" }
        if ([double]::Parse($matches[1], [Globalization.CultureInfo]::InvariantCulture) -lt 80) { throw "$diagFunction requires at least 80% coverage" }
    }
    if ($Race) {
        & go test -race -p 1 ./... -count=1
        Assert-NativeSuccess "go test -race"
    }
    Write-Host "Phase-3 checks passed ($diagVersion)."
}
finally {
    $env:PATH = $diagOriginalPath
    $env:GOTOOLCHAIN = $diagOriginalToolchain
    Pop-Location
}
