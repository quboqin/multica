param(
    [ValidateRange(1, 20)]
    [int]$Concurrency = 8,

    [Parameter(Mandatory = $true)]
    [string]$InputFile,

    [ValidateRange(1, 30)]
    [int]$TimeoutMinutes = 15,

    [string]$Profile = 'direct-image2'
)

$ErrorActionPreference = 'Stop'
$userProfile = [Environment]::GetFolderPath('UserProfile')
$multicaHome = Join-Path $userProfile '.multica'
$cli = Join-Path $multicaHome 'bin\multica.exe'
$profileDir = Join-Path (Join-Path $multicaHome 'profiles') $Profile
$secretPath = Join-Path $profileDir 'image-api-key.dpapi'
$resolvedInput = (Resolve-Path -LiteralPath $InputFile).Path
$repoRoot = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..')).Path
$runId = Get-Date -Format 'yyyyMMdd-HHmmss'
$runDir = Join-Path $repoRoot "tmp\image-concurrency-$runId-c$Concurrency"
$promptPath = Join-Path $runDir 'prompt.txt'

if (-not (Test-Path -LiteralPath $cli -PathType Leaf)) {
    throw "Multica CLI not found: $cli"
}
if (-not (Test-Path -LiteralPath $secretPath -PathType Leaf)) {
    throw "Encrypted image API key not found: $secretPath"
}
if (-not (Test-Path -LiteralPath $resolvedInput -PathType Leaf)) {
    throw "Input image not found: $resolvedInput"
}

New-Item -ItemType Directory -Force -Path $runDir | Out-Null
@'
Concurrency capacity probe. Preserve the reference composition. Produce a clean, simple advertising draft with no logo, QR code, legal text, watermark, or additional claims. This output is only for provider capacity testing.
'@ | Set-Content -LiteralPath $promptPath -Encoding utf8NoBOM

$encrypted = (Get-Content -Raw -LiteralPath $secretPath).Trim()
$secureKey = ConvertTo-SecureString $encrypted
$credential = [System.Net.NetworkCredential]::new('', $secureKey)
$oldApiKey = $env:OPENAI_API_KEY
$oldBaseUrl = $env:OPENAI_BASE_URL
$oldEditPath = $env:OPENAI_IMAGE_EDIT_PATH
$oldFileField = $env:OPENAI_IMAGE_FILE_FIELD
$oldImageConcurrency = $env:MULTICA_IMAGE_MAX_CONCURRENT

try {
    $env:OPENAI_API_KEY = $credential.Password
    $env:OPENAI_BASE_URL = 'http://one-ai.adakamicorp.id'
    $env:OPENAI_IMAGE_EDIT_PATH = '/images/edits'
    $env:OPENAI_IMAGE_FILE_FIELD = 'image'
    $env:MULTICA_IMAGE_MAX_CONCURRENT = '20'

    $startedAt = Get-Date
    $processes = foreach ($index in 1..$Concurrency) {
        $outputFile = Join-Path $runDir ("output-{0:D2}.png" -f $index)
        $stdoutFile = Join-Path $runDir ("stdout-{0:D2}.json" -f $index)
        $stderrFile = Join-Path $runDir ("stderr-{0:D2}.log" -f $index)
        $arguments = @(
            '--profile', $Profile,
            'image', 'edit',
            '--input', $resolvedInput,
            '--prompt-file', $promptPath,
            '--size', '1088x1088',
            '--max-attempts', '1',
            '--output-file', $outputFile,
            '--output', 'json'
        )
        $process = Start-Process -FilePath $cli -ArgumentList $arguments -PassThru -WindowStyle Hidden `
            -RedirectStandardOutput $stdoutFile -RedirectStandardError $stderrFile
        [pscustomobject]@{
            Index = $index
            Process = $process
            OutputFile = $outputFile
            StdoutFile = $stdoutFile
            StderrFile = $stderrFile
        }
    }

    $deadline = $startedAt.AddMinutes($TimeoutMinutes)
    foreach ($entry in $processes) {
        $remaining = [Math]::Max(0, [int]($deadline - (Get-Date)).TotalMilliseconds)
        if (-not $entry.Process.WaitForExit($remaining)) {
            $entry.Process.Kill($true)
        }
    }

    $completedAt = Get-Date
    $results = foreach ($entry in $processes) {
        $stdout = if (Test-Path -LiteralPath $entry.StdoutFile) { ([string](Get-Content -Raw -LiteralPath $entry.StdoutFile)).Trim() } else { '' }
        $stderr = if (Test-Path -LiteralPath $entry.StderrFile) { ([string](Get-Content -Raw -LiteralPath $entry.StderrFile)).Trim() } else { '' }
        $payload = $null
        if ($stdout) {
            try { $payload = $stdout | ConvertFrom-Json } catch { $payload = $null }
        }
        $requestId = if ($payload) {
            $payload.request_id
        } elseif ($stderr -match 'request_id=([0-9a-f-]+)') {
            $Matches[1]
        } else {
            ''
        }
        $category = if ($entry.Process.ExitCode -eq 0) {
            'success'
        } elseif ($stderr -match '(?i)(429|too many requests|rate.?limit)') {
            'rate_limited'
        } elseif ($stderr -match '(?i)(408|timeout|deadline exceeded)') {
            'timeout'
        } elseif ($stderr -match '(?i)(status (5\d\d)|server error)') {
            'provider_5xx'
        } else {
            'other_error'
        }
        [pscustomobject]@{
            index = $entry.Index
            status = $category
            exit_code = $entry.Process.ExitCode
            request_id = $requestId
            attempts = if ($payload) { $payload.attempts } else { 1 }
            bytes = if ($payload) { $payload.bytes } else { 0 }
        }
    }

    $summary = [ordered]@{
        concurrency = $Concurrency
        wall_seconds = [Math]::Round(($completedAt - $startedAt).TotalSeconds, 1)
        success = @($results | Where-Object status -eq 'success').Count
        rate_limited = @($results | Where-Object status -eq 'rate_limited').Count
        timeout = @($results | Where-Object status -eq 'timeout').Count
        provider_5xx = @($results | Where-Object status -eq 'provider_5xx').Count
        other_error = @($results | Where-Object status -eq 'other_error').Count
        run_directory = $runDir
        results = $results
    }
    $summary | ConvertTo-Json -Depth 5
}
finally {
    if ($null -eq $oldApiKey) { Remove-Item Env:OPENAI_API_KEY -ErrorAction SilentlyContinue } else { $env:OPENAI_API_KEY = $oldApiKey }
    if ($null -eq $oldBaseUrl) { Remove-Item Env:OPENAI_BASE_URL -ErrorAction SilentlyContinue } else { $env:OPENAI_BASE_URL = $oldBaseUrl }
    if ($null -eq $oldEditPath) { Remove-Item Env:OPENAI_IMAGE_EDIT_PATH -ErrorAction SilentlyContinue } else { $env:OPENAI_IMAGE_EDIT_PATH = $oldEditPath }
    if ($null -eq $oldFileField) { Remove-Item Env:OPENAI_IMAGE_FILE_FIELD -ErrorAction SilentlyContinue } else { $env:OPENAI_IMAGE_FILE_FIELD = $oldFileField }
    if ($null -eq $oldImageConcurrency) { Remove-Item Env:MULTICA_IMAGE_MAX_CONCURRENT -ErrorAction SilentlyContinue } else { $env:MULTICA_IMAGE_MAX_CONCURRENT = $oldImageConcurrency }
}
