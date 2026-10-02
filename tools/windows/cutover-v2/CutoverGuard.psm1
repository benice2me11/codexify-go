Set-StrictMode -Version Latest

function ConvertTo-CutoverTime {
    param([Parameter(Mandatory=$true)]$Value)
    if ($Value -is [DateTimeOffset]) { return $Value }
    if ($Value -is [DateTime]) { return [DateTimeOffset]$Value }
    return [DateTimeOffset]::Parse([string]$Value)
}

function Get-CutoverEventText {
    param([Parameter(Mandatory=$true)]$Event, [Parameter(Mandatory=$true)][string]$Name)
    $property = $Event.PSObject.Properties[$Name]
    if ($null -eq $property) { return '' }
    return [string]$property.Value
}

function Get-CutoverGuardDecision {
    [CmdletBinding()]
    param(
        [Parameter(Mandatory=$true)][DateTimeOffset]$Now,
        [object[]]$DiagnosticEvents = @(),
        [object[]]$TunnelEvents = @(),
        [switch]$Settled,
        [switch]$QuietWindow,
        [Parameter(Mandatory=$true)]$OwnerState,
        [object[]]$RestartTimes = @()
    )

    if (-not [bool]$OwnerState.Controlled) {
        return [pscustomobject]@{Action='ABORT';Reason='local_control_lost';Count=1}
    }
    if ([int]$OwnerState.GoOwners -gt 0 -and [int]$OwnerState.RustOwners -gt 0) {
        return [pscustomobject]@{Action='ABORT';Reason='duplicate_live_owner';Count=([int]$OwnerState.GoOwners + [int]$OwnerState.RustOwners)}
    }

    $recentRestarts = @($RestartTimes | Where-Object {
        $t = ConvertTo-CutoverTime $_
        $t -ge $Now.AddMinutes(-5) -and $t -le $Now
    })
    if ($recentRestarts.Count -ge 2) {
        return [pscustomobject]@{Action='ABORT';Reason='owned_process_restarts';Count=$recentRestarts.Count}
    }

    $rateLimited = @($TunnelEvents | Where-Object {
        $t = ConvertTo-CutoverTime $_.time
        $message = Get-CutoverEventText $_ 'message'
        if (-not $message) { $message = Get-CutoverEventText $_ 'msg' }
        $t -ge $Now.AddMinutes(-1) -and $t -le $Now -and $message -match 'RATE_LIMITED|rate.?limit|HTTP\s*429'
    })
    if ($Settled -and $QuietWindow) {
        $starts = @($DiagnosticEvents | Where-Object {
            $t = ConvertTo-CutoverTime $_.time
            $t -ge $Now.AddSeconds(-10) -and $t -le $Now -and [string]$_.phase -eq 'mcp_start' -and
                (Get-CutoverEventText $_ 'rpc_method') -eq 'tools/call'
        })
        if ($starts.Count -ge 10) {
            return [pscustomobject]@{Action='ABORT';Reason='mcp_calls_10s';Count=$starts.Count}
        }

        $repeat = @($starts |
            Where-Object { -not [string]::IsNullOrWhiteSpace((Get-CutoverEventText $_ 'operation_hash')) } |
            Group-Object operation_hash |
            Sort-Object Count -Descending |
            Select-Object -First 1)
        if ($repeat.Count -gt 0 -and [int]$repeat[0].Count -ge 5) {
            return [pscustomobject]@{Action='ABORT';Reason='repeated_operation_10s';Count=[int]$repeat[0].Count}
        }

        $forwarded = @($TunnelEvents | Where-Object {
            $t = ConvertTo-CutoverTime $_.time
            $message = Get-CutoverEventText $_ 'message'
            if (-not $message) { $message = Get-CutoverEventText $_ 'msg' }
            $t -ge $Now.AddSeconds(-60) -and $t -le $Now -and $message -eq 'dispatcher forwarded command to MCP server'
        })
        if ($forwarded.Count -gt 100) {
            return [pscustomobject]@{Action='ABORT';Reason='forwarded_60s';Count=$forwarded.Count}
        }
    }

    if ($rateLimited.Count -gt 0) {
        return [pscustomobject]@{Action='HOLD';Reason='rate_limited';Count=$rateLimited.Count}
    }

    return [pscustomobject]@{Action='CONTINUE';Reason='within_limits';Count=0}
}

function Test-RecoverySnapshot {
    [CmdletBinding()]
    param([Parameter(Mandatory=$true)][string]$RecoveryDirectory)

    $root = [IO.Path]::GetFullPath($RecoveryDirectory)
    $manifestPath = Join-Path $root 'manifest.json'
    if (!(Test-Path -LiteralPath $manifestPath -PathType Leaf)) {
        throw "Recovery manifest not found: $manifestPath"
    }
    $manifest = Get-Content -LiteralPath $manifestPath -Raw | ConvertFrom-Json
    if ([int]$manifest.version -ne 1) { throw 'Unsupported recovery manifest version' }
    $files = @($manifest.files)
    if ($files.Count -lt 5) { throw 'Recovery manifest has too few files' }

    foreach ($entry in $files) {
        $rel = [string]$entry.path
        if ([string]::IsNullOrWhiteSpace($rel) -or [IO.Path]::IsPathRooted($rel) -or $rel -match '(^|[\\/])\.\.([\\/]|$)') {
            throw "Unsafe recovery path: $rel"
        }
        $path = [IO.Path]::GetFullPath((Join-Path $root $rel))
        $prefix = $root.TrimEnd('\') + '\'
        if ($path -ne $root -and -not $path.StartsWith($prefix,[StringComparison]::OrdinalIgnoreCase)) {
            throw "Recovery path escapes root: $rel"
        }
        if (!(Test-Path -LiteralPath $path -PathType Leaf)) {
            throw "Recovery file missing: $rel"
        }
        $actual = (Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash
        if ($actual -ne [string]$entry.sha256) {
            throw "Recovery hash mismatch: $rel"
        }
    }
    return $manifest
}

Export-ModuleMember -Function Get-CutoverGuardDecision,Test-RecoverySnapshot
