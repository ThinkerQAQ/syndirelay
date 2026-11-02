# Start first-use workflow without modifying other applications.
$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $MyInvocation.MyCommand.Path
& (Join-Path $root 'Install-SyndiRelay.ps1')
$folder = Join-Path $root 'extension'
if (!(Test-Path -LiteralPath (Join-Path $folder 'manifest.json'))) { throw 'Extension missing' }
Start-Process -FilePath explorer.exe -ArgumentList ('"' + $folder + '"')
foreach ($browser in @(
  (Join-Path $env:ProgramFiles 'Microsoft\Edge\Application\msedge.exe'),
  (Join-Path ${env:ProgramFiles(x86)} 'Microsoft\Edge\Application\msedge.exe'),
  (Join-Path $env:ProgramFiles 'Google\Chrome\Application\chrome.exe'),
  (Join-Path ${env:ProgramFiles(x86)} 'Google\Chrome\Application\chrome.exe'))) {
  if (Test-Path -LiteralPath $browser) {
    $url = if ($browser -match 'msedge.exe$') { 'edge://extensions/' } else { 'chrome://extensions/' }
    Start-Process -FilePath $browser -ArgumentList $url
    break
  }
}
