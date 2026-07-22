param(
  [string]$IssueId = "ec7b81c7-252b-41d2-a44e-fa7b4fc91eb1",
  [string]$WorkspaceId = "ad81666a-0eba-4100-8297-98fe79d4ec5b",
  [string]$ActorId = "5ec0e491-62d4-4759-91bd-009cf9224ec4"
)

$ErrorActionPreference = "Stop"
$leanOutputRoot = "C:\Users\zhangzhenyu\ad-creative-factory-lean\outputs"
$imports = @(
  @{ id="lean_20260717_162530_33e3b749"; title="Local import: repayment plan cards"; competitor="Adapundi"; source="35d2614bf69f67111ddb269cfaadc69b.webp" },
  @{ id="lean_20260717_162532_17ad81ae"; title="Local import: interest and app layout"; competitor="Easycash"; source="0e60749152922a56685666524f823a74.webp" },
  @{ id="lean_20260717_162533_cb417889"; title="Local import: rate and benefit cards"; competitor="Easycash"; source="0f921d177f690039d7bd0ce367822500.webp" }
)

function SqlQuote([string]$value) {
  return "'" + $value.Replace("'", "''") + "'"
}

$jobId = [guid]::NewGuid().ToString()
$processCandidates = @()
$sql = [System.Collections.Generic.List[string]]::new()
$sql.Add("BEGIN;")

foreach ($entry in $imports) {
  $candidateId = [guid]::NewGuid().ToString()
  $entry.candidate_id = $candidateId
  $root = "http://127.0.0.1:8010/files/$($entry.id)"
  $record = Get-Content -LiteralPath (Join-Path $leanOutputRoot "$($entry.id)\status.json") -Raw | ConvertFrom-Json
  $rounds = @()
  foreach ($round in @($record.process_data.rounds)) {
    $verdicts = @()
    foreach ($verdict in @($round.verdicts)) {
      $copy = [ordered]@{}
      foreach ($property in $verdict.PSObject.Properties) { $copy[$property.Name] = $property.Value }
      $artifacts = [ordered]@{}
      if ($verdict.artifacts) {
        foreach ($property in $verdict.artifacts.PSObject.Properties) { $artifacts[$property.Name] = $property.Value }
      }
      if ($artifacts.image) { $artifacts.image_url = "$root/$($artifacts.image)" }
      $copy.artifacts = $artifacts
      $verdicts += [pscustomobject]$copy
    }
    $roundCopy = [ordered]@{}
    foreach ($property in $round.PSObject.Properties) { $roundCopy[$property.Name] = $property.Value }
    $roundCopy.verdicts = $verdicts
    $rounds += [pscustomobject]$roundCopy
  }
  $processCandidates += [pscustomobject]@{
    candidate_id = $candidateId
    submissions = @([pscustomobject]@{
      submission_index = 1
      status = $record.status
      process = [pscustomobject]@{ rounds = $rounds }
    })
  }

  $rawJson = (@{ source="lean_local_import"; lean_job_id=$entry.id; reference=$entry.source } | ConvertTo-Json -Compress)
  $sql.Add("INSERT INTO creative_material_candidate (id, workspace_id, connector_id, external_id, dedupe_key, competitor, title, asset_type, preview_url, resource_url, poster_url, original_url, raw) VALUES ($(SqlQuote $candidateId), $(SqlQuote $WorkspaceId), 'lean_local_import', $(SqlQuote $entry.id), $(SqlQuote "lean-local-$($entry.id)"), $(SqlQuote $entry.competitor), $(SqlQuote $entry.title), 'image', $(SqlQuote "$root/reference.jpg"), $(SqlQuote "$root/reference.jpg"), $(SqlQuote "$root/reference.jpg"), $(SqlQuote "$root/reference.jpg"), $(SqlQuote $rawJson)::jsonb);")
  $sql.Add("INSERT INTO creative_material_issue_candidate (issue_id, candidate_id, workspace_id, status, tags, note) VALUES ($(SqlQuote $IssueId), $(SqlQuote $candidateId), $(SqlQuote $WorkspaceId), 'edited', ARRAY['temporary-import','lean-local'], 'Temporary display of a completed Lean local generation job.');")
}

$processJson = (@{ schema_version="1"; source="temporary_lean_local_import"; candidates=$processCandidates } | ConvertTo-Json -Depth 30 -Compress)
$rulesJson = (@{ mode="imported"; temporary=$true; source="lean_local_import" } | ConvertTo-Json -Compress)
$sql.Add("INSERT INTO creative_edit_job (id, workspace_id, issue_id, status, prompt, rules, created_by_type, created_by_id, external_provider, external_job_id, external_status, stage, progress, completed_at, process_data) VALUES ($(SqlQuote $jobId), $(SqlQuote $WorkspaceId), $(SqlQuote $IssueId), 'partial', 'Temporary import: 2026-07-17 AdaKami local generation results', $(SqlQuote $rulesJson)::jsonb, 'agent', $(SqlQuote $ActorId), 'lean_local_import', 'lean_batch_20260717_1625', 'partial', 'completed', 100, now(), $(SqlQuote $processJson)::jsonb);")

foreach ($entry in $imports) {
  $sql.Add("INSERT INTO creative_edit_job_candidate (job_id, candidate_id) VALUES ($(SqlQuote $jobId), $(SqlQuote $entry.candidate_id));")
  $record = Get-Content -LiteralPath (Join-Path $leanOutputRoot "$($entry.id)\status.json") -Raw | ConvertFrom-Json
  $variantIndex = 0
  foreach ($resultItem in @($record.results)) {
    $variantIndex++
    $variantId = [guid]::NewGuid().ToString()
    $sql.Add("INSERT INTO creative_edit_variant (id, job_id, candidate_id, variant_index, title, description, qc_status) VALUES ($(SqlQuote $variantId), $(SqlQuote $jobId), $(SqlQuote $entry.candidate_id), $variantIndex, $(SqlQuote "Variant $variantIndex"), $(SqlQuote "Imported from $($entry.source); Lean job $($entry.id)."), 'passed');")
    foreach ($asset in @($resultItem.assets)) {
      $assetId = [guid]::NewGuid().ToString()
      $assetUrl = "http://127.0.0.1:8010/files/$($entry.id)/$($asset.filename)"
      $sql.Add("INSERT INTO creative_edit_asset (id, variant_id, width, height, label, asset_url, content_type) VALUES ($(SqlQuote $assetId), $(SqlQuote $variantId), $($asset.width), $($asset.height), $(SqlQuote $asset.exact_output), $(SqlQuote $assetUrl), 'image/png');")
    }
  }
}

$sql.Add("COMMIT;")
$sql.Add("SELECT $(SqlQuote $jobId) AS imported_job_id;")
$line = Get-Content -LiteralPath (Join-Path $PSScriptRoot "..\.env") | Where-Object { $_ -match "^POSTGRES_PASSWORD=" } | Select-Object -First 1
$env:PGPASSWORD = $line.Substring("POSTGRES_PASSWORD=".Length)
($sql -join "`n") | & psql -v ON_ERROR_STOP=1 -h 127.0.0.1 -U multica -d multica
