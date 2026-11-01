# Offline contract test of the Windows-native setup; no OpenAI API calls.
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$folder = Join-Path $env:TEMP ('syndirelay-tunnel-test-' + [Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $folder -Force | Out-Null
$client = Join-Path $folder 'fake-tunnel-client.cmd'
$server = Join-Path $folder 'syndirelay-windows-amd64.exe'
$record = Join-Path $folder 'calls.log'
$script = Join-Path $PSScriptRoot 'connect-chatgpt-mcp.ps1'
$oldKey = $env:CONTROL_PLANE_API_KEY
$oldLocal = $env:LOCALAPPDATA
try {
    @'
@echo off
echo %*>>"%SYNDIRELAY_TEST_RECORD%"
if "%2"=="status" echo {"healthy":true,"ready":true}
exit /b 0
'@ | Set-Content -LiteralPath $client -Encoding ASCII
    [IO.File]::WriteAllBytes($server, [byte[]]@(77, 90))
    $env:SYNDIRELAY_TEST_RECORD = $record
    $env:LOCALAPPDATA = $folder
    $env:CONTROL_PLANE_API_KEY = $null

    $refused = $false
    try { & $script -TunnelId tunnel_test123 -TunnelClientPath $client -SyndiRelayPath $server | Out-Null } catch { $refused = $true }
    if (-not $refused) { throw 'Missing authorization should fail closed.' }
    if (Test-Path $record) { throw 'No Tunnel subprocess should start without authorization.' }

    $env:CONTROL_PLANE_API_KEY = 'offline-test-credential-placeholder'
    & $script -TunnelId tunnel_test123 -TunnelClientPath $client -SyndiRelayPath $server | Out-Null
    $calls = Get-Content -LiteralPath $record
    if ($calls.Count -ne 2) { throw 'Expected one connection and one status call.' }
    if ($calls[0] -notmatch '--tunnel-id tunnel_test123') { throw 'Tunnel ID missing.' }
    if ($calls[0] -notmatch '--alias syndirelay') { throw 'Wrong MCP alias.' }
    if ($calls[0] -notmatch '--runtime-api-key env:CONTROL_PLANE_API_KEY') { throw 'Existing environment credential reference not used.' }
    if ($calls[0] -notmatch 'syndirelay-windows-amd64.exe') { throw 'Windows-native executable missing.' }
    if ($calls[0] -match 'wsl.exe|/mnt/|ubuntu') { throw 'WSL must not be a Windows dependency.' }

    & $script -TunnelId tunnel_test123 -TunnelClientPath $client -Status | Out-Null
    if ((Get-Content -LiteralPath $record).Count -ne 3) { throw 'Status command must not reconnect.' }
    $invalid = $false
    try { & $script -TunnelId 'not-a-tunnel' -TunnelClientPath $client -Status | Out-Null } catch { $invalid = $true }
    if (-not $invalid) { throw 'Invalid Tunnel ID was accepted.' }
    Write-Output 'PASS: Windows native Tunnel alias, executable, existing credential, status and fail-closed checks'
}
finally {
    $env:CONTROL_PLANE_API_KEY = $oldKey
    $env:LOCALAPPDATA = $oldLocal
    Remove-Item Env:SYNDIRELAY_TEST_RECORD -ErrorAction SilentlyContinue
    Remove-Item -LiteralPath $folder -Recurse -Force -ErrorAction SilentlyContinue
}
