[CmdletBinding()]
param(
    [switch]$EvaluateOnce,
    [string]$DiagnosticPath,
    [string]$TunnelLogPath,
    [string]$OwnerStatePath,
    [string]$CandidateExecutable,
    [string]$RustExecutable = (Join-Path $env:USERPROFILE '.codexify\bin\codexify.exe'),
    [string]$GoServiceName = 'CodexifyGo',
    [string]$ObservationPath,
    [string]$RecoveryDirectory,
    [string]$RollbackScript,
    [switch]$ArmRollback,
    [switch]$QuietWindow,
    [switch]$Settled,
    [string]$Now,
    [int]$PollSeconds = 2,
    [int]$SettlingSeconds = 30
)

$ErrorActionPreference = 'Stop'

$toolDir = Split-Path -Parent $MyInvocation.MyCommand.Path
Import-Module (Join-Path $toolDir 'CutoverGuard.psm1') -Force
if (!$RollbackScript) { $RollbackScript = Join-Path $toolDir 'Rollback-To-Rust.ps1' }

function Read-JsonLines {
    param([string]$Path,[int]$Tail=25000)
    if ([string]::IsNullOrWhiteSpace($Path) -or !(Test-Path -LiteralPath $Path)) { return @() }
    $files = @()
    $item = Get-Item -LiteralPath $Path
    if ($item.PSIsContainer) {
        $files = @(Get-ChildItem -LiteralPath $Path -File -Filter '*.jsonl' -Recurse | Sort-Object LastWriteTime)
    } else {
        $files = @($item)
    }
    $result = New-Object 'System.Collections.Generic.List[object]'
    foreach($file in $files) {
        foreach($line in @(Get-Content -LiteralPath $file.FullName -Tail $Tail -ErrorAction Stop)) {
            if ([string]::IsNullOrWhiteSpace($line)) { continue }
            try { $result.Add(($line | ConvertFrom-Json -ErrorAction Stop)) } catch { }
        }
    }
    return $result.ToArray()
}

function Get-ServiceExecutable([string]$PathName) {
    if ([string]::IsNullOrWhiteSpace($PathName)) { return '' }
    $s=$PathName.Trim()
    if($s.StartsWith('"')) {
        $end=$s.IndexOf('"',1)
        if($end -gt 1){ return $s.Substring(1,$end-1) }
        return ''
    }
    $space=$s.IndexOf(' ')
    if($space -lt 0){ return $s }
    return $s.Substring(0,$space)
}

function Get-LiveOwnerState {
    param([string]$GoExe,[string]$RustExe,[string]$ServiceName)
    $controlled=$true
    $go=0
    $rust=0
    $goPath = if($GoExe){[IO.Path]::GetFullPath($GoExe)}else{''}
    $rustPath = if($RustExe){[IO.Path]::GetFullPath($RustExe)}else{''}
    $processes=@(Get-CimInstance Win32_Process | Where-Object {$_.ExecutablePath})
    foreach($p in $processes) {
        $path=[IO.Path]::GetFullPath([string]$p.ExecutablePath)
        if($goPath -and $path.Equals($goPath,[StringComparison]::OrdinalIgnoreCase)){ $go++ }
        if($rustPath -and $path.Equals($rustPath,[StringComparison]::OrdinalIgnoreCase)){ $rust++ }
    }
    if($goPath) {
        $svc=Get-CimInstance Win32_Service -Filter ("Name='" + $ServiceName.Replace("'","''") + "'") -ErrorAction SilentlyContinue
        if($svc) {
            $serviceExe=Get-ServiceExecutable ([string]$svc.PathName)
            if(!$serviceExe -or !([IO.Path]::GetFullPath($serviceExe)).Equals($goPath,[StringComparison]::OrdinalIgnoreCase)) {
                $controlled=$false
            }
        }
    }
    return [pscustomobject]@{GoOwners=$go;RustOwners=$rust;Controlled=$controlled}
}

function Get-OwnedSignature {
    param([string]$GoExe)
    if(!$GoExe){ return '' }
    $goPath=[IO.Path]::GetFullPath($GoExe)
    $runRoot=Split-Path -Parent (Split-Path -Parent $goPath)
    $parts=@(Get-CimInstance Win32_Process | Where-Object {
        $_.ExecutablePath -and (
            ([IO.Path]::GetFullPath([string]$_.ExecutablePath)).Equals($goPath,[StringComparison]::OrdinalIgnoreCase) -or
            ([IO.Path]::GetFullPath([string]$_.ExecutablePath)).StartsWith($runRoot.TrimEnd('\')+'\',[StringComparison]::OrdinalIgnoreCase)
        )
    } | Sort-Object ProcessId | ForEach-Object { ([string]$_.ProcessId)+':'+([string]$_.Name) })
    return ($parts -join ',')
}

function Write-Observation {
    param($Decision,[DateTimeOffset]$At,$OwnerState,[string]$Path)
    if([string]::IsNullOrWhiteSpace($Path)){ return }
    $parent=Split-Path -Parent $Path
    if($parent){ New-Item -ItemType Directory -Force -Path $parent | Out-Null }
    [pscustomobject]@{
        time=$At.ToString('o')
        action=$Decision.Action
        reason=$Decision.Reason
        count=$Decision.Count
        goOwners=$OwnerState.GoOwners
        rustOwners=$OwnerState.RustOwners
        controlled=$OwnerState.Controlled
    } | ConvertTo-Json -Compress | Add-Content -LiteralPath $Path -Encoding UTF8
}

if($EvaluateOnce) {
    if(!$DiagnosticPath -or !$TunnelLogPath -or !$OwnerStatePath){ throw 'EvaluateOnce requires DiagnosticPath, TunnelLogPath and OwnerStatePath.' }
    $at=if($Now){[DateTimeOffset]::Parse($Now)}else{[DateTimeOffset]::UtcNow}
    $owners=Get-Content -LiteralPath $OwnerStatePath -Raw | ConvertFrom-Json
    $decision=Get-CutoverGuardDecision -Now $at -DiagnosticEvents @(Read-JsonLines $DiagnosticPath) -TunnelEvents @(Read-JsonLines $TunnelLogPath) -Settled:$Settled -QuietWindow:$QuietWindow -OwnerState $owners -RestartTimes @()
    $decision | ConvertTo-Json -Compress
    exit 0
}

if(!$CandidateExecutable){ throw 'Monitor mode requires CandidateExecutable.' }
if(!$DiagnosticPath){ throw 'Monitor mode requires DiagnosticPath.' }
if(!$TunnelLogPath){ throw 'Monitor mode requires TunnelLogPath.' }
if($PollSeconds -lt 1){ throw 'PollSeconds must be >= 1.' }
if($SettlingSeconds -lt 0){ throw 'SettlingSeconds must be >= 0.' }
if($ArmRollback -and (!$RecoveryDirectory -or !(Test-Path -LiteralPath $RollbackScript -PathType Leaf))) {
    throw 'ArmRollback requires RecoveryDirectory and a valid RollbackScript.'
}

$candidate=[IO.Path]::GetFullPath($CandidateExecutable)
$started=[DateTimeOffset]::UtcNow
$restartTimes=New-Object 'System.Collections.Generic.List[object]'
$lastSignature=''
$haveBaseline=$false

while($true) {
    $at=[DateTimeOffset]::UtcNow
    $owners=[pscustomobject]@{GoOwners=0;RustOwners=0;Controlled=$false}
    try {
        $isSettled=($at -ge $started.AddSeconds($SettlingSeconds))
        $owners=Get-LiveOwnerState -GoExe $candidate -RustExe $RustExecutable -ServiceName $GoServiceName
        $signature=Get-OwnedSignature -GoExe $candidate
        if(!$haveBaseline) {
            $lastSignature=$signature
            $haveBaseline=$true
        } elseif($isSettled -and $signature -ne $lastSignature -and $lastSignature -ne '' -and $signature -ne '') {
            $restartTimes.Add($at)
            $lastSignature=$signature
        } else {
            $lastSignature=$signature
        }

        $decision=Get-CutoverGuardDecision -Now $at -DiagnosticEvents @(Read-JsonLines $DiagnosticPath) -TunnelEvents @(Read-JsonLines $TunnelLogPath) -Settled:$isSettled -QuietWindow:$QuietWindow -OwnerState $owners -RestartTimes $restartTimes.ToArray()
        Write-Observation -Decision $decision -At $at -OwnerState $owners -Path $ObservationPath
    } catch {
        $decision=[pscustomobject]@{Action='ABORT';Reason='observation_failed';Count=1}
        try { Write-Observation -Decision $decision -At $at -OwnerState $owners -Path $ObservationPath }
        catch { Write-Warning 'Observation could not be written; proceeding with local abort.' }
    }

    if($decision.Action -eq 'ABORT') {
        if($ArmRollback) {
            & $RollbackScript -RecoveryDirectory $RecoveryDirectory -ExpectedGoExecutable $candidate -GoServiceName $GoServiceName -Execute
            if($LASTEXITCODE -ne 0){ exit $LASTEXITCODE }
        }
        $decision | ConvertTo-Json -Compress
        exit 20
    }
    if($decision.Action -eq 'HOLD') {
        $decision | ConvertTo-Json -Compress
    }
    Start-Sleep -Seconds $PollSeconds
}
