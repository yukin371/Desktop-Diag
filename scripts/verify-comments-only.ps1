# verify-comments-only.ps1
#
# 校验工作区里已修改的 .go 文件相对 HEAD **只改了注释**，代码未变。
#
# 做法：用 go/ast 去掉全部注释后打印，两者必须逐字节相同。
# 基于 AST 而非正则，字符串里的 // 不会被误判。
#
# 用法：powershell -NoProfile -ExecutionPolicy Bypass -File scripts\verify-comments-only.ps1 [-Scope <路径前缀>]
# 退出码：0 = 全部只改了注释；1 = 有文件改动了代码；2 = 脚本自身出错
#
# 注意：本文件含中文，必须存为 UTF-8 with BOM，否则 Windows PowerShell 5.1 按 ANSI 解码会解析失败。

param([string]$Scope = "")

$ErrorActionPreference = "Stop"
$repo = Split-Path -Parent $PSScriptRoot
$tool = Join-Path $repo "tools\stripcomments\main.go"

function Write-Utf8NoBom {
    param([string]$Path, [string]$Text)
    [System.IO.File]::WriteAllText($Path, $Text, (New-Object System.Text.UTF8Encoding $false))
}

function Get-StrippedCode {
    param([string]$Exe, [string]$Path)
    $out = & $Exe $Path 2>&1
    if ($LASTEXITCODE -ne 0) { throw "stripcomments 处理 $Path 失败: $out" }
    # 去注释会改变行号，go/printer 的保空行策略随之变化，于是出现纯空行/缩进差异。
    # 先把缩进与空行规范化掉，剩下的任何差异都是真正的代码差异。
    $lines = $out | ForEach-Object { $_.Trim() } | Where-Object { $_ -ne "" }
    return ($lines -join "`n")
}

Push-Location $repo
try {
    # 未跟踪的新文件没有 HEAD 版本可比，不参与校验。
    $changed = @(& git diff --name-only HEAD -- "*.go")
    if ($LASTEXITCODE -ne 0) { throw "git diff 失败" }
    if ($Scope) {
        $prefix = $Scope -replace '[\\/]', '\'
        $changed = @($changed | Where-Object { $_ -and ($_ -replace '[\\/]', '\') -like "$prefix*" })
    }

    if ($changed.Count -eq 0) {
        Write-Host "没有需要校验的已修改 .go 文件。"
        exit 0
    }

    $tmp = Join-Path ([System.IO.Path]::GetTempPath()) ("commentcheck_" + [guid]::NewGuid().ToString("N"))
    New-Item -ItemType Directory -Path $tmp | Out-Null

    # 必须从显式文件构建：main.go 带 //go:build ignore，按包路径 build 会被排除；
    # 而 `go run main.go <目标>` 会把 .go 目标当成第二个源文件。
    $exe = Join-Path $tmp "stripcomments.exe"
    & go build -o $exe $tool
    if ($LASTEXITCODE -ne 0) { throw "构建 stripcomments 失败" }

    $bad = @()
    $checked = 0
    foreach ($rel in $changed) {
        $rel = $rel.Trim()
        if (-not $rel) { continue }
        $headFile = Join-Path $tmp ($rel -replace '[\\/]', '_')

        # 不能用 > 重定向：Windows PowerShell 5.1 默认写 UTF-16LE，go/parser 读不了。
        $headText = @(& git show "HEAD:$rel" 2>$null)
        if ($LASTEXITCODE -ne 0) {
            Write-Host "跳过（HEAD 中不存在，属新增文件）: $rel"
            continue
        }
        Write-Utf8NoBom -Path $headFile -Text ($headText -join "`n")

        $before = Get-StrippedCode -Exe $exe -Path $headFile
        $after = Get-StrippedCode -Exe $exe -Path (Join-Path $repo $rel)
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
