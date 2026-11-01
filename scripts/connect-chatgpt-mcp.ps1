# Windows-native Secure MCP Tunnel for SyndiRelay. Does not require WSL.
[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [ValidatePattern('^tunnel_[A-Za-z0-9]+$')]
    [string]$TunnelId,
    [switch]$Status,
    [string]$TunnelClientPath,
    [string]$SyndiRelayPath,
    [string]$RuntimeKeyRef
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

function Resolve-NativeExecutable([string]$Explicit, [string[]]$Names, [string[]]$CandidatePaths) {
    if ($Explicit) {
        if (-not [System.IO.Path]::IsPathRooted($Explicit) -or -not (Test-Path -LiteralPath $Explicit -PathType Leaf)) {
            throw 'Executable override must be an existing absolute path.'
        }
        return (Resolve-Path -LiteralPath $Explicit).ProviderPath
    }
    foreach ($candidate in $CandidatePaths) {
        if ($candidate -and (Test-Path -LiteralPath $candidate -PathType Leaf)) {
            return (Resolve-Path -LiteralPath $candidate).ProviderPath
        }
    }
    foreach ($name in $Names) {
        $command = Get-Command $name -CommandType Application -ErrorAction SilentlyContinue
        if ($command) { return $command.Source }
    }
    throw ('Native Windows executable not found: ' + ($Names -join ', '))
}

$installRoot = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..')).ProviderPath
$clientCandidates = @(
    (Join-Path $installRoot 'tunnel-client.exe'),
    (Join-Path $installRoot 'bin\tunnel-client.exe')
)
$client = Resolve-NativeExecutable $TunnelClientPath @('tunnel-client.exe') $clientCandidates
if ($client -match '(?i)wsl\.exe$|/mnt/|\\wsl\\') { throw 'Windows setup must use the native tunnel-client.exe.' }

if ($Status) {
    & $client runtimes status syndirelay --json
    if ($LASTEXITCODE -ne 0) { throw 'SyndiRelay Tunnel is not ready.' }
    return
}

$server = Resolve-NativeExecutable $SyndiRelayPath @('syndirelay.exe', 'syndirelay-windows-amd64.exe') @(
    (Join-Path $installRoot 'syndirelay-windows-amd64.exe'),
    (Join-Path $installRoot 'syndirelay.exe')
)
if ($server -match '(?i)wsl\.exe$|/mnt/|\\wsl\\') { throw 'Use the Windows SyndiRelay executable.' }

# A tunnel runtime needs an authorized credential, not a new per-project or
# one-time API key. Reuse an already authorized Windows environment reference.
# Never read or copy another application's profile or store the key in Git.
if (-not $RuntimeKeyRef) { $RuntimeKeyRef = $env:SYNDIRELAY_RUNTIME_KEY_REF }
if (-not $RuntimeKeyRef -and $env:CONTROL_PLANE_API_KEY) {
    $RuntimeKeyRef = 'env:CONTROL_PLANE_API_KEY'
}
if (-not $RuntimeKeyRef) {
    throw 'Set CONTROL_PLANE_API_KEY to an already-authorized Tunnel runtime credential, or provide -RuntimeKeyRef env:NAME / file:C:\private\key. A new key is not required per application.'
}
if ($RuntimeKeyRef -match '^env:([A-Za-z_][A-Za-z0-9_]*)$') {
    $variableName = $Matches[1]
    $credential = [Environment]::GetEnvironmentVariable($variableName, 'Process')
    if (-not $credential) { throw 'The selected runtime credential environment variable is unset.' }
} elseif ($RuntimeKeyRef -match '^file:(.+)$') {
    $keyFile = $Matches[1]
    if (-not [IO.Path]::IsPathRooted($keyFile) -or -not (Test-Path -LiteralPath $keyFile -PathType Leaf)) {
        throw 'Runtime credential must reference an existing absolute file path.'
    }
} else {
    throw 'RuntimeKeyRef must use env:NAME or file:ABSOLUTE_PATH, not a literal secret.'
}

$profileDir = Join-Path $env:LOCALAPPDATA 'SyndiRelay\tunnel'
if (-not (Test-Path -LiteralPath $profileDir)) {
    New-Item -ItemType Directory -Path $profileDir -Force | Out-Null
}
$serverCommand = '"' + $server.Replace('"', '\"') + '" mcp-stdio'
& $client runtimes connect `
    --alias syndirelay `
    --name SyndiRelay `
    --tunnel-id $TunnelId `
    --profile syndirelay `
    --profile-dir $profileDir `
    --runtime-api-key $RuntimeKeyRef `
    --mcp-command $serverCommand `
    --json
if ($LASTEXITCODE -ne 0) { throw 'Windows-native Tunnel failed to connect. Check runtime authorization.' }
& $client runtimes status syndirelay --json
if ($LASTEXITCODE -ne 0) { throw 'Windows-native Tunnel is not healthy.' }
