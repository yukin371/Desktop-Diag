<#
.SYNOPSIS
    执行 Desktop-Diag 阶段五质量门禁。

.DESCRIPTION
    依次检查 Go 格式、vet、测试与覆盖率、windows/amd64 与 windows/arm64
    交叉编译，以及生产 Go 源码中的禁用 API/外部进程调用候选项。
    测试和扫描失败均通过非零退出码返回。

.PARAMETER CoverageFile
    覆盖率文件路径，默认 dist/quality-coverage.out。

.PARAMETER OutputDir
    交叉编译产物目录，默认 dist/quality-gate。
#>
[CmdletBinding()]
param(
    [string]$CoverageFile = "dist/quality-coverage.out",
    [string]$OutputDir = "dist/quality-gate"
)

$ErrorActionPreference = "Stop"
$diagRepoRoot = Split-Path -Parent $PSScriptRoot
$diagOriginalPath = $env:PATH
$diagOriginalGoOS = $env:GOOS
$diagOriginalGoArch = $env:GOARCH
$diagOriginalCGO = $env:CGO_ENABLED
$diagOriginalToolchain = $env:GOTOOLCHAIN

function Assert-NativeSuccess {
    param([Parameter(Mandatory)][string]$Step)
    if ($LASTEXITCODE -ne 0) { throw "$Step 失败，退出码 $LASTEXITCODE" }
}

function Invoke-GoBuild {
    param(
        [Parameter(Mandatory)][string]$Architecture,
        [Parameter(Mandatory)][string]$Destination
    )
    $env:GOOS = "windows"
    $env:GOARCH = $Architecture
    $env:CGO_ENABLED = "0"
    & go build -trimpath -o $Destination ./cmd/desktop-diag
    Assert-NativeSuccess "windows/$Architecture 交叉编译"
}

Push-Location $diagRepoRoot
try {
    $env:GOTOOLCHAIN = "go1.26.0"
    $diagGoRoot = (& go env GOROOT | Out-String).Trim()
    Assert-NativeSuccess "选择 Go 1.26.0"
    $env:PATH = (Join-Path $diagGoRoot "bin") + ";" + $diagOriginalPath
    $env:GOTOOLCHAIN = "local"
    $diagVersion = (& go env GOVERSION | Out-String).Trim()
    Assert-NativeSuccess "读取 Go 版本"
    if ($diagVersion -ne "go1.26.0") { throw "需要 go1.26.0，实际为 $diagVersion" }

    $diagFormatting = @(& gofmt -l internal cmd)
    Assert-NativeSuccess "gofmt"
    if ($diagFormatting.Count -gt 0) { throw "存在未格式化文件:`n$($diagFormatting -join "`n")" }

    & go vet ./...
    Assert-NativeSuccess "go vet"

    & powershell -NoProfile -ExecutionPolicy Bypass -File (Join-Path $PSScriptRoot "test.ps1") -CoverageFile $CoverageFile
    Assert-NativeSuccess "测试与覆盖率门禁"

    New-Item -ItemType Directory -Force -Path $OutputDir | Out-Null
    Invoke-GoBuild -Architecture "amd64" -Destination (Join-Path $OutputDir "Desktop-Diag-amd64.exe")
    Invoke-GoBuild -Architecture "arm64" -Destination (Join-Path $OutputDir "Desktop-Diag-arm64.exe")

    $diagProductionFiles = @(
        Get-ChildItem -LiteralPath (Join-Path $diagRepoRoot "internal"), (Join-Path $diagRepoRoot "cmd") -Filter "*.go" -File -Recurse |
            Where-Object { $_.Name -notlike "*_test.go" }
    )
    $diagForbiddenPattern = 'RegSetValue|RegCreateKey|SetIpInterfaceEntry|CreateService|InternetOpen|os/exec|exec\.Command|CreateProcess|ShellExecute|powershell|wmic|wscript|cscript|cmd\.exe'
    $diagHits = @($diagProductionFiles | Select-String -Pattern $diagForbiddenPattern)
    if ($diagHits.Count -gt 0) {
        $diagDetails = $diagHits | ForEach-Object { "$($_.Path):$($_.LineNumber): $($_.Line.Trim())" }
        throw "生产代码命中禁用 API/外部进程候选项:`n$($diagDetails -join "`n")"
    }
    Write-Host "质量门禁通过：$diagVersion"
}
finally {
    $env:PATH = $diagOriginalPath
    $env:GOOS = $diagOriginalGoOS
    $env:GOARCH = $diagOriginalGoArch
    $env:CGO_ENABLED = $diagOriginalCGO
    $env:GOTOOLCHAIN = $diagOriginalToolchain
    Pop-Location
}
