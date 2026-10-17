[CmdletBinding()]
param()
$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path -Parent $PSScriptRoot
& dotnet publish (Join-Path $repoRoot 'tools/battery-hub/BatteryProbe.csproj') -c Release -o (Join-Path $repoRoot '.local/bin/battery-hub')
if ($LASTEXITCODE -ne 0) { throw 'Battery probe build failed.' }
