[CmdletBinding()]
param(
    [Parameter(Mandatory)][string]$TransactionDirectory,
    [switch]$ArmGoRecovery,
    [ValidateRange(2,120)][int]$IntervalSeconds=10,
    [ValidateRange(3,30)][int]$RecoveryThreshold=6,
    [ValidateRange(1,72)][int]$DurationHours=27
)
$ErrorActionPreference='Stop'
Import-Module (Join-Path $PSScriptRoot 'GoOnlyUpgrade.psm1') -Force

$root=[IO.Path]::GetFullPath($TransactionDirectory)
$manifestPath=Join-Path $root 'manifest.json'
if(!(Test-Path -LiteralPath $manifestPath -PathType Leaf)){throw 'Go-only watcher manifest missing'}
$m=Get-Content -LiteralPath $manifestPath -Raw|ConvertFrom-Json
if($m.version -ne 1 -or !$m.newExe -or !$m.configPath -or !$m.serviceName) {
    throw 'Invalid Go-only watcher manifest'
}
$evidence=Join-Path $root 'evidence'
New-Item -ItemType Directory -Force -Path $evidence|Out-Null
$guardPath=Join-Path $evidence 'probe-observations.jsonl'
$samples=Join-Path $evidence 'local-samples.jsonl'
$metrics=Join-Path $evidence 'g10-metrics.jsonl'
$events=Join-Path $evidence 'operator-events.jsonl'
$stopMarker=Join-Path $root 'observer.stop'
$recovery=Join-Path $PSScriptRoot 'Invoke-GoOnlyRecovery.ps1'
$deadline=[DateTimeOffset]::UtcNow.AddHours($DurationHours)
$count=0
$consecutive=0

Write-GoUpgradeEvent -Path $events -Phase 'go_only_watcher_started' -Reason $(if($ArmGoRecovery){'armed_for_go_only_recovery'}else{'observation_only'})
while([DateTimeOffset]::UtcNow -lt $deadline) {
    if(Test-Path -LiteralPath $stopMarker -PathType Leaf) {
        Write-GoUpgradeEvent -Path $events -Phase 'watcher_requested_stop'
        break
    }
    $at=[DateTimeOffset]::UtcNow
    try {
        $service=Get-CimInstance Win32_Service -Filter ("Name='"+$m.serviceName.Replace("'","''")+"'") -ErrorAction Stop
        if(!$service){throw 'CodexifyGo service not installed'}
        $expectedExe=[string]$m.newExe
        $actualImage=Get-GoServiceExecutable ([string]$service.PathName)
        $imageMatches=Compare-GoPath $actualImage $expectedExe
        $all=@(Get-CimInstance Win32_Process -ErrorAction Stop)
        $goOwners=@($all | Where-Object { $_.Name -ieq 'codexify-go.exe' -and $_.ExecutablePath -and (Compare-GoPath $_.ExecutablePath $expectedExe) })
        $rustOwners=@($all | Where-Object { $_.Name -ieq 'codexify.exe' })
        $supervisor=@($goOwners | Where-Object { [int]$_.ProcessId -eq [int]$service.ProcessId })
        $workers=@($goOwners | Where-Object { [int]$_.ParentProcessId -eq [int]$service.ProcessId })
        $tunnels=@($all | Where-Object { $_.Name -ieq 'tunnel-client-runtime.exe' -and [int]$_.ParentProcessId -eq [int]$service.ProcessId })
        $serviceUnavailable=([string]$service.State -ne 'Running' -or $supervisor.Count -ne 1)
        if($serviceUnavailable){$consecutive++}else{$consecutive=0}
        $snapshot=[pscustomobject]@{
            ServiceState=[string]$service.State
            GoOwners=$goOwners.Count
            GoSupervisors=$supervisor.Count;Workers=$workers.Count
            Tunnels=$tunnels.Count;RustCount=$rustOwners.Count
            ImageMatchesExpected=$imageMatches
        }
        $decision=Get-GoTrialDecision -Snapshot $snapshot -ConsecutiveServiceFailures $consecutive -RecoveryThreshold $RecoveryThreshold
        $obs=[pscustomobject]@{
            time=$at.ToString('o');action=$decision.Action;reason=$decision.Reason
            goOwners=$goOwners.Count;rustOwners=$rustOwners.Count;controlled=$imageMatches
            supervisorPid=[int]$service.ProcessId;workerCount=$workers.Count;tunnelCount=$tunnels.Count
            consecutiveServiceFailures=$consecutive
        }
        $obs|ConvertTo-Json -Compress|Add-Content -LiteralPath $guardPath -Encoding UTF8 -ErrorAction Stop
        ([pscustomobject]@{
            time=$at.ToString('o');sample=$count;ok=$true
            goOwners=$goOwners.Count;rustOwners=$rustOwners.Count
            serviceState=$service.State;workers=$workers.Count;tunnels=$tunnels.Count
            reason=$decision.Reason
        }|ConvertTo-Json -Compress)|Add-Content -LiteralPath $samples -Encoding UTF8 -ErrorAction Stop
        if($count%6 -eq 0){
            $pids=@($goOwners)+@($tunnels)
            $procRows=@(foreach($p in $pids){
                $inst=Get-Process -Id $p.ProcessId -ErrorAction SilentlyContinue
                if(!$inst){continue}
                [pscustomobject]@{
                    pid=$inst.Id;name=$inst.ProcessName;cpuSeconds=[math]::Round($inst.CPU,2)
                    workingSetMB=[math]::Round($inst.WorkingSet64/1MB,2)
                    handles=$inst.HandleCount;threads=$inst.Threads.Count
                }
            })
            $cpu=(Get-CimInstance Win32_Processor|Measure-Object -Property LoadPercentage -Average).Average
            ([pscustomobject]@{
                time=$at.ToString('o');serviceState=$service.State
                goCount=$goOwners.Count;tunnelCount=$tunnels.Count
                machineCpuLoad=$cpu;processes=$procRows
            }|ConvertTo-Json -Compress -Depth 6)|Add-Content -LiteralPath $metrics -Encoding UTF8 -ErrorAction Stop
        }
        if($decision.Action -eq 'RECOVER_GO' -and $ArmGoRecovery) {
            Write-GoUpgradeEvent -Path $events -Phase 'go_only_recovery_triggered' -Result 'WARN' -Reason $decision.Reason
            & $recovery -TransactionDirectory $root -Reason $decision.Reason -Execute
            if($LASTEXITCODE -ne 0) {
                Write-GoUpgradeEvent -Path $events -Phase 'go_only_recovery_failed' -Result 'ERROR'
            }
            break
        }
        if($decision.Action -eq 'BLOCKED') {
            Write-GoUpgradeEvent -Path $events -Phase 'local_safety_blocked' -Result 'WARN' -Reason $decision.Reason
            # Do not start Rust, kill foreign owners, or silently rewrite an
            # SCM path that could have been changed intentionally elsewhere.
            break
        }
    } catch {
        try {
            Write-GoUpgradeEvent -Path $events -Phase 'watcher_observation_failed' -Result 'WARN' -Reason $_.Exception.GetType().Name
        }catch{}
        # Observation failure invalidates G10 coverage; not a reason to
        # restore Rust or mutate SCM. Continue attempting fresh samples.
    }
    $count++
    Start-Sleep -Seconds $IntervalSeconds
}
Write-GoUpgradeEvent -Path $events -Phase 'go_only_watcher_exited'
