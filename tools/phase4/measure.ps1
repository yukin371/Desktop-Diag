[CmdletBinding()]
param(
    [Parameter(Mandatory)][string]$Exe,
    [Parameter(Mandatory)][string]$OutputDir,
    [int]$Count = 10,
    [int]$SampleMilliseconds = 10
)

# Development-only measurement helper. It does not run as part of Desktop-Diag.
$ErrorActionPreference = 'Stop'
$diagExe = (Resolve-Path -LiteralPath $Exe).Path
$diagOutput = [IO.Path]::GetFullPath($OutputDir)
if ($Count -lt 1 -or $SampleMilliseconds -lt 1) { throw 'Count and sample interval must be positive.' }
New-Item -ItemType Directory -Force -Path $diagOutput | Out-Null
$diagUTF8 = New-Object Text.UTF8Encoding($false)
$diagRows = @()

for ($diagRun = 1; $diagRun -le $Count; $diagRun++) {
    $diagReports = Join-Path $diagOutput "run-$diagRun"
    New-Item -ItemType Directory -Force -Path $diagReports | Out-Null
    $diagInfo = New-Object Diagnostics.ProcessStartInfo
    $diagInfo.FileName = $diagExe
    $diagInfo.Arguments = '-o "' + $diagReports + '"'
    $diagInfo.UseShellExecute = $false
    $diagInfo.CreateNoWindow = $true
    $diagInfo.RedirectStandardOutput = $true
    $diagInfo.RedirectStandardError = $true
    $diagInfo.StandardOutputEncoding = $diagUTF8
    $diagInfo.StandardErrorEncoding = $diagUTF8
    $diagProcess = New-Object Diagnostics.Process
    $diagProcess.StartInfo = $diagInfo
    $diagTimer = [Diagnostics.Stopwatch]::StartNew()
    $diagStart = [DateTimeOffset]::Now
    $diagFirst = $null
    $diagWorkingSet = 0L
    $diagPrivate = 0L
    $diagSamples = 0
    $diagStdout = New-Object Text.StringBuilder
    try {
        if (-not $diagProcess.Start()) { throw 'Candidate process did not start.' }
        $diagErrorTask = $diagProcess.StandardError.ReadToEndAsync()
        $diagLineTask = $diagProcess.StandardOutput.ReadLineAsync()
        $diagEOF = $false
        while (-not $diagProcess.HasExited -or -not $diagEOF) {
            if (-not $diagProcess.HasExited) {
                $diagProcess.Refresh()
                $diagWorkingSet = [Math]::Max($diagWorkingSet, $diagProcess.WorkingSet64)
                $diagPrivate = [Math]::Max($diagPrivate, $diagProcess.PrivateMemorySize64)
                $diagSamples++
            }
            while (-not $diagEOF -and $diagLineTask.IsCompleted) {
                $diagLine = $diagLineTask.GetAwaiter().GetResult()
                if ($null -eq $diagLine) { $diagEOF = $true; break }
                if ($null -eq $diagFirst) { $diagFirst = $diagTimer.Elapsed.TotalMilliseconds }
                [void]$diagStdout.AppendLine($diagLine)
                $diagLineTask = $diagProcess.StandardOutput.ReadLineAsync()
            }
            if (-not $diagProcess.HasExited -or -not $diagEOF) { Start-Sleep -Milliseconds $SampleMilliseconds }
        }
        $diagProcess.WaitForExit()
        $diagTimer.Stop()
        $diagEnd = [DateTimeOffset]::Now
        $diagStderr = $diagErrorTask.GetAwaiter().GetResult()
        [IO.File]::WriteAllText((Join-Path $diagOutput "run-$diagRun.stdout.txt"), $diagStdout.ToString(), $diagUTF8)
        [IO.File]::WriteAllText((Join-Path $diagOutput "run-$diagRun.stderr.txt"), $diagStderr, $diagUTF8)
        $diagRow = [pscustomobject]@{
            Run = $diagRun; Start = $diagStart.ToString('o'); End = $diagEnd.ToString('o')
            FirstLineMilliseconds = $diagFirst; TotalMilliseconds = $diagTimer.Elapsed.TotalMilliseconds
            ExitCode = $diagProcess.ExitCode; WorkingSetPeakBytes = $diagWorkingSet
            PrivateBytesPeak = $diagPrivate; Samples = $diagSamples
            Reports = @(Get-ChildItem -LiteralPath $diagReports -Filter 'diag_*.txt' | ForEach-Object { $_.FullName })
        }
        $diagRows += $diagRow
        Write-Host ("Run {0}: exit={1} first={2:N1}ms total={3:N1}ms WS={4:N2}MiB" -f $diagRun, $diagRow.ExitCode, $diagFirst, $diagRow.TotalMilliseconds, ($diagWorkingSet / 1MB))
    }
    finally { $diagProcess.Dispose() }
}

$diagEvidence = [pscustomobject]@{
    Exe = $diagExe; SHA256 = (Get-FileHash -LiteralPath $diagExe -Algorithm SHA256).Hash
    Bytes = (Get-Item -LiteralPath $diagExe).Length; Identity = (& whoami | Out-String).Trim()
    SampleMilliseconds = $SampleMilliseconds
    TimingMethod = 'Stopwatch before Process.Start through exit and stdout EOF; first line observed by polling.'
    MemoryMethod = 'Sampled working set; 1 MiB = 1048576 bytes. Short peaks may be missed.'
    Runs = $diagRows
}
[IO.File]::WriteAllText((Join-Path $diagOutput 'measurements.json'), ($diagEvidence | ConvertTo-Json -Depth 6), $diagUTF8)
