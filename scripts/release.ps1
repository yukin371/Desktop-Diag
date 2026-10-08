<#
.SYNOPSIS
    生成 Desktop-Diag 的发布目录、SHA256 校验文件和构建元数据。

.DESCRIPTION
    调用 build.ps1 生成带版本注入的单文件 EXE，再把产物复制到独立发布目录。
    脚本只使用当前进程环境，不创建 Git tag、不上传文件、不修改系统配置。

.PARAMETER Version
    发布版本号。留空时由 build.ps1 使用 git describe 取得。

.PARAMETER OutputDir
    发布目录，默认 dist/release/<版本号>。

.EXAMPLE
    powershell -NoProfile -ExecutionPolicy Bypass -File scripts\release.ps1 -Version v0.1.0
#>
[CmdletBinding()]
param(
    [string]$Version = "",
    [string]$OutputDir = "",
    [string]$CertificateThumbprint = "",
    [string]$TimestampServer = "",
    [switch]$RequireSignature
)

$ErrorActionPreference = "Stop"
$diagRepoRoot = Split-Path -Parent $PSScriptRoot
$diagBuildDir = Join-Path $diagRepoRoot ("dist\release-build-" + [guid]::NewGuid().ToString("N"))
$diagOriginalPath = $env:PATH
$diagOriginalCGO = $env:CGO_ENABLED
$diagOriginalToolchain = $env:GOTOOLCHAIN

function Get-GitValue {
    param(
        [Parameter(Mandatory)][string[]]$Arguments,
        [Parameter(Mandatory)][string]$Fallback
    )
    $diagPreviousErrorAction = $ErrorActionPreference
    $ErrorActionPreference = "Continue"
    try {
        $diagOutput = (& git @Arguments 2>&1 | Out-String).Trim()
        if ($LASTEXITCODE -ne 0 -or [string]::IsNullOrWhiteSpace($diagOutput)) { return $Fallback }
        return ($diagOutput -split "`r?`n" | Select-Object -First 1).Trim()
    }
    finally {
        $ErrorActionPreference = $diagPreviousErrorAction
    }
}

# Refuse a signing-required release before creating any artifacts when no identity is supplied.
if ($RequireSignature -and -not $CertificateThumbprint) { throw "RequireSignature 必须提供可信 CertificateThumbprint" }

Push-Location $diagRepoRoot
try {
    if ([string]::IsNullOrWhiteSpace($Version)) {
        $Version = Get-GitValue -Arguments @("describe", "--tags", "--always", "--dirty") -Fallback "dev"
    }
    if ($Version.IndexOfAny([IO.Path]::GetInvalidFileNameChars()) -ge 0) {
        throw "版本号包含 Windows 文件名非法字符: $Version"
    }

    if ([string]::IsNullOrWhiteSpace($OutputDir)) {
        $OutputDir = Join-Path $diagRepoRoot ("dist\release\" + $Version)
    }
    $diagBuildScript = Join-Path $PSScriptRoot "build.ps1"
    & powershell -NoProfile -ExecutionPolicy Bypass -File $diagBuildScript -Version $Version -OutputDir $diagBuildDir
    if ($LASTEXITCODE -ne 0) { throw "build.ps1 失败，退出码 $LASTEXITCODE" }

    $diagSourceExe = Join-Path $diagBuildDir "Desktop-Diag.exe"
    if (-not (Test-Path -LiteralPath $diagSourceExe -PathType Leaf)) {
        throw "构建完成但未找到产物: $diagSourceExe"
    }
    New-Item -ItemType Directory -Force -Path $OutputDir | Out-Null
    $diagTargetExe = Join-Path $OutputDir "Desktop-Diag.exe"
    Copy-Item -LiteralPath $diagSourceExe -Destination $diagTargetExe -Force

    # Sign before hashing; no private key or password is copied into the release directory.
    if ($CertificateThumbprint) {
        if (-not $TimestampServer.StartsWith("https://")) { throw "签名须提供 HTTPS RFC3161 TimestampServer" }
        $diagSigner = Get-Command signtool.exe -ErrorAction Stop
        & $diagSigner.Source sign /sha1 $CertificateThumbprint /fd SHA256 /tr $TimestampServer /td SHA256 $diagTargetExe
        if ($LASTEXITCODE -ne 0) { throw "Authenticode 签名失败" }
        & $diagSigner.Source verify /pa /all $diagTargetExe
        if ($LASTEXITCODE -ne 0) { throw "Authenticode 验证失败" }
    }
    $diagSignature = Get-AuthenticodeSignature -LiteralPath $diagTargetExe
    if ($RequireSignature -and $diagSignature.Status -ne 'Valid') { throw "正式发布要求有效可信签名，当前为 $($diagSignature.Status)" }
    if ($CertificateThumbprint -and $diagSignature.Status -ne 'Valid') { throw "签名不受信任: $($diagSignature.Status)" }
    if ($diagSignature.Status -ne 'Valid') { Write-Warning "候选 EXE 未取得可信签名；不能保证 SmartScreen 放行。" }

    $diagHash = (Get-FileHash -LiteralPath $diagTargetExe -Algorithm SHA256).Hash.ToUpperInvariant()
    $diagHashPath = Join-Path $OutputDir "Desktop-Diag.exe.sha256"
    [IO.File]::WriteAllText($diagHashPath, "$diagHash  Desktop-Diag.exe`n", (New-Object Text.UTF8Encoding($false)))

    $diagCommit = Get-GitValue -Arguments @("rev-parse", "--short", "HEAD") -Fallback "unknown"
    $diagBuildTime = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")
    $diagMetadata = [ordered]@{
        signatureStatus = [string]$diagSignature.Status
        signerSubject = if ($diagSignature.SignerCertificate) { $diagSignature.SignerCertificate.Subject } else { $null }
        version = $Version
        commit = $diagCommit
        buildTime = $diagBuildTime
        go = "go1.26.0"
        target = "windows/amd64"
        sha256 = $diagHash
        bytes = (Get-Item -LiteralPath $diagTargetExe).Length
    } | ConvertTo-Json
    [IO.File]::WriteAllText((Join-Path $OutputDir "build-metadata.json"), "$diagMetadata`n", (New-Object Text.UTF8Encoding($false)))
    Write-Host "发布产物：$diagTargetExe"
    Write-Host "SHA256  ：$diagHash"
}
finally {
    if (Test-Path -LiteralPath $diagBuildDir) {
        Remove-Item -LiteralPath $diagBuildDir -Recurse -Force -ErrorAction SilentlyContinue
    }
    $env:PATH = $diagOriginalPath
    $env:CGO_ENABLED = $diagOriginalCGO
    $env:GOTOOLCHAIN = $diagOriginalToolchain
    Pop-Location
}
