# Portable Windows initialization. Never replace local runtime data.
$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $MyInvocation.MyCommand.Path
$binary = Join-Path $root 'bin\syndirelay.exe'
if (!(Test-Path -LiteralPath $binary)) { throw 'SyndiRelay executable is missing' }
New-Item -ItemType Directory -Force -Path (Join-Path $root 'Data') | Out-Null
& $binary install
if ($LASTEXITCODE -ne 0) { throw 'Native Messaging registration failed' }
Write-Output 'SyndiRelay registered its own native host.'
