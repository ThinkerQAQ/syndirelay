# Adapted from VidOtter's local extension guide; see third_party/vidotter.
$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
& (Join-Path $root 'Install-SyndiRelay.ps1')
$folder = Join-Path $root 'extension'
if (!(Test-Path (Join-Path $folder 'manifest.json'))) { throw 'SyndiRelay extension is missing' }
Start-Process -FilePath explorer.exe -ArgumentList ('"' + $folder + '"')
$candidates = @(
    @((Join-Path ${env:ProgramFiles(x86)} 'Microsoft\Edge\Application\msedge.exe'), 'edge://extensions/'),
    @((Join-Path $env:ProgramFiles 'Microsoft\Edge\Application\msedge.exe'), 'edge://extensions/'),
    @((Join-Path $env:ProgramFiles 'Google\Chrome\Application\chrome.exe'), 'chrome://extensions/'),
    @((Join-Path ${env:ProgramFiles(x86)} 'Google\Chrome\Application\chrome.exe'), 'chrome://extensions/')
)
foreach ($candidate in $candidates) {
    if (Test-Path $candidate[0]) {
        Start-Process -FilePath $candidate[0] -ArgumentList $candidate[1]
        break
    }
}
$guide = Join-Path $root 'README_ZH.md'
if (Test-Path $guide) { Start-Process -FilePath $guide }
