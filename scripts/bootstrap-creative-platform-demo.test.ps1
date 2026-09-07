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
