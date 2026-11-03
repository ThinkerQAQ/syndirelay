# Windows-native per-user Tunnel lifecycle. Never stops other profiles.
param([Parameter(Mandatory=$true)][ValidateSet('start','stop','restart')][string]$Action)
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$private = Join-Path $env:LOCALAPPDATA 'SyndiRelay\MCP'
$profile = Join-Path $private 'syndirelay.yaml'
$runner = Join-Path $private 'run-mcp.ps1'
if (!(Test-Path -LiteralPath $profile) -or !(Test-Path -LiteralPath $runner)) {
    throw 'SyndiRelay Tunnel is not configured yet.'
}

function Get-SyndiRelayTunnel {
    # The complete profile path, rather than the executable name, owns the process.
    # Compare without case sensitivity so alternate drive/path casing cannot create duplicates.
    @(Get-CimInstance Win32_Process -Filter "Name='tunnel-client.exe'" -ErrorAction Stop |
        Where-Object {
            $_.CommandLine -and $_.CommandLine.IndexOf($profile, [StringComparison]::OrdinalIgnoreCase) -ge 0
        })
}
function Stop-SyndiRelayTunnel {
    foreach ($proc in @(Get-SyndiRelayTunnel)) {
        $result = Invoke-CimMethod -InputObject $proc -MethodName Terminate
        if ($result.ReturnValue -ne 0 -and $result.ReturnValue -ne 9) {
            throw ('Could not stop owned Tunnel process: ' + $result.ReturnValue)
        }
    }
    for ($i = 0; $i -lt 40; $i++) {
        if (@(Get-SyndiRelayTunnel).Count -eq 0) { return }
        Start-Sleep -Milliseconds 200
    }
    throw 'SyndiRelay Tunnel did not stop before the deadline'
}
function Start-SyndiRelayTunnel {
    $running = @(Get-SyndiRelayTunnel)
    if ($running.Count -gt 1) {
        # Competing stdio children split initialize and tool calls. Recover by
        # stopping all instances associated with this exact profile.
        Stop-SyndiRelayTunnel
        $running = @()
    }
    if ($running.Count -eq 1) { return }
    $launcher = Start-Process -FilePath (Get-Command powershell.exe).Source -ArgumentList @(
        '-NoProfile','-WindowStyle','Hidden','-File',('"' + $runner + '"')
    ) -WindowStyle Hidden -PassThru
    # Wait for the actual tunnel-client child, not just its PowerShell launcher.
    # Without this wait a second setup/start can race and launch another child.
    for ($i = 0; $i -lt 60; $i++) {
        $current = @(Get-SyndiRelayTunnel)
        if ($current.Count -eq 1) { return }
        if ($current.Count -gt 1) {
            Stop-SyndiRelayTunnel
            throw 'Concurrent Tunnel launch detected; all duplicate instances were stopped'
        }
        if ($launcher.HasExited) { throw 'Tunnel launcher exited before the client started' }
        Start-Sleep -Milliseconds 200
    }
    throw 'Tunnel client did not start; check the local setup and runtime key'
}

# Serialize lifecycle operations across Setup, Startup and Settings actions.
# The mutex is per Windows session. It is not shared with other applications.
$mutex = New-Object System.Threading.Mutex($false, 'Local\ThinkerQAQ.SyndiRelay.MCP.Lifecycle')
$owned = $false
try {
    try { $owned = $mutex.WaitOne(20000) }
    catch [System.Threading.AbandonedMutexException] { $owned = $true }
    if (!$owned) { throw 'Another SyndiRelay Tunnel lifecycle operation is in progress' }
    if ($Action -in @('stop','restart')) { Stop-SyndiRelayTunnel }
    if ($Action -in @('start','restart')) { Start-SyndiRelayTunnel }
} finally {
    if ($owned) { $mutex.ReleaseMutex() }
    $mutex.Dispose()
}
Write-Host "SyndiRelay Tunnel action $Action completed; check readiness in SyndiRelay Settings."
