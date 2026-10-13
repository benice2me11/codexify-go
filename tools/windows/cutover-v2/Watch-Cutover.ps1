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
    return Get-CutoverGoSignature -GoExecutable $GoExe -Processes @(Get-CimInstance Win32_Process)
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
$observationFailures=0
$abortLogPath=if($ObservationPath){Join-Path (Split-Path -Parent $ObservationPath) 'guard-abort.log'}else{Join-Path $env:TEMP 'cutover-guard-abort.log'}

# Preflight: prove the observation write path works before entering the loop.
# A watcher that cannot record its own decisions must fail loudly at launch,
# not start armed and abort blind a few seconds in.
if($ObservationPath) {
    try {
        Write-Observation -Decision ([pscustomobject]@{Action='INFO';Reason='guard_started';Count=0}) -At $started -OwnerState ([pscustomobject]@{GoOwners=0;RustOwners=0;Controlled=$false}) -Path $ObservationPath
    } catch {
        throw "ObservationPath '$ObservationPath' is not writable at launch: $($_.Exception.Message)"
    }
}

while($true) {
    $at=[DateTimeOffset]::UtcNow
    $owners=[pscustomobject]@{GoOwners=0;RustOwners=0;Controlled=$false}
    $ownerObserved=$false
    try {
        $isSettled=($at -ge $started.AddSeconds($SettlingSeconds))
        $owners=Get-LiveOwnerState -GoExe $candidate -RustExe $RustExecutable -ServiceName $GoServiceName
        $ownerObserved=$true
        $signature=Get-OwnedSignature -GoExe $candidate
        if(!$haveBaseline) {
            $lastSignature=$signature
            $haveBaseline=$true
        } elseif($isSettled -and $signature -ne $lastSignature -and $lastSignature -ne '' -and $signature -ne '') {
            # A single supervised child restart produces two signature
            # transitions (kill removes the PID, respawn adds the new one)
            # sampled ~2-4s apart. Merge transitions closer than 30s into one
            # restart episode; genuinely repeated restarts minutes apart still
            # accumulate toward the abort threshold.
            if (Test-CutoverRestartEpisode -At $at -RestartTimes $restartTimes.ToArray()) {
                [void]$restartTimes.Add($at)
            }
            $lastSignature=$signature
        } else {
            $lastSignature=$signature
        }

        $decision=Get-CutoverGuardDecision -Now $at -DiagnosticEvents @(Read-JsonLines $DiagnosticPath) -TunnelEvents @(Read-JsonLines $TunnelLogPath) -Settled:$isSettled -QuietWindow:$QuietWindow -OwnerState $owners -RestartTimes $restartTimes.ToArray()
        Write-Observation -Decision $decision -At $at -OwnerState $owners -Path $ObservationPath
        $observationFailures=0
    } catch {
        # Observation failure is a guard/tooling fault, not a Go crash.
        # Do not switch connector owners on missing or malformed telemetry.
        # Keep retrying, and invalidate G10 until complete observation resumes.
        $observationFailures++
        "$($at.ToString('o')) observation_failed#$observationFailures : $($_.Exception.Message)" | Add-Content -LiteralPath $abortLogPath -Encoding UTF8 -ErrorAction SilentlyContinue
        try { Write-Observation -Decision ([pscustomobject]@{Action='WARN';Reason='observation_failed';Count=$observationFailures}) -At $at -OwnerState $owners -Path $ObservationPath } catch { }
        if ($ownerObserved -and (!$owners.Controlled -or ($owners.GoOwners -gt 0 -and $owners.RustOwners -gt 0))) {
            # A separately confirmed ownership violation still warrants rollback.
            $reason=if (!$owners.Controlled) {'local_control_lost'} else {'duplicate_live_owner'}
            $decision=[pscustomobject]@{Action='ABORT';Reason=$reason;Count=$observationFailures}
        } else {
            if ($observationFailures -ge 3) {
                if ($observationFailures -eq 3) {
                    "$($at.ToString('o')) OBSERVER_DEGRADED no_rollback=true; G10 invalid until healthy" | Add-Content -LiteralPath $abortLogPath -Encoding UTF8 -ErrorAction SilentlyContinue
                }
                try { Write-Observation -Decision ([pscustomobject]@{Action='HOLD';Reason='observer_unhealthy';Count=$observationFailures}) -At $at -OwnerState $owners -Path $ObservationPath } catch { }
            }
            Start-Sleep -Seconds $PollSeconds
            continue
        }
    }

    if($decision.Action -eq 'ABORT') {
        # Durable abort record written BEFORE any rollback runs, so postmortem
        # can always attribute who aborted and why even when the observation
        # file itself is broken.
        "$($at.ToString('o')) ABORT reason=$($decision.Reason) count=$($decision.Count) go=$($owners.GoOwners) rust=$($owners.RustOwners) controlled=$($owners.Controlled)" | Add-Content -LiteralPath $abortLogPath -Encoding UTF8 -ErrorAction SilentlyContinue
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
