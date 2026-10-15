Set-StrictMode -Version Latest

# Deterministic policy for a Go-only Windows upgrade and its observer.
# External HTTP 403/429/503 and missing tunnel child are never service
# rollback inputs. This is not a Rust fallback mechanism.
function Get-GoTrialDecision {
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)]$Snapshot,
        [ValidateRange(0,10000)][int]$ConsecutiveServiceFailures = 0,
        [ValidateRange(1,1000)][int]$RecoveryThreshold = 6
    )

    if ([int]$Snapshot.RustCount -gt 0) {
        return [pscustomobject]@{Action='BLOCKED';Reason='rust_owner_detected';Recovery=$false}
    }
    if (-not [bool]$Snapshot.ImageMatchesExpected) {
        return [pscustomobject]@{Action='BLOCKED';Reason='unexpected_scm_image';Recovery=$false}
    }
    # An orphaned or duplicate Go process using the service image makes the
    # ownership topology ambiguous. Do not attempt Go recovery or mark G10
    # healthy until an operator can identify the extra owner.
    if ([int]$Snapshot.GoOwners -gt 2) {
        return [pscustomobject]@{Action='BLOCKED';Reason='duplicate_go_processes';Recovery=$false}
    }
    if ([string]$Snapshot.ServiceState -ne 'Running' -or [int]$Snapshot.GoSupervisors -ne 1) {
        if ($ConsecutiveServiceFailures -ge $RecoveryThreshold) {
            return [pscustomobject]@{Action='RECOVER_GO';Reason='go_supervisor_unavailable';Recovery=$true}
        }
        return [pscustomobject]@{Action='HOLD';Reason='go_supervisor_unavailable';Recovery=$false}
    }
    if ([int]$Snapshot.Workers -ne 1) {
        return [pscustomobject]@{Action='HOLD';Reason='worker_supervision';Recovery=$false}
    }
    if ([int]$Snapshot.Tunnels -ne 1) {
        return [pscustomobject]@{Action='HOLD';Reason='tunnel_child_recovery';Recovery=$false}
    }
    return [pscustomobject]@{Action='CONTINUE';Reason='within_limits';Recovery=$false}
}

function Get-GoServiceExecutable {
    [CmdletBinding()]
    param([Parameter(Mandatory)][string]$PathName)
    $s=$PathName.Trim()
    if ($s.StartsWith('"')) {
        $end=$s.IndexOf('"',1)
        if ($end -le 1){throw 'Unterminated SCM ImagePath executable'}
        return $s.Substring(1,$end-1)
    }
    if ($s -notmatch '^(\S+)(?:\s|$)') { throw 'Invalid SCM ImagePath' }
    return $Matches[1]
}

function Compare-GoPath {
    param([string]$Left,[string]$Right)
    if ([string]::IsNullOrWhiteSpace($Left) -or [string]::IsNullOrWhiteSpace($Right)) {return $false}
    try {
        $l=[IO.Path]::GetFullPath($Left)
        $r=[IO.Path]::GetFullPath($Right)
        return [string]::Equals($l,$r,[StringComparison]::OrdinalIgnoreCase)
    } catch {return $false}
}

function Set-GoServiceImage {
    [CmdletBinding()]
    param([Parameter(Mandatory)][string]$ServiceName,
        [Parameter(Mandatory)][string]$ImagePath)
    $safeName=$ServiceName.Replace("'","''")
    $service=Get-CimInstance Win32_Service -Filter ("Name='"+$safeName+"'") -ErrorAction Stop
    if(!$service){throw 'Go SCM service not found'}
    # Win32_Service.Change calls ChangeServiceConfig in the SCM; editing the
    # registry value directly can leave an out-of-date cached service config.
    $result=Invoke-CimMethod -InputObject $service -MethodName Change -Arguments @{PathName=$ImagePath} -ErrorAction Stop
    if([int]$result.ReturnValue -ne 0){
        throw "SCM ChangeServiceConfig failed with result $($result.ReturnValue)"
    }
    $actual=Get-CimInstance Win32_Service -Filter ("Name='"+$safeName+"'") -ErrorAction Stop
    if(!$actual -or [string]$actual.PathName -ne $ImagePath) {
        throw 'SCM new ImagePath does not match requested binary/config'
    }
}

function Assert-GoUpgradeAssets {
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)][string]$OldExecutable,
        [Parameter(Mandatory)][string]$NewExecutable,
        [Parameter(Mandatory)][string]$ConfigPath,
        [Parameter(Mandatory)][string]$OldSHA256,
        [Parameter(Mandatory)][string]$NewSHA256,
        [Parameter(Mandatory)][string]$ConfigSHA256
    )
    foreach($pair in @(
        @{path=$OldExecutable;hash=$OldSHA256;role='old Go'},
        @{path=$NewExecutable;hash=$NewSHA256;role='new Go'},
        @{path=$ConfigPath;hash=$ConfigSHA256;role='config'}
    )){
        if(!(Test-Path -LiteralPath $pair.path -PathType Leaf)){throw "Missing $($pair.role) asset"}
        if($pair.hash -notmatch '^[A-Fa-f0-9]{64}$'){throw "Invalid $($pair.role) SHA256"}
        $actual=(Get-FileHash -LiteralPath $pair.path -Algorithm SHA256).Hash
        if($actual -ne $pair.hash.ToUpperInvariant()){throw "SHA256 mismatch for $($pair.role)"}
    }
    if(Compare-GoPath $OldExecutable $NewExecutable){throw 'Source and target binaries must be different immutable files'}
    return $true
}

function Write-GoUpgradeEvent {
    [CmdletBinding()]
    param([Parameter(Mandatory)][string]$Path,[Parameter(Mandatory)][string]$Phase,
        [string]$Result='INFO',[string]$Reason='')
    $record=[pscustomobject]@{
        time=[DateTimeOffset]::UtcNow.ToString('o')
        phase=$Phase;result=$Result;reason=$Reason
    }
    $record|ConvertTo-Json -Compress|Add-Content -LiteralPath $Path -Encoding UTF8 -ErrorAction Stop
}

Export-ModuleMember -Function Get-GoTrialDecision,Get-GoServiceExecutable,Compare-GoPath,Set-GoServiceImage,Assert-GoUpgradeAssets,Write-GoUpgradeEvent
