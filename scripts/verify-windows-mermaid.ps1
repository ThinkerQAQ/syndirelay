[CmdletBinding()]
param(
    [Parameter(Mandatory=$true)][string]$EngineRoot,
    [string]$NodePath = 'node'
)

Set-StrictMode -Version 2.0
$ErrorActionPreference = 'Stop'
$renderer = Join-Path $EngineRoot 'tools\blogctl\renderers\node\publishing-image.mjs'
if (-not (Test-Path -LiteralPath $renderer -PathType Leaf)) {
    throw "BlogCTL renderer not found: $renderer"
}
$tempDir = Join-Path ([System.IO.Path]::GetTempPath()) ('blogctl-windows-mermaid-e2e-' + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $tempDir -Force | Out-Null
try {
    $source = Join-Path $tempDir 'sample.mmd'
    $output = Join-Path $tempDir 'sample.png'
    [System.IO.File]::WriteAllText($source, "flowchart LR`n  A[Compile] --> B[Render]`n  B --> C[Distribute]`n", [System.Text.UTF8Encoding]::new($false))
    & $NodePath $renderer --kind mermaid --input $source --output $output --renderer '@mermaid-js/mermaid-cli@11.17.0' --width 1200 --scale 2
    if ($LASTEXITCODE -ne 0) { throw "Mermaid renderer failed with exit code $LASTEXITCODE" }
    if (-not (Test-Path -LiteralPath $output -PathType Leaf)) { throw 'Mermaid renderer returned no PNG file' }
    $bytes = [System.IO.File]::ReadAllBytes($output)
    $signature = @(137, 80, 78, 71, 13, 10, 26, 10)
    if ($bytes.Length -lt 100) { throw 'Mermaid PNG is empty or invalid' }
    for ($i = 0; $i -lt $signature.Count; $i++) {
        if ($bytes[$i] -ne $signature[$i]) { throw 'Mermaid renderer returned a non-PNG file' }
    }
    Write-Output ("Windows Mermaid end-to-end passed: {0} bytes" -f $bytes.Length)
} finally {
    Remove-Item -LiteralPath $tempDir -Recurse -Force -ErrorAction SilentlyContinue
}
