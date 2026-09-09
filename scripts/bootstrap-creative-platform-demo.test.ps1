$ErrorActionPreference = 'Stop'
$tokens = $null
$parseErrors = $null
$path = Join-Path $PSScriptRoot 'bootstrap-creative-platform-demo.ps1'
$ast = [System.Management.Automation.Language.Parser]::ParseFile($path, [ref]$tokens, [ref]$parseErrors)
if ($parseErrors.Count -gt 0) { throw ($parseErrors | Out-String) }
$assignment = $ast.Find({ param($node)
    $node -is [System.Management.Automation.Language.AssignmentStatementAst] -and $node.Left.Extent.Text -eq '$seedMarketPack'
}, $true)
if (-not $assignment) { throw 'Market seed guard is missing' }
$guard = [scriptblock]::Create($assignment.Right.Extent.Text)
foreach ($marketPackIsNew in @($false, $true)) {
    foreach ($ResetBusinessConfig in @($false, $true)) {
        foreach ($status in @('draft', 'published')) {
            $marketPack = [pscustomobject]@{ status = $status; config = @{ prime_composition_mode = 'model_integrated' } }
            $result = & $guard
            if ($result -ne ($marketPackIsNew -or $ResetBusinessConfig)) {
                throw "Existing user settings were selected for reset: new=$marketPackIsNew reset=$ResetBusinessConfig status=$status"
            }
        }
    }
}
Write-Output 'Market bootstrap guard: 8 cases passed'

$agentDefinition = $ast.Find({ param($node)
    $node -is [System.Management.Automation.Language.FunctionDefinitionAst] -and $node.Name -eq 'Set-AgentDefinition'
}, $true)
if (-not $agentDefinition) { throw 'Agent definition updater is missing' }
. ([scriptblock]::Create($agentDefinition.Extent.Text))
function Get-Items { param($Value, $Key) return @($Value) }
$script:updatedAgentBody = $null
$script:existingAgent = [pscustomobject]@{ id = 'agent'; name = '素材_出图'; model = 'gpt-6-astra'; thinking_level = 'xhigh'; max_concurrent_tasks = 50 }
function Invoke-MulticaApi {
    param($Method, $Path, $Body)
    if ($Method -eq 'Get') { return $script:existingAgent }
    if ($Path -eq '/api/agents/agent') { $script:updatedAgentBody = $Body; return $script:existingAgent }
}
Set-AgentDefinition -Name '素材_出图' -Description 'Updated description' -Instructions 'Updated instructions' -SkillIDs @('skill') -Model 'default-model' -ThinkingLevel 'low' -MaxConcurrentTasks 10 | Out-Null
if ($script:updatedAgentBody.model -ne 'gpt-6-astra' -or $script:updatedAgentBody.thinking_level -ne 'xhigh' -or $script:updatedAgentBody.max_concurrent_tasks -ne 50) { throw 'Agent execution preferences were overwritten' }
Write-Output 'Agent bootstrap preserves model, thinking level and concurrency'
