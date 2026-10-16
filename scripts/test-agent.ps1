[CmdletBinding()]
param(
    [ValidatePattern('^[A-Za-z0-9][A-Za-z0-9_.-]*\.exe$')]
    [string]$OutputName = 'zerobot-agent-validation.exe'
)

$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'windows-service-identity.ps1')
$repoRoot = Resolve-LocalServicePath (Split-Path -Parent $PSScriptRoot)
$runName = 'run-' + (Get-Date -Format 'yyyyMMdd-HHmmss') + '-' + [Guid]::NewGuid().ToString('N').Substring(0, 8)
$runDirectory = Join-Path $repoRoot (Join-Path '.local/agent-validation' $runName)
New-Item -ItemType Directory -Force -Path $runDirectory | Out-Null
$reportPath = Join-Path $runDirectory 'result.json'
$report = [ordered]@{
    passed = $false
    startedAt = [DateTimeOffset]::UtcNow.ToString('o')
    completedAt = $null
    binary = '.local/bin/' + $OutputName
    binarySHA256 = $null
    integrationResult = $null
    steps = [Collections.Generic.List[object]]::new()
}

function Invoke-AgentCheck {
    param([string]$Name, [string]$Directory, [string]$Command, [string[]]$Arguments)
    $timer = [Diagnostics.Stopwatch]::StartNew()
    $exitCode = 1
    $logPath = Join-Path $runDirectory ($Name + '.log')
    Push-Location $Directory
    try {
        Write-Host "[$Name] $Command $($Arguments -join ' ')"
        & $Command @Arguments 2>&1 | Tee-Object -FilePath $logPath
        $exitCode = $LASTEXITCODE
        if ($exitCode -ne 0) { throw "Agent validation failed: $Name (exit $exitCode). See $logPath" }
    } finally {
        $timer.Stop()
        $report.steps.Add([ordered]@{ name = $Name; exitCode = $exitCode; seconds = [Math]::Round($timer.Elapsed.TotalSeconds, 3); log = $logPath })
        Pop-Location
    }
}

$previousCgo = $env:CGO_ENABLED
try {
    $env:CGO_ENABLED = '0'
    Invoke-AgentCheck 'sdk' (Join-Path $repoRoot 'third_party/go-onebot-agent') 'go' @('test', './...', '-count=1', '-timeout=180s')
    Invoke-AgentCheck 'chat' (Join-Path $repoRoot 'third_party/zbputils') 'go' @('test', './chat', '-count=1', '-timeout=180s')
    Invoke-AgentCheck 'backend' $repoRoot 'go' @('test', './internal/localadmin', './plugin/aichat', './plugin/llm', './internal/modelrequest', './internal/modelstats', './internal/replystats', '-count=1', '-timeout=180s')
    Invoke-AgentCheck 'smoke-tests' $repoRoot 'go' @('test', 'scripts/smoke-local-admin.go', 'scripts/smoke-local-admin_test.go', '-count=1', '-timeout=120s')
    Invoke-AgentCheck 'model-provider-tests' $repoRoot 'go' @('test', 'scripts/test-real-model.go', 'scripts/test-real-model_test.go', '-count=1', '-timeout=120s')
    foreach ($asset in @('agent-archive', 'agent-memory', 'agent-tasks', 'ai')) {
        Invoke-AgentCheck ('syntax-' + $asset) $repoRoot 'node' @('--check', "internal/localadmin/assets/$asset.js")
    }
    Invoke-AgentCheck 'browser' $repoRoot 'node' @('--test', 'scripts/test-agent-archive-ui.mjs', 'scripts/test-agent-memory-ui.mjs', 'scripts/test-agent-tasks-ui.mjs', 'scripts/test-ai-draft-ui.mjs', 'scripts/test-conversation-shortcuts-ui.mjs')
    New-Item -ItemType Directory -Force -Path (Join-Path $repoRoot '.local/bin') | Out-Null
    Invoke-AgentCheck 'build' $repoRoot 'go' @('build', '-trimpath', '-o', $report.binary, '.')
    $report.binarySHA256 = (Get-FileHash -LiteralPath (Join-Path $repoRoot $report.binary) -Algorithm SHA256).Hash
    Invoke-AgentCheck 'model-provider-integration' $repoRoot 'go' @('test', 'scripts/test-real-model.go', 'scripts/test-real-model_test.go', '-count=1', '-run=^TestRealModelIsolatedNamedRuntime$', '-timeout=180s', '-args', '-validation-binary', $OutputName)
    Invoke-AgentCheck 'integration' $repoRoot 'go' @('run', 'scripts/smoke-local-admin.go', '-binary-name', $OutputName)
    $artifactLine = Select-String -LiteralPath (Join-Path $runDirectory 'integration.log') -Pattern '^ARTIFACTS=(.+)$'
    $report.integrationResult = Join-Path $artifactLine.Matches[0].Groups[1].Value.Trim() 'result.json'
    $integration = Get-Content -LiteralPath $report.integrationResult -Raw | ConvertFrom-Json
    $report.passed = $integration.passed -eq $true -and $integration.isolated -eq $true -and $integration.realQQMessagesSent -eq $false
    if (-not $report.passed) { throw 'Isolated integration result did not pass.' }
} catch {
    $report.error = $_.Exception.Message
    throw
} finally {
    $env:CGO_ENABLED = $previousCgo
    $report.completedAt = [DateTimeOffset]::UtcNow.ToString('o')
    [IO.File]::WriteAllText($reportPath, ($report | ConvertTo-Json -Depth 5), [Text.UTF8Encoding]::new($false))
    Write-Host "AGENT_VALIDATION_RESULT=$reportPath"
}
