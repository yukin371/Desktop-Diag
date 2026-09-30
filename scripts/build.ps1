<#
.SYNOPSIS
    构建 Desktop-Diag 单文件可执行程序，并注入版本信息。

.DESCRIPTION
    产出 dist/Desktop-Diag.exe。版本信息通过 -ldflags -X 注入到
    internal/version 包，因此 `Desktop-Diag.exe -version` 能打印真实构建元数据。

    关键点：
      · CGO_ENABLED=0  → 纯静态链接，不依赖 MSVC 运行库，保证“单文件免安装”
      · -trimpath      → 去除本地绝对路径，兼顾可复现性与信息泄露防护
      · -s -w          → 剥离符号表与 DWARF，压缩体积（REQ-N-04 ≤ 15 MB）

    兼容性：Windows PowerShell 5.1 与 PowerShell 7 均可运行。
    注意：本文件含中文，必须以 UTF-8 BOM 保存，否则 PS 5.1 会按 ANSI 解析导致乱码。

.PARAMETER Version
    覆盖版本号。留空时自动取 `git describe --tags --always --dirty`；无 git 元数据时回退 dev。

.PARAMETER OutputDir
    产物目录，默认 dist。

.EXAMPLE
    powershell -NoProfile -File scripts\build.ps1
    powershell -NoProfile -File scripts\build.ps1 -Version v0.1.0
#>
[CmdletBinding()]
param(
    [string]$Version = "",
    [string]$OutputDir = "dist"
)

$ErrorActionPreference = "Stop"

$modulePath = "github.com/yukin371/desktop-diag"
$repoRoot = Split-Path -Parent $PSScriptRoot

<#
    安全调用 git：仓库可能尚无任何提交（HEAD 不存在），
    git 会把 "fatal: bad revision 'HEAD'" 写到 stderr。
    在 $ErrorActionPreference='Stop' 下，PS 5.1 会把它升级为终止错误，
    因此这里临时放宽并显式检查退出码。
#>
function Get-GitValue {
    param(
        [Parameter(Mandatory)][string[]]$Arguments,
        [Parameter(Mandatory)][string]$Fallback
    )
    $previous = $ErrorActionPreference
    $ErrorActionPreference = "Continue"
    try {
        $output = (& git @Arguments 2>&1 | Out-String)
        if ($LASTEXITCODE -ne 0) { return $Fallback }
        $line = ($output -split "`r?`n" | Where-Object { $_.Trim() } | Select-Object -First 1)
        if ([string]::IsNullOrWhiteSpace($line)) { return $Fallback }
        return $line.Trim()
    }
    catch {
        return $Fallback
    }
    finally {
        $ErrorActionPreference = $previous
    }
}

# Preserve caller settings; toolchain selection and child PATH stay process-local.
$diagOriginalPath = $env:PATH
$diagOriginalCGO = $env:CGO_ENABLED
$diagOriginalToolchain = $env:GOTOOLCHAIN
Push-Location $repoRoot
try {
    # Pin the complete Go toolchain, including coverage/compiler child processes.
    $diagRequiredGo = "go1.26.0"
    $env:GOTOOLCHAIN = $diagRequiredGo
    $diagGoRoot = (& go env GOROOT | Out-String).Trim()
    if ($LASTEXITCODE -ne 0) { throw "Required Go toolchain $diagRequiredGo unavailable. Preinstall it for offline builds." }
    $env:PATH = (Join-Path $diagGoRoot "bin") + ";" + $diagOriginalPath
    $env:GOTOOLCHAIN = "local"
    $diagGoVersion = (& go env GOVERSION | Out-String).Trim()
    if ($LASTEXITCODE -ne 0 -or $diagGoVersion -ne $diagRequiredGo) { throw "Expected $diagRequiredGo, got $diagGoVersion" }

    # ── 版本元数据 ────────────────────────────────────────────
    if ([string]::IsNullOrWhiteSpace($Version)) {
        $Version = Get-GitValue -Arguments @("describe", "--tags", "--always", "--dirty") -Fallback "dev"
    }
    $commit = Get-GitValue -Arguments @("rev-parse", "--short", "HEAD") -Fallback "unknown"
    $buildTime = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")

    Write-Host "版本号   : $Version"
    Write-Host "提交     : $commit"
    Write-Host "构建时间 : $buildTime"

    # ── 构建参数 ──────────────────────────────────────────────
    $env:CGO_ENABLED = "0"

    # 注意：-X 的目标必须是包内变量的完整导入路径，而非 main.version。
    $ldflags = @(
        "-s",
        "-w",
        "-X '$modulePath/internal/version.Version=$Version'",
        "-X '$modulePath/internal/version.Commit=$commit'",
        "-X '$modulePath/internal/version.BuildTime=$buildTime'"
    ) -join " "

    New-Item -ItemType Directory -Force -Path $OutputDir | Out-Null
    $outExe = Join-Path $OutputDir "Desktop-Diag.exe"

    Write-Host "构建中 ..."
    & go build -trimpath -ldflags $ldflags -o $outExe ./cmd/desktop-diag
    if ($LASTEXITCODE -ne 0) { throw "go build 失败，退出码 $LASTEXITCODE" }

    # ── 结果 ──────────────────────────────────────────────────
    $sizeBytes = (Get-Item $outExe).Length
    $sizeMB = [math]::Round($sizeBytes / 1MB, 2)
    Write-Host ""
    Write-Host "构建成功：$outExe ($sizeMB MB)"

    if ($sizeBytes -gt 15MB) {
        Write-Warning "产物体积 $sizeMB MB 超过 REQ-N-04 上限 15 MB"
    }

    # 自检：确认版本注入确实生效
    $reported = (& $outExe -version 2>&1 | Out-String).Trim()
    Write-Host "版本自检：$reported"
}
finally {
    $env:PATH = $diagOriginalPath
    $env:CGO_ENABLED = $diagOriginalCGO
    $env:GOTOOLCHAIN = $diagOriginalToolchain
    Pop-Location
}
