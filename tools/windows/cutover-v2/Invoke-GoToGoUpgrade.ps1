[CmdletBinding()]
param(
    [Parameter(Mandatory)][string]$TransactionDirectory,
    [switch]$Execute
)
$ErrorActionPreference='Stop'
Import-Module (Join-Path $PSScriptRoot 'GoOnlyUpgrade.psm1') -Force
$root=[IO.Path]::GetFullPath($TransactionDirectory)
$manifestPath=Join-Path $root 'manifest.json'
if(!(Test-Path -LiteralPath $manifestPath -PathType Leaf)){throw 'Missing transaction manifest'}
$m=Get-Content -LiteralPath $manifestPath -Raw|ConvertFrom-Json
if([int]$m.version -ne 1 -or [bool]$m.autoRustFallback){throw 'Invalid or unsafe transaction manifest'}
$serviceName=[string]$m.serviceName
$taskName=[string]$m.g10TaskName
$evidence=Join-Path $root 'evidence'
$events=Join-Path $evidence 'operator-events.jsonl'
$regPath='HKLM:\SYSTEM\CurrentControlSet\Services\'+$serviceName

[void](Assert-GoUpgradeAssets -OldExecutable $m.oldExe -NewExecutable $m.newExe -ConfigPath $m.configPath -OldSHA256 $m.oldSha -NewSHA256 $m.newSha -ConfigSHA256 $m.configSha)
$xmlPath=Join-Path $root 'g10-observers-before.xml'
if((Get-FileHash -LiteralPath $xmlPath -Algorithm SHA256).Hash -ne [string]$m.taskXmlSHA){
    throw 'Boot observer rollback XML hash mismatch'
}
$reg=Get-Item -LiteralPath $regPath -ErrorAction Stop
$image=[string]$reg.GetValue('ImagePath')
if($image -ne [string]$m.originalImage){throw 'Live SCM ImagePath is not the prepared baseline'}
$service=Get-CimInstance Win32_Service -Filter ("Name='"+$serviceName.Replace("'","''")+"'") -ErrorAction Stop
if(!$service -or $service.State -ne 'Running' -or [int]$service.ProcessId -le 0){throw 'Old Go service not in Running state'}
$all=@(Get-CimInstance Win32_Process -ErrorAction Stop)
$process=@($all|Where-Object{[int]$_.ProcessId -eq [int]$service.ProcessId})
if($process.Count -ne 1 -or !(Compare-GoPath $process[0].ExecutablePath $m.oldExe)){throw 'Old Go supervisor changed since preparation'}
$rust=@($all|Where-Object{$_.Name -ieq 'codexify.exe'})
if($rust.Count -ne 0){throw 'Rust process present; upgrade is not single-owner safe'}
foreach($n in @('Codexify','Codexify Watchdog')){
    $task=Get-ScheduledTask -TaskName $n -ErrorAction SilentlyContinue
    if($task -and $task.State -ne 'Disabled'){throw "Rust task $n enabled"}
}
$task=Get-ScheduledTask -TaskName $taskName -ErrorAction Stop
if($task.Principal.RunLevel -ne 'Highest'){throw 'Boot-time observer task lost elevation'}
$oldWatchers=@($all|Where-Object{
    $_.Name -ieq 'powershell.exe' -and $_.CommandLine -match '(?i)-File\s+"?[^"]*Watch-Cutover\.ps1' -and
    $_.CommandLine -match '(?i)-ArmRollback' -and $_.CommandLine.Contains([string]$m.oldExe)
})
if($oldWatchers.Count -ne 1){throw "Expected exactly 1 old rollback-armed watcher, observed $($oldWatchers.Count)"}
$healthProbe=[Uri]([string](Get-Content -LiteralPath $m.configPath -Raw|ConvertFrom-Json).tunnel.mcpServerUrl)
if($healthProbe.Host -ne '127.0.0.1' -or $healthProbe.Scheme -ne 'http'){
    throw 'Local MCP health endpoint must use loopback HTTP'
}
$healthURL='http://127.0.0.1:'+$healthProbe.Port+'/health'
$brief=[pscustomobject]@{
    status='READY_GO_TO_GO';mode=$(if($Execute){'EXECUTE'}else{'PLAN_ONLY'})
    source='previous_go';target='setup_v3_go';externalRustFallback=$false
    candidateSha=$m.newSha;oldSha=$m.oldSha;existingPID=$service.ProcessId
    oldWatcherCount=$oldWatchers.Count;hasBootTask=$true;workerHealth=$healthURL
}
if(!$Execute){
    $brief|ConvertTo-Json -Depth 4 | Set-Content -LiteralPath (Join-Path $root 'operator-preflight.json') -Encoding UTF8
    $brief|ConvertTo-Json -Depth 4
    return
}

$lockPath=Join-Path $root 'upgrade.running.lock'
$lock=$null
$registryChanged=$false
$taskDisabled=$false
$watcherStopped=$false
try {
    # CreateNew enforces a single transaction; task and lock survive the
    # MCP connection disappearing while the service restarts.
    $lock=[IO.File]::Open($lockPath,[IO.FileMode]::CreateNew,[IO.FileAccess]::Write,[IO.FileShare]::None)
    $lock.Close();$lock=$null
    Write-GoUpgradeEvent -Path $events -Phase 'operator_independent_task_started' -Result 'PASS'

    # Stop the old Rust-capable watcher BEFORE changing the expected SCM path.
    Disable-ScheduledTask -TaskName $taskName -ErrorAction Stop|Out-Null
    $taskDisabled=$true
    Stop-Process -Id $oldWatchers[0].ProcessId -Force -ErrorAction Stop
    for($i=0;$i -lt 25;$i++){
        if(!(Get-Process -Id $oldWatchers[0].ProcessId -ErrorAction SilentlyContinue)){break}
        Start-Sleep -Milliseconds 200
    }
    if(Get-Process -Id $oldWatchers[0].ProcessId -ErrorAction SilentlyContinue){
        throw 'Old rollback-armed watcher did not terminate'
    }
    $watcherStopped=$true
    Write-GoUpgradeEvent -Path $events -Phase 'old_rust_capable_watcher_disabled' -Result 'PASS'

    $registryChanged=$true
    # Change only the SCM binary path, retaining account, start type, service
    # recovery policy and the original JSON profile.
    Set-GoServiceImage -ServiceName $serviceName -ImagePath ([string]$m.newImage)
    Write-GoUpgradeEvent -Path $events -Phase 'scm_image_switched' -Result 'PASS'

    Stop-Service -Name $serviceName -ErrorAction Stop
    (Get-Service -Name $serviceName).WaitForStatus('Stopped',[timespan]::FromSeconds(30))
    Start-Service -Name $serviceName -ErrorAction Stop

    $newHealthy=$false
    $newPID=0
    for($i=0;$i -lt 75;$i++){
        Start-Sleep -Seconds 1
        $current=Get-CimInstance Win32_Service -Filter ("Name='"+$serviceName.Replace("'","''")+"'") -ErrorAction SilentlyContinue
        if(!$current -or $current.State -ne 'Running' -or [int]$current.ProcessId -le 0){continue}
        $newPID=[int]$current.ProcessId
        $ps=@(Get-CimInstance Win32_Process -ErrorAction SilentlyContinue)
        $super=@($ps|Where-Object{[int]$_.ProcessId -eq $newPID -and (Compare-GoPath $_.ExecutablePath $m.newExe)})
        $workers=@($ps|Where-Object{$_.Name -ieq 'codexify-go.exe' -and [int]$_.ParentProcessId -eq $newPID})
        $tunnels=@($ps|Where-Object{$_.Name -ieq 'tunnel-client-runtime.exe' -and [int]$_.ParentProcessId -eq $newPID})
        if($super.Count -ne 1 -or $workers.Count -ne 1 -or $tunnels.Count -ne 1){continue}
        try {
            $status=(Invoke-WebRequest -Uri $healthURL -UseBasicParsing -TimeoutSec 2 -ErrorAction Stop).StatusCode
            if($status -eq 200){$newHealthy=$true;break}
        } catch {
            # Temporary MCP startup issues only delay the 75s readiness probe,
            # never trigger a Rust fallback.
        }
    }
    if(!$newHealthy){throw 'New Go service did not reach verified local worker readiness'}
    Write-GoUpgradeEvent -Path $events -Phase 'new_go_supervisor_worker_tunnel_ready' -Result 'PASS'

    $watch=Join-Path $PSScriptRoot 'Watch-GoTrial.ps1'
    if(!(Test-Path -LiteralPath $watch)){throw 'Go-only watcher missing'}
    $watchArgs='-NoLogo -NoProfile -NonInteractive -ExecutionPolicy Bypass -File "'+$watch+'" -TransactionDirectory "'+$root+'" -ArmGoRecovery -IntervalSeconds 10 -DurationHours 27'
    $action=New-ScheduledTaskAction -Execute 'powershell.exe' -Argument $watchArgs
    Set-ScheduledTask -TaskName $taskName -Action $action -ErrorAction Stop|Out-Null
    Enable-ScheduledTask -TaskName $taskName -ErrorAction Stop|Out-Null
    if(Test-Path -LiteralPath (Join-Path $root 'observer.stop')) {
        Remove-Item -LiteralPath (Join-Path $root 'observer.stop') -Force -ErrorAction Stop
    }
    Start-ScheduledTask -TaskName $taskName -ErrorAction Stop
    $guardPath=Join-Path $evidence 'probe-observations.jsonl'
    $newGuard=$false
    for($i=0;$i -lt 35;$i++){
        Start-Sleep -Seconds 1
        if(!(Test-Path -LiteralPath $guardPath)){continue}
        $line=Get-Content -LiteralPath $guardPath -Tail 1|ConvertFrom-Json
        $stamp=[DateTimeOffset]::Parse([string]$line.time)
        if([DateTimeOffset]::UtcNow.Subtract($stamp).TotalSeconds -le 15 -and $line.action -eq 'CONTINUE' -and $line.rustOwners -eq 0 -and $line.goOwners -eq 2){
            $newGuard=$true;break
        }
    }
    if(!$newGuard){throw 'New Go-only G10 watcher did not report fresh CONTINUE'}
    Write-GoUpgradeEvent -Path $events -Phase 'go_only_watcher_ready' -Result 'PASS'

    # Old samplers use the old frozen binary and must not contaminate new G10
    # coverage. These exact identified observer processes are not live MCP.
    $legacy=@(Get-CimInstance Win32_Process | Where-Object {
        $_.Name -ieq 'powershell.exe' -and $_.CommandLine -match '(Observe-G05Local|Collect-G10Metrics)\.ps1' -and
        $_.CommandLine.Contains('20261005T005800Z-g10-tunnel-selfheal-v3')
    })
    foreach($oldObserver in $legacy) {
        Stop-Process -Id $oldObserver.ProcessId -Force -ErrorAction SilentlyContinue
    }
    Write-GoUpgradeEvent -Path $events -Phase 'old_observers_retired' -Result 'PASS' -Reason ('count='+$legacy.Count)

    [pscustomobject]@{
        status='GO_UPGRADED';atUtc=[DateTimeOffset]::UtcNow.ToString('o')
        owner='go';candidateSha=$m.newSha
        observer='go_only_g10';g10Acceptance='RUNNING_NOT_ACCEPTED'
        hostedChatGPTUI='NOT_YET_VERIFIED'
    }|ConvertTo-Json -Depth 4|Set-Content -LiteralPath (Join-Path $root 'upgrade-result.json') -Encoding UTF8
    Write-GoUpgradeEvent -Path $events -Phase 'go_upgrade_transaction_completed' -Result 'PASS'
} catch {
    try {Write-GoUpgradeEvent -Path $events -Phase 'go_upgrade_transaction_failed' -Result 'ERROR' -Reason $_.Exception.GetType().Name}catch{}
    if($registryChanged){
        try{
            & (Join-Path $PSScriptRoot 'Invoke-GoOnlyRecovery.ps1') -TransactionDirectory $root -Reason 'deploy_validation_failed' -Execute
        }catch{
            try { Write-GoUpgradeEvent -Path $events -Phase 'go_only_restore_failed' -Result 'ERROR' -Reason $_.Exception.GetType().Name }catch{}
        }
    } elseif($taskDisabled) {
        # No SCM change happened, the old Go binary is still live. Restore
        # the old task only in this before-swap failure path.
        try{
            Enable-ScheduledTask -TaskName $taskName -ErrorAction Stop|Out-Null
            if($watcherStopped){Start-ScheduledTask -TaskName $taskName -ErrorAction Stop}
        }catch{}
    }
    [pscustomobject]@{status='GO_UPGRADE_FAILED';oldGoRecoveryAttempted=$registryChanged;phase='see_operator_events'}|
        ConvertTo-Json|Set-Content -LiteralPath (Join-Path $root 'upgrade-result.json') -Encoding UTF8
    throw
}
