$ErrorActionPreference='Stop'
Import-Module (Join-Path $PSScriptRoot 'GoOnlyUpgrade.psm1') -Force
function Assert($value,[string]$message){if(!$value){throw $message}}

$base=[pscustomobject]@{
    ServiceState='Running';GoOwners=2;GoSupervisors=1;Workers=1;Tunnels=1
    RustCount=0;ImageMatchesExpected=$true
    Http403=0;Http429=0;Http503=0;PollFailures=0
}
$normal=Get-GoTrialDecision -Snapshot $base
Assert ($normal.Action -eq 'CONTINUE' -and !$normal.Recovery) 'healthy service'

$networkOnly=$base.PSObject.Copy()
$networkOnly.Http403=5000
$networkOnly.Http429=5000
$networkOnly.Http503=5000
$networkOnly.PollFailures=5000
$network=Get-GoTrialDecision -Snapshot $networkOnly -ConsecutiveServiceFailures 0
Assert ($network.Action -eq 'CONTINUE' -and !$network.Recovery) 'network must never restore Go or Rust'

$temporaryTunnel=$base.PSObject.Copy()
$temporaryTunnel.Tunnels=0
$tunnel=Get-GoTrialDecision -Snapshot $temporaryTunnel -ConsecutiveServiceFailures 50
Assert ($tunnel.Action -eq 'HOLD' -and !$tunnel.Recovery) 'tunnel-child self-healing'

$noWorker=$base.PSObject.Copy()
$noWorker.Workers=0
$noWorker.GoOwners=1
Assert ((Get-GoTrialDecision -Snapshot $noWorker).Action -eq 'HOLD') 'worker self-healing'

$duplicateGo=$base.PSObject.Copy()
$duplicateGo.GoOwners=3
$duplicateDecision=Get-GoTrialDecision -Snapshot $duplicateGo -ConsecutiveServiceFailures 99
Assert ($duplicateDecision.Action -eq 'BLOCKED' -and $duplicateDecision.Reason -eq 'duplicate_go_processes' -and !$duplicateDecision.Recovery) 'extra Go owner cannot silently pass or trigger recovery'

$serviceDown=$base.PSObject.Copy()
$serviceDown.ServiceState='Stopped'
$serviceDown.GoSupervisors=0
$serviceDown.GoOwners=0
Assert ((Get-GoTrialDecision -Snapshot $serviceDown -ConsecutiveServiceFailures 5).Action -eq 'HOLD') 'early stoppage'
$d=Get-GoTrialDecision -Snapshot $serviceDown -ConsecutiveServiceFailures 6
Assert ($d.Action -eq 'RECOVER_GO' -and $d.Recovery) 'confirmed Go service unavailability'

$rust=$base.PSObject.Copy()
$rust.RustCount=1
Assert ((Get-GoTrialDecision -Snapshot $rust -ConsecutiveServiceFailures 9).Action -eq 'BLOCKED') 'no automatic Rust takeover'

$foreign=$base.PSObject.Copy()
$foreign.ImageMatchesExpected=$false
Assert ((Get-GoTrialDecision -Snapshot $foreign -ConsecutiveServiceFailures 9).Action -eq 'BLOCKED') 'unexpected external SCM changes'

$exe=Get-GoServiceExecutable '"C:\Fixtures\go.exe" service run --config "C:\Fixtures\cfg.json"'
Assert ($exe -eq 'C:\Fixtures\go.exe') 'service path parse'

$root=Join-Path $env:TEMP ('codexify-go-upgrade-fixture-'+[guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $root | Out-Null
try {
    $old=Join-Path $root 'previous-go.exe'
    $new=Join-Path $root 'next-go.exe'
    $cfg=Join-Path $root 'config.json'
    Set-Content -LiteralPath $old -Value 'old fixture bytes' -Encoding ASCII
    Set-Content -LiteralPath $new -Value 'new fixture bytes' -Encoding ASCII
    Set-Content -LiteralPath $cfg -Value '{}' -Encoding ASCII
    $good=@{OldExecutable=$old;NewExecutable=$new;ConfigPath=$cfg;
        OldSHA256=(Get-FileHash $old -Algorithm SHA256).Hash;
        NewSHA256=(Get-FileHash $new -Algorithm SHA256).Hash;
        ConfigSHA256=(Get-FileHash $cfg -Algorithm SHA256).Hash}
    Assert (Assert-GoUpgradeAssets @good) 'good verified assets'
    $bad=$good.Clone()
    $bad.NewSHA256='0'*64
    $rejected=$false
    try {Assert-GoUpgradeAssets @bad | Out-Null}catch{$rejected=$true}
    Assert $rejected 'invalid candidate SHA must fail'
    $bad=$good.Clone()
    $bad.NewExecutable=$old
    $bad.NewSHA256=$good.OldSHA256
    $rejected=$false
    try {Assert-GoUpgradeAssets @bad | Out-Null}catch{$rejected=$true}
    Assert $rejected 'same Go executable must fail'
} finally {
    $prefix=[IO.Path]::GetFullPath($env:TEMP).TrimEnd('\')+'\'
    $abs=[IO.Path]::GetFullPath($root)
    if(!$abs.StartsWith($prefix,[StringComparison]::OrdinalIgnoreCase) -or
        (Split-Path -Leaf $abs) -notmatch '^codexify-go-upgrade-fixture-[a-f0-9]{32}$') {throw 'Unsafe fixture cleanup'}
    Remove-Item -LiteralPath $abs -Recurse -Force
}
Write-Output 'GO_ONLY_UPGRADE_POLICY=PASS'
