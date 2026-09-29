# verify-comments-only.ps1
#
# 校验工作区里所有已修改的 .go 文件相对 HEAD **只改了注释**，代码未变。
#
# 做法：用 go/ast 分别解析 HEAD 版本与工作区版本，去掉全部注释后打印，
# 两者必须逐字节相同。基于 AST 而非正则，字符串里的 // 不会被误判。
#
# 用法：powershell -NoProfile -ExecutionPolicy Bypass -File scripts\verify-comments-only.ps1
# 退出码：0 = 全部只改了注释；1 = 有文件改动了代码；2 = 脚本自身出错

$ErrorActionPreference = "Stop"
$repo = Split-Path -Parent $PSScriptRoot
$tool = Join-Path $repo "tools\stripcomments\main.go"

function Get-StrippedCode {
    param([string]$Path)
    $out = & go run $tool $Path 2>&1
    if ($LASTEXITCODE -ne 0) { throw "stripcomments 处理 $Path 失败: $out" }
    return ($out -join "`n")
}

Push-Location $repo
try {
    # 只取已跟踪文件的改动；未跟踪的新文件没有 HEAD 版本可比，不参与校验。
    $changed = & git diff --name-only HEAD -- "*.go"
    if ($LASTEXITCODE -ne 0) { throw "git diff 失败" }
    if (-not $changed) {
        Write-Host "没有已修改的 .go 文件，无需校验。"
        exit 0
    }

    $tmp = Join-Path ([System.IO.Path]::GetTempPath()) ("commentcheck_" + [guid]::NewGuid().ToString("N"))
    New-Item -ItemType Directory -Path $tmp | Out-Null

    $bad = @()
    $checked = 0
    foreach ($rel in $changed) {
        $rel = $rel.Trim()
        if (-not $rel) { continue }
        $headFile = Join-Path $tmp ((($rel -replace '[\\/]', '_')))

        & git show "HEAD:$rel" > $headFile 2>$null
        if ($LASTEXITCODE -ne 0) {
            Write-Host "跳过（HEAD 中不存在，属新增文件）: $rel"
            continue
        }

        $before = Get-StrippedCode -Path $headFile
        $after  = Get-StrippedCode -Path (Join-Path $repo $rel)
        $checked++

        if ($before -ne $after) {
            $bad += $rel
            Write-Host "代码被改动: $rel" -ForegroundColor Red
        } else {
            Write-Host "仅注释变化: $rel" -ForegroundColor Green
        }
    }

    Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue

    Write-Host ""
    Write-Host "已校验 $checked 个文件。"
    if ($bad.Count -gt 0) {
        Write-Host "以下文件的代码被改动了（只允许改注释）:" -ForegroundColor Red
        $bad | ForEach-Object { Write-Host "  $_" }
        exit 1
    }
    Write-Host "全部只改了注释。" -ForegroundColor Green
    exit 0
}
catch {
    Write-Host "脚本出错: $_" -ForegroundColor Red
    exit 2
}
finally {
    Pop-Location
}
