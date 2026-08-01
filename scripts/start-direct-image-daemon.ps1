param(
    [Parameter(Mandatory = $false)]
    [SecureString]$ImageApiKey,

    [ValidateRange(5, 12)]
    [int]$MaxConcurrentTasks = 8
)

$ErrorActionPreference = 'Stop'
$cli = 'C:\Users\zhangzhenyu\.multica\bin\multica.exe'
$profileDir = 'C:\Users\zhangzhenyu\.multica\profiles\direct-image2'
$secretPath = Join-Path $profileDir 'image-api-key.dpapi'

if (-not (Test-Path -LiteralPath $cli)) {
    throw "Multica CLI not found: $cli"
}

if (-not $ImageApiKey -and $env:MULTICA_IMAGE_API_KEY) {
    $ImageApiKey = ConvertTo-SecureString $env:MULTICA_IMAGE_API_KEY -AsPlainText -Force
}
if (-not $ImageApiKey -and (Test-Path -LiteralPath $secretPath)) {
    $encrypted = (Get-Content -Raw -LiteralPath $secretPath).Trim()
    $ImageApiKey = ConvertTo-SecureString $encrypted
}
if (-not $ImageApiKey) {
    $ImageApiKey = Read-Host 'Image API key' -AsSecureString
}

New-Item -ItemType Directory -Path $profileDir -Force | Out-Null
$ImageApiKey | ConvertFrom-SecureString | Set-Content -LiteralPath $secretPath -Encoding ascii

$credential = [System.Net.NetworkCredential]::new('', $ImageApiKey)
try {
    $env:OPENAI_API_KEY = $credential.Password
    $env:OPENAI_BASE_URL = 'http://one-ai.adakamicorp.id'
    $env:OPENAI_IMAGE_EDIT_PATH = '/images/edits'
    $env:OPENAI_IMAGE_FILE_FIELD = 'image'

    & $cli --profile direct-image2 daemon stop
    & $cli --profile direct-image2 daemon start --no-auto-update --max-concurrent-tasks $MaxConcurrentTasks
    & $cli --profile direct-image2 daemon status
}
finally {
    Remove-Item Env:OPENAI_API_KEY -ErrorAction SilentlyContinue
    Remove-Item Env:OPENAI_BASE_URL -ErrorAction SilentlyContinue
    Remove-Item Env:OPENAI_IMAGE_EDIT_PATH -ErrorAction SilentlyContinue
    Remove-Item Env:OPENAI_IMAGE_FILE_FIELD -ErrorAction SilentlyContinue
}
