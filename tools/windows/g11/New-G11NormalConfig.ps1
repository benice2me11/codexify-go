# G11 normal production profile candidate: prepare only, never install/restart.
# Changes exactly the ASCII JSON token mcp.diagnostics.enabled true -> false;
# every other source byte, including BOM/newlines/tunnel/upstreams, is preserved.
[CmdletBinding()]
param(
    [Parameter(Mandatory)][string]$SourceConfig,
    [Parameter(Mandatory)][string]$ExpectedSourceSha256,
    [Parameter(Mandatory)][string]$OutputConfig,
    [switch]$Execute
)
$ErrorActionPreference='Stop'
$source=[IO.Path]::GetFullPath($SourceConfig)
$target=[IO.Path]::GetFullPath($OutputConfig)
if([string]::Equals($source,$target,[StringComparison]::OrdinalIgnoreCase)){
    throw 'Cannot overwrite running production config'
}
$private=([IO.Path]::GetFullPath((Join-Path $env:USERPROFILE '.codexify-go\staging'))).TrimEnd('\')+'\'
if(!$target.StartsWith($private,[StringComparison]::OrdinalIgnoreCase)){
    throw 'Sensitive config candidates must remain outside Git in private staging'
}
if(Test-Path -LiteralPath $target){throw 'Refusing to overwrite existing config candidate'}
if($ExpectedSourceSha256 -notmatch '^[a-fA-F0-9]{64}$'){throw 'Expected pinned source SHA256'}
$actual=(Get-FileHash -LiteralPath $source -Algorithm SHA256 -ErrorAction Stop).Hash
if($actual -ne $ExpectedSourceSha256.ToUpperInvariant()){throw 'Production source config SHA mismatch'}
$bytes=[IO.File]::ReadAllBytes($source)
$utf8=New-Object System.Text.UTF8Encoding($false,$true)
$raw=$utf8.GetString($bytes)
$before= $raw|ConvertFrom-Json -ErrorAction Stop
if($before.mcp.projectGroupsEnabled -ne $false -or $before.mcp.diagnostics.enabled -ne $true){
    throw 'This transition is only for diagnostics=true, Project Groups=false'
}
$pattern='(?<prefix>"diagnostics"\s*:\s*\{\s*"enabled"\s*:\s*)true\b'
$matches=[regex]::Matches($raw,$pattern,[Text.RegularExpressions.RegexOptions]::CultureInvariant)
if($matches.Count -ne 1){throw 'Refusing ambiguous diagnostics-token substitution'}
$matched=$matches[0]
$start=$matched.Index+$matched.Groups['prefix'].Length
$prefixCount=$utf8.GetByteCount($raw.Substring(0,$start))
$suffixStart=$prefixCount+4
if(($utf8.GetString($bytes,$prefixCount,4)) -cne 'true'){throw 'Malformed UTF8 substitution boundary'}
$out=New-Object byte[] ($bytes.Length+1)
[Array]::Copy($bytes,0,$out,0,$prefixCount)
[Array]::Copy($utf8.GetBytes('false'),0,$out,$prefixCount,5)
[Array]::Copy($bytes,$suffixStart,$out,$prefixCount+5,$bytes.Length-$suffixStart)
$after=$utf8.GetString($out)|ConvertFrom-Json -ErrorAction Stop
$before.mcp.diagnostics.enabled=$false
if(($before|ConvertTo-Json -Depth 100 -Compress) -cne ($after|ConvertTo-Json -Depth 100 -Compress)){
    throw 'Other JSON semantics changed unexpectedly'
}
if($after.mcp.projectGroupsEnabled -ne $false){throw 'Project Groups must remain disabled'}
$sha=[Security.Cryptography.SHA256]::Create()
try{$targetHash=[BitConverter]::ToString($sha.ComputeHash($out)).Replace('-','')}finally{$sha.Dispose()}
if($Execute){
    New-Item -ItemType Directory -Path (Split-Path -Parent $target) -Force|Out-Null
    [IO.File]::WriteAllBytes($target,$out)
    if((Get-FileHash -LiteralPath $target -Algorithm SHA256).Hash -ne $targetHash){
        throw 'Written normal candidate failed SHA256 verification'
    }
}
[pscustomobject]@{
    status=$(if($Execute){'PRIVATE_CANDIDATE_PREPARED_NOT_ACTIVE'}else{'PLAN_ONLY'})
    sourceSha256=$actual;candidateSha256=$targetHash
    changedField='mcp.diagnostics.enabled'
    oldValue=$true;newValue=$false;projectGroupsEnabled=$false
    sourceBytes=$bytes.Length;candidateBytes=$out.Length
    restOfFileByteIdentical=$true;serviceMutated=$false
}|ConvertTo-Json -Compress
