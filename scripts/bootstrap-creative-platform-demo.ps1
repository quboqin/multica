param(
    [string]$ApiUrl = 'http://127.0.0.1:8080',
    [string]$AppUrl = 'http://localhost:3000',
    [string]$WorkspaceSlug = 'ad-creative-direct-pilot',
    [string]$Token = $env:MULTICA_BOOTSTRAP_TOKEN,
    [string]$CliPath = $env:MULTICA_CLI,
    [string]$CliProfile = $env:MULTICA_BOOTSTRAP_PROFILE,
    [string]$ImageApiKey = $env:MULTICA_IMAGE_API_KEY,
    [string]$PrimeDirectory = 'E:\Documents\WXWork\1688853548483782\Cache\File\2026-07\Prime Template - PNG file',
    [string[]]$AppUIReferencePaths = @('E:\Documents\WXWork\1688853548483782\Cache\Image\2026-07\首页-新客未戳额(1).jpg'),
    [switch]$ResetBusinessConfig
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

function Get-MulticaCliProfileArgs {
    if ([string]::IsNullOrWhiteSpace($CliProfile)) { return @() }
    return @('--profile', $CliProfile)
}

function Get-MulticaConfigPath {
    $home = [Environment]::GetFolderPath('UserProfile')
    if ([string]::IsNullOrWhiteSpace($home)) { $home = $env:USERPROFILE }
    if ([string]::IsNullOrWhiteSpace($home)) { return $null }

    $base = Join-Path $home '.multica'
    if ([string]::IsNullOrWhiteSpace($CliProfile)) {
        return Join-Path $base 'config.json'
    }
    return Join-Path (Join-Path (Join-Path $base 'profiles') $CliProfile) 'config.json'
}

function Get-MulticaProfileToken {
    $configPath = Get-MulticaConfigPath
    if ([string]::IsNullOrWhiteSpace($configPath) -or -not (Test-Path -LiteralPath $configPath)) {
        return ''
    }
    try {
        $config = Get-Content -Raw -LiteralPath $configPath | ConvertFrom-Json
        if ($config.PSObject.Properties.Name -contains 'token') {
            return ([string]$config.token).Trim()
        }
    }
    catch {
        return ''
    }
    return ''
}

function Resolve-MulticaCliPath {
    if (-not [string]::IsNullOrWhiteSpace($CliPath)) {
        if (Test-Path -LiteralPath $CliPath) {
            return (Resolve-Path -LiteralPath $CliPath).Path
        }
        $configuredCommand = Get-Command $CliPath -ErrorAction SilentlyContinue
        if ($configuredCommand) { return $configuredCommand.Source }
        throw "Multica CLI not found: $CliPath"
    }

    $directImageCli = Join-Path $env:USERPROFILE '.multica\bin\direct-image2\multica.exe'
    if (Test-Path -LiteralPath $directImageCli) {
        if ([string]::IsNullOrWhiteSpace($CliProfile)) {
            $script:CliProfile = 'direct-image2'
        }
        return $directImageCli
    }

    $candidateCommands = @('multica')
    if ($env:OS -eq 'Windows_NT') {
        $candidateCommands = @('multica.com', 'multica')
    }
    foreach ($candidate in $candidateCommands) {
        $command = Get-Command $candidate -ErrorAction SilentlyContinue
        if ($command) { return $command.Source }
    }

    throw 'Set MULTICA_BOOTSTRAP_TOKEN or pass -Token. No Multica CLI was found to start browser authorization.'
}

function Invoke-MulticaCli {
    param([Parameter(Mandatory)][string[]]$Arguments)
    & $script:ResolvedCliPath @Arguments
    if ($LASTEXITCODE -ne 0) {
        throw "multica CLI failed: $($Arguments -join ' ')"
    }
}

function Set-MulticaCliBootstrapConfig {
    $profileArgs = Get-MulticaCliProfileArgs
    Invoke-MulticaCli -Arguments @($profileArgs + @('config', 'set', 'server_url', $ApiUrl)) | Out-Null
    if (-not [string]::IsNullOrWhiteSpace($AppUrl)) {
        Invoke-MulticaCli -Arguments @($profileArgs + @('config', 'set', 'app_url', $AppUrl)) | Out-Null
    }
}

function Set-MulticaCliWorkspace {
    if ([string]::IsNullOrWhiteSpace($WorkspaceSlug)) { return }
    $profileArgs = Get-MulticaCliProfileArgs
    Invoke-MulticaCli -Arguments @($profileArgs + @('workspace', 'switch', $WorkspaceSlug)) | Out-Null
}

function Resolve-BootstrapToken {
    $explicitToken = ([string]$Token).Trim()
    if (-not [string]::IsNullOrWhiteSpace($explicitToken)) {
        return $explicitToken
    }

    $script:ResolvedCliPath = Resolve-MulticaCliPath
    Set-MulticaCliBootstrapConfig

    $profileToken = Get-MulticaProfileToken
    if (-not [string]::IsNullOrWhiteSpace($profileToken)) {
        Set-MulticaCliWorkspace
        return $profileToken
    }

    $profileLabel = if ([string]::IsNullOrWhiteSpace($CliProfile)) { 'default' } else { $CliProfile }
    Write-Host "MULTICA_BOOTSTRAP_TOKEN is not set. Opening Multica login to authorize CLI profile '$profileLabel'."
    Invoke-MulticaCli -Arguments @((Get-MulticaCliProfileArgs) + @('login'))

    $profileToken = Get-MulticaProfileToken
    if ([string]::IsNullOrWhiteSpace($profileToken)) {
        throw "Authorization completed but no token was saved to CLI profile '$profileLabel'."
    }
    Set-MulticaCliWorkspace
    return $profileToken
}

$Token = Resolve-BootstrapToken
$headers = @{
    Authorization = "Bearer $Token"
    'X-Workspace-Slug' = $WorkspaceSlug
}

function Invoke-MulticaApi {
    param(
        [Parameter(Mandatory)][string]$Method,
        [Parameter(Mandatory)][string]$Path,
        [object]$Body
    )
    $arguments = @{
        Method = $Method
        Uri = "$ApiUrl$Path"
        Headers = $headers
    }
    if ($PSBoundParameters.ContainsKey('Body')) {
        $json = $Body | ConvertTo-Json -Depth 100 -Compress
        $client = [Net.Http.HttpClient]::new()
        $client.DefaultRequestHeaders.Authorization = [Net.Http.Headers.AuthenticationHeaderValue]::new('Bearer', $Token)
        $client.DefaultRequestHeaders.Add('X-Workspace-Slug', $WorkspaceSlug)
        try {
            $request = [Net.Http.HttpRequestMessage]::new([Net.Http.HttpMethod]::new($Method), "$ApiUrl$Path")
            $request.Content = [Net.Http.StringContent]::new($json, [Text.Encoding]::UTF8, 'application/json')
            $response = $client.SendAsync($request).GetAwaiter().GetResult()
            $responseBody = $response.Content.ReadAsStringAsync().GetAwaiter().GetResult()
            if (-not $response.IsSuccessStatusCode) {
                throw "$Method $Path failed with $([int]$response.StatusCode): $responseBody"
            }
            if ($responseBody) { return $responseBody | ConvertFrom-Json -Depth 100 }
            return $null
        }
        finally {
            $client.Dispose()
        }
    }
    Invoke-RestMethod @arguments
}

function Get-Items {
    param([object]$Response, [string]$Property)
    if ($null -eq $Response) { return @() }
    if ($Response.PSObject.Properties.Name -contains $Property) { return @($Response.$Property) }
    return @($Response)
}

function Get-SkillFiles {
    param([Parameter(Mandatory)][string]$Directory)
    $files = @()
    $referenceDirectory = Join-Path $Directory 'references'
    if (Test-Path -LiteralPath $referenceDirectory) {
        foreach ($file in Get-ChildItem -LiteralPath $referenceDirectory -File | Sort-Object Name) {
            $files += @{
                path = "references/$($file.Name)"
                content = Get-Content -Raw -LiteralPath $file.FullName
            }
        }
    }
    return $files
}

function Set-WorkspaceSkill {
    param(
        [Parameter(Mandatory)][string]$Name,
        [Parameter(Mandatory)][string]$Description,
        [Parameter(Mandatory)][string]$Directory,
        [Parameter(Mandatory)][hashtable]$Config,
        [string[]]$Aliases = @()
    )
    $content = Get-Content -Raw -LiteralPath (Join-Path $Directory 'SKILL.md')
    $files = Get-SkillFiles -Directory $Directory
    $skills = Get-Items (Invoke-MulticaApi -Method Get -Path '/api/skills') ''
    $existing = $skills | Where-Object { $_.name -eq $Name -or $Aliases -contains $_.name } | Select-Object -First 1
    $body = @{ name = $Name; description = $Description; content = $content; config = $Config; files = @($files) }
    if ($existing) {
        return Invoke-MulticaApi -Method Put -Path "/api/skills/$($existing.id)" -Body $body
    }
    return Invoke-MulticaApi -Method Post -Path '/api/skills' -Body $body
}

function Set-AgentDefinition {
    param(
        [Parameter(Mandatory)][string]$Name,
        [Parameter(Mandatory)][string]$Description,
        [Parameter(Mandatory)][string]$Instructions,
        [Parameter(Mandatory)][string[]]$SkillIDs,
        [string[]]$Aliases = @(),
        [string]$RuntimeID,
        [string]$Model = 'gpt-5.6-luna',
        [string]$ThinkingLevel = 'low',
        [ValidateRange(1, 12)][int]$MaxConcurrentTasks = 3
    )
    $agents = Get-Items (Invoke-MulticaApi -Method Get -Path '/api/agents') ''
    $agent = $agents | Where-Object { $_.name -eq $Name -or $Aliases -contains $_.name } | Select-Object -First 1
    if (-not $agent) {
        if (-not $RuntimeID) { throw "RuntimeID is required to create agent $Name" }
        $agent = Invoke-MulticaApi -Method Post -Path '/api/agents' -Body @{
            name = $Name
            description = $Description
            instructions = $Instructions
            runtime_id = $RuntimeID
            runtime_config = @{}
            custom_env = @{}
            custom_args = @()
            visibility = 'workspace'
            max_concurrent_tasks = $MaxConcurrentTasks
            model = $Model
            thinking_level = $ThinkingLevel
        }
    } else {
        $agent = Invoke-MulticaApi -Method Put -Path "/api/agents/$($agent.id)" -Body @{
            name = $Name
            description = $Description
            instructions = $Instructions
            visibility = 'workspace'
            max_concurrent_tasks = $MaxConcurrentTasks
            model = $Model
            thinking_level = $ThinkingLevel
        }
    }
    Invoke-MulticaApi -Method Put -Path "/api/agents/$($agent.id)/skills" -Body @{ skill_ids = $SkillIDs } | Out-Null
    return $agent
}

function Set-AgentImageCredential {
    param(
        [Parameter(Mandatory)][string]$AgentID,
        [AllowEmptyString()][string]$ApiKey
    )
    if ([string]::IsNullOrWhiteSpace($ApiKey)) {
        return $false
    }

    # The dedicated env endpoint replaces the map. Read it first so this
    # bootstrap update preserves unrelated secrets while configuring the
    # complete image-provider contract used by `multica image edit`.
    $existing = Invoke-MulticaApi -Method Get -Path "/api/agents/$AgentID/env"
    $customEnv = @{}
    if ($existing -and $existing.PSObject.Properties.Name -contains 'custom_env' -and $existing.custom_env) {
        foreach ($property in $existing.custom_env.PSObject.Properties) {
            $customEnv[$property.Name] = [string]$property.Value
        }
    }
    $customEnv['OPENAI_API_KEY'] = $ApiKey
    $customEnv['OPENAI_BASE_URL'] = 'http://one-ai.adakamicorp.id'
    $customEnv['OPENAI_IMAGE_EDIT_PATH'] = '/images/edits'
    $customEnv['OPENAI_IMAGE_FILE_FIELD'] = 'image'
    Invoke-MulticaApi -Method Put -Path "/api/agents/$AgentID/env" -Body @{ custom_env = $customEnv } | Out-Null
    return $true
}

function Add-MarketFile {
    param(
        [Parameter(Mandatory)][object]$MarketPack,
        [Parameter(Mandatory)][string]$Role,
        [Parameter(Mandatory)][string]$Label,
        [Parameter(Mandatory)][string]$Path,
        [hashtable]$Metadata = @{},
        [switch]$Multiple
    )
    if (-not $Path -or -not (Test-Path -LiteralPath $Path)) {
        throw "Resource slot $Role is empty; pass a valid source file path"
    }
    $files = Get-Items (Invoke-MulticaApi -Method Get -Path "/api/creative/resources/$($MarketPack.id)/files") 'files'
    $filename = [IO.Path]::GetFileName($Path)
    $hash = (Get-FileHash -Algorithm SHA256 -LiteralPath $Path).Hash.ToLowerInvariant()
    $fileMetadata = @{} + $Metadata
    $fileMetadata.sha256 = $hash
    $fileMetadata.source_filename = $filename
    $roleFiles = @($files | Where-Object role -eq $Role)
    $matching = $roleFiles | Where-Object {
        $_.filename -eq $filename -and $_.metadata.sha256 -eq $hash
    } | Select-Object -First 1
    if ($matching -and ($Multiple -or $roleFiles.Count -eq 1)) {
        $metadataMatches = @($matching.metadata.PSObject.Properties).Count -eq $fileMetadata.Count
        foreach ($key in $fileMetadata.Keys) {
            $actual = $matching.metadata.PSObject.Properties[$key]
            if ($null -eq $actual -or
                ($actual.Value | ConvertTo-Json -Depth 100 -Compress) -ne ($fileMetadata[$key] | ConvertTo-Json -Depth 100 -Compress)) {
                $metadataMatches = $false
                break
            }
        }
        if ($matching.label -eq $Label -and $metadataMatches) { return }
        Invoke-MulticaApi -Method Put -Path "/api/creative/resources/$($MarketPack.id)/files/$($matching.id)" -Body @{
            role = $Role
            label = $Label
            metadata = $fileMetadata
        } | Out-Null
        return
    }
    if (-not $Multiple) {
        foreach ($existing in $roleFiles) {
            Invoke-MulticaApi -Method Delete -Path "/api/creative/resources/$($MarketPack.id)/files/$($existing.id)" | Out-Null
        }
    }
    $uploaded = Invoke-RestMethod -Method Post -Uri "$ApiUrl/api/upload-file" -Headers $headers -Form @{ file = Get-Item -LiteralPath $Path }
    Invoke-MulticaApi -Method Post -Path "/api/creative/resources/$($MarketPack.id)/files" -Body @{
        attachment_id = $uploaded.id
        role = $Role
        label = $Label
        metadata = $fileMetadata
    } | Out-Null
}

$repositoryRoot = Split-Path $PSScriptRoot -Parent
$skillTemplateRoot = Join-Path $repositoryRoot 'scripts\creative-platform-skills'

# Remove the previous demo's market-pack-as-Skill and fixed-stage role Skills.
$legacySkillNames = @(
    'AdaKami Indonesia Market Pack',
    '广告参考布局分析'
)
$existingSkills = Get-Items (Invoke-MulticaApi -Method Get -Path '/api/skills') ''
foreach ($legacy in $existingSkills | Where-Object {
    $legacySkillNames -contains $_.name -or $_.name -like '广告创意生产（*'
}) {
    Invoke-MulticaApi -Method Delete -Path "/api/skills/$($legacy.id)" | Out-Null
}

$collectorSkill = Set-WorkspaceSkill -Name '素材_技能_采集' -Aliases @('AppGrowing 素材采集') -Description '创建 Crawl Run，只采集真实图片广告，并用原生 task fanout 自动预分析新增图片。' -Directory (Join-Path $skillTemplateRoot 'appgrowing-material-collector') -Config @{ kind = 'creative_role'; capability = 'material_collection'; version = 13 }
$diagnosisSkill = Set-WorkspaceSkill -Name '素材_技能_诊断' -Aliases @('创意流程诊断', '出图诊断', 'AppGrowing 采集诊断') -Description '读取创意采集、出图、Prime、QC、订单和 daemon/runtime 证据，在允许范围内恢复或给出明确动作。' -Directory (Join-Path $skillTemplateRoot 'creative-flow-diagnostician') -Config @{ kind = 'creative_role'; capability = 'crawl_diagnosis'; version = 3 }
$analysisSkill = Set-WorkspaceSkill -Name '素材_技能_分析' -Aliases @('广告参考分析') -Description '市场中立地读取真实图片，识别可变视觉区域、原图文字及坐标、主题、利益点、语义锚点、App UI 类型和布局约束。' -Directory (Join-Path $skillTemplateRoot 'ad-creative-analysis') -Config @{ kind = 'creative_role'; capability = 'reference_analysis'; version = 16 }
$preAdaptationSkill = Set-WorkspaceSkill -Name '素材_技能_文案适配' -Aliases @('广告预适配') -Description '由模型按原图视觉区域选择已审核文案 key 和还款计划 key，服务端按冻结版本派生最终文字与数值。' -Directory (Join-Path $skillTemplateRoot 'ad-creative-pre-adaptation') -Config @{ kind = 'creative_role'; capability = 'pre_adaptation'; version = 18 }
$marketPackExtractionSkill = Set-WorkspaceSkill -Name '素材_技能_组件识别' -Aliases @('市场包组件识别') -Description '读取完整成图，发现数量不定的品牌与合规组件并写回待确认候选。' -Directory (Join-Path $skillTemplateRoot 'ad-creative-market-pack-extraction') -Config @{ kind = 'creative_role'; capability = 'market_pack_component_extraction'; version = 2 }
$planSkill = Set-WorkspaceSkill -Name '素材_技能_方案' -Aliases @('广告生成方案') -Description '消费冻结分析、逐块文案与市场快照，规划 3 个同题创意变体。' -Directory (Join-Path $skillTemplateRoot 'ad-creative-plan') -Config @{ kind = 'creative_role'; capability = 'generation_plan'; version = 28 }
$productionSkill = Set-WorkspaceSkill -Name '素材_技能_出图' -Aliases @('广告图像编辑') -Description '使用冻结的逐块文字替换和 Prime 安全区护栏生成无品牌底图，保存完整模型证据。' -Directory (Join-Path $skillTemplateRoot 'ad-creative-production') -Config @{ kind = 'creative_role'; capability = 'image_edit'; version = 47 }
$directEditSkill = Set-WorkspaceSkill -Name '素材_技能_改图' -Aliases @('广告图片直接修改') -Description '按用户原话修改固定底图；正式发布按 expected_sizes 进入 Prime 与独立 QC。' -Directory (Join-Path $skillTemplateRoot 'ad-creative-direct-edit') -Config @{ kind = 'creative_role'; capability = 'direct_image_edit'; version = 7 }
$composeSkill = Set-WorkspaceSkill -Name '素材_技能_贴片' -Aliases @('Prime 完整贴图') -Description '按冻结市场合同固定坐标合成 expected_sizes，净化贴片背景并保存逐图机器证据。' -Directory (Join-Path $skillTemplateRoot 'ad-creative-prime-compose') -Config @{ kind = 'creative_role'; capability = 'prime_compose'; version = 24 }
$qcSkill = Set-WorkspaceSkill -Name '素材_技能_质检' -Aliases @('广告成图验收') -Description '独立执行 technical 或 visual QC，并通过事务 barrier 收口。' -Directory (Join-Path $skillTemplateRoot 'ad-creative-qc') -Config @{ kind = 'creative_role'; capability = 'quality_control'; version = 23 }

$agents = Get-Items (Invoke-MulticaApi -Method Get -Path '/api/agents') ''
$leaderSeed = $agents | Where-Object { $_.name -eq '素材_流程' -or $_.name -eq '素材_统筹' -or $_.name -eq '素材小队 Leader' } | Select-Object -First 1
if (-not $leaderSeed) { throw '素材_流程 does not exist' }
$runtimeID = $leaderSeed.runtime_id

$specialistHandoff = '只处理 task context 指定的对象、revision 和 scope；按绑定 Skill 写结构化领域结果和机器证据。不得创建或修改 Issue，不得用评论代替领域数据。输入、凭证、工具或写回失败时保留已成功对象，写真实 error_code/error_message 并让当前 task 失败；兄弟对象继续。'
$analyst = Set-AgentDefinition -Name '素材_分析' -Aliases @('广告参考分析智能体') -Description '按 task workflow 读取真实像素、识别可变视觉区域、写市场中立分析，并在后台完成可确认的文案与数值预适配。' -Instructions "全程使用中文。creative_reference_analysis 只写指定 candidate/version 的市场中立 Source Analysis；每个可变原图文字区块必须归入唯一 copy 或 numeric 视觉区域，不能把同一画面组件拆入两条处理路径。creative_pre_adaptation 只消费指定的冻结市场包和文案库，逐区域优先绑定已审核内容；无可用项时先给出有依据的公式计算或系统推荐并标为待确认，不能伪装成已绑定，也不能改写原图事实或虚构金融文案；creative_market_pack_component_extraction 只写指定 extraction 的待确认组件候选。三种输入和产物不得混用，不生成图片，不修改市场包或文案库。$specialistHandoff" -SkillIDs @($analysisSkill.id, $preAdaptationSkill.id, $marketPackExtractionSkill.id) -RuntimeID $runtimeID -Model 'gpt-5.6-terra' -ThinkingLevel 'medium' -MaxConcurrentTasks 6
$collector = Set-AgentDefinition -Name '素材_采集' -Aliases @('AppGrowing 素材采集智能体') -Description '按 task 配置创建 Crawl Run，只导入真实图片广告并委派新增图片分析。' -Instructions "全程使用中文。只执行 task context 和 AutoPilot 明确的 AppGrowing 查询；只导入 asset_type=image，视频、非图片和未知类型不占用采集配额。使用注入的 analysis_agent_id=$($analyst.id)，不得按名称猜测。筛选、分页、预算和 fallback 由 task/平台配置决定。结果、证据和失败写 Crawl Run；导入后用原生 fanout 委派本次新增图片，不创建 Issue，不使用测试数据。" -SkillIDs @($collectorSkill.id) -RuntimeID $runtimeID -MaxConcurrentTasks 6
$diagnostician = Set-AgentDefinition -Name '素材_诊断' -Aliases @('创意流程诊断智能体', '出图诊断智能体', 'AppGrowing 采集诊断智能体') -Description '诊断创意采集、出图、Prime、QC、订单状态和 daemon/runtime 异常，并通过平台入口执行受控恢复。' -Instructions '全程使用中文。处理 creative_crawl_diagnosis、订单短 ID、Variant 标签、页面卡片文案、报错文本和用户明确指向的创意流程诊断。先定位当前订单、order item、Variant、revision、task、daemon/runtime 与 Skill 快照证据，再给结论；需要恢复时只通过 multica CLI 或平台 API 重试、取消、fanout、推进明确授权的 Variant revision 或调用现有修复入口。不得直接写 DB、修改凭证、业务筛选、市场包、文案库或生产代码，不得把诊断图当成交付资产；修改前说明对象和原因，修改后回读验证。' -SkillIDs @($diagnosisSkill.id) -RuntimeID $runtimeID -Model 'gpt-5.6-luna' -ThinkingLevel 'medium' -MaxConcurrentTasks 2
$planner = Set-AgentDefinition -Name '素材_方案' -Aliases @('生成方案智能体') -Description '消费冻结分析、文案与市场快照，写 3 个同题 Variant 并委派生产。' -Instructions "全程使用中文。只执行 creative_plan。copy_snapshot 与 market snapshot 是唯一文案、事实和资源真值；不得重选或改写。写 V01-V03 结构化 brief，保留语义与主体，只改变表达；将缺失 production items 一次 fanout。需要输入时写 needs_input/action_required，不生成图片。$specialistHandoff" -SkillIDs @($planSkill.id) -RuntimeID $runtimeID -Model 'gpt-5.6-terra' -ThinkingLevel 'medium' -MaxConcurrentTasks 6
$producer = Set-AgentDefinition -Name '素材_出图' -Aliases @('图像编辑智能体') -Description '为标准 Variant 生成同内容族三尺寸底图并委派 Prime。' -Instructions "全程使用中文。只执行 creative_production；使用冻结 brief/copy_snapshot 和无品牌来源，写 generated assets、lineage 与 CLI 原始模型证据，只补当前 revision 的缺失尺寸。调用 multica image edit/edit-batch 时必须给 Bash 工具设置 timeout_ms 至少 900000（15 分钟），等待 CLI 返回完整 JSON 后再登记资产。不得处理 direct_edit、Prime、QC、采集或分析；齐备后只委派该 Variant 的 Prime。$specialistHandoff" -SkillIDs @($productionSkill.id) -RuntimeID $runtimeID -Model 'gpt-5.6-terra' -ThinkingLevel 'low' -MaxConcurrentTasks 10
$directEditor = Set-AgentDefinition -Name '素材_改图' -Aliases @('图片直接修改智能体') -Description '按用户原话修改固定来源底图，并按 delivery mode 决定是否委派 Prime。' -Instructions "全程使用中文。只执行 creative_direct_edit；source asset 不可覆盖，输出 revision 加一并记录 lineage 和 CLI 原始模型证据。preview 到 generated 结束；publish 只按 context expected_sizes 委派 Prime。不得触发采集、分析、方案或标准生产。$specialistHandoff" -SkillIDs @($directEditSkill.id) -RuntimeID $runtimeID -Model 'gpt-5.6-terra' -ThinkingLevel 'low' -MaxConcurrentTasks 10
$composer = Set-AgentDefinition -Name '素材_贴片' -Aliases @('Prime 包装智能体') -Description '按冻结市场合同批量合成 Prime 包并委派独立双路 QC。' -Instructions "全程使用中文。只执行 creative_prime；对 context 的 Variant/revision/expected_sizes 使用冻结 market snapshot 的固定坐标确定性合成，写 primed assets、manifest、固定背景净化和 QR 证据。不得写死组件、文字或坐标，不得动态移动 Prime 组件，不调用图像模型；整包齐备后一次 fanout 缺失 QC lanes。$specialistHandoff" -SkillIDs @($composeSkill.id) -RuntimeID $runtimeID -Model 'gpt-5.6-terra' -ThinkingLevel 'low' -MaxConcurrentTasks 6
$reviewer = Set-AgentDefinition -Name '素材_质检' -Aliases @('广告验收智能体') -Description '独立执行一个 technical 或 visual lane，并调用 QC barrier。' -Instructions "全程使用中文。只执行 context 指定 QC lane，读取同 Variant/revision/expected_sizes 的 Prime 包。technical 检查文件、尺寸、Prime、QR 与实际遮挡；visual 检查冻结文案、语义、一致性与画质。写独立 QC Report 后调用 qc-finalize；阻断必须 failed，不自动返工，不影响兄弟 Variant。$specialistHandoff" -SkillIDs @($qcSkill.id) -RuntimeID $runtimeID -Model 'gpt-5.6-terra' -ThinkingLevel 'medium' -MaxConcurrentTasks 6

$producerImageCredentialConfigured = Set-AgentImageCredential -AgentID $producer.id -ApiKey $ImageApiKey
$directEditorImageCredentialConfigured = Set-AgentImageCredential -AgentID $directEditor.id -ApiKey $ImageApiKey
$imageCredentialConfigured = $producerImageCredentialConfigured -and $directEditorImageCredentialConfigured

$squads = Get-Items (Invoke-MulticaApi -Method Get -Path '/api/squads') ''
$squad = $squads | Where-Object { $_.name -eq '素材流程小队' -or $_.name -eq 'AdaKami 素材小队' } | Select-Object -First 1
if (-not $squad) {
    $squad = Invoke-MulticaApi -Method Post -Path '/api/squads' -Body @{
        name = '素材流程小队'
        description = '从 AppGrowing 候选采集、逐图文案确认到每张素材 3 个创意、每创意 3 个尺寸的修图交付。'
        leader_id = $leaderSeed.id
    }
}
Invoke-MulticaApi -Method Put -Path "/api/squads/$($squad.id)" -Body @{
    name = '素材流程小队'
    instructions = 'Leader 读取 Creative Order 领域状态以及名册中每个智能体的职责和平台 Skill，动态选择成员。一个订单只关联一个用户可见 Issue；分析、方案、变体生成、Prime 和 QC 通过原生 task fanout 委派，不创建子 Issue。领域对象保存过程与证据，Issue 只保留用户目标、决定、真实阻塞和最终验收。'
} | Out-Null

$memberDefinitions = @(
    @{ agent = $collector; role = '素材采集' },
    @{ agent = $diagnostician; role = '流程诊断' },
    @{ agent = $analyst; role = '参考分析' },
    @{ agent = $planner; role = '生成方案' },
    @{ agent = $producer; role = '图像编辑' },
    @{ agent = $directEditor; role = '图片直接修改' },
    @{ agent = $composer; role = '完整贴图' },
    @{ agent = $reviewer; role = '质量验收' }
)
$members = Get-Items (Invoke-MulticaApi -Method Get -Path "/api/squads/$($squad.id)/members") ''
foreach ($definition in $memberDefinitions) {
    $existing = $members | Where-Object { $_.member_type -eq 'agent' -and $_.member_id -eq $definition.agent.id } | Select-Object -First 1
    if (-not $existing) {
        Invoke-MulticaApi -Method Post -Path "/api/squads/$($squad.id)/members" -Body @{
            member_type = 'agent'; member_id = $definition.agent.id; role = $definition.role
        } | Out-Null
    } elseif ($existing.role -ne $definition.role) {
        Invoke-MulticaApi -Method Patch -Path "/api/squads/$($squad.id)/members/role" -Body @{
            member_type = 'agent'; member_id = $definition.agent.id; role = $definition.role
        } | Out-Null
    }
}

$leaderSkillDirectory = Join-Path $skillTemplateRoot 'ad-creative-leadership'
$leaderSkill = Set-WorkspaceSkill -Name '素材_技能_流程' -Aliases @('素材_技能_统筹', '创意素材协作', '素材小队 Leader 编排') -Description '使用原生 task fanout 启动并恢复标准生产或直接改图，汇总结构化结果。' -Directory $leaderSkillDirectory -Config @{ kind = 'creative_role'; capability = 'creative_leadership'; version = 47 }
$leader = Set-AgentDefinition -Name '素材_流程' -Aliases @('素材_统筹', '素材小队 Leader') -Description '按冻结能力映射启动和恢复 Creative Order，并负责用户汇总。' -Instructions '全程使用中文。只执行判断、原生 fanout、异常恢复和用户汇总，不代替专业角色。标准订单只创建方案 task；direct_edit 只创建直接修改 task；正常下游由各阶段唯一 owner 续链。每次唤醒回读订单、task 与冻结 squad snapshot，按 target/source/item_key 只补真正缺失项，一次提交后立即结束。不得按名称猜 Agent，不轮询，不创建阶段子 Issue；Issue 只记录人工决定、真实阻塞和最终验收。' -SkillIDs @($leaderSkill.id) -RuntimeID $runtimeID -Model 'gpt-5.6-terra' -ThinkingLevel 'medium' -MaxConcurrentTasks 6

$resources = Get-Items (Invoke-MulticaApi -Method Get -Path '/api/creative/resources') 'resources'
$copyLibrary = $resources | Where-Object { $_.kind -eq 'copy_library' -and $_.name -eq 'AdaKami Indonesia 文案库' } | Select-Object -First 1
$copyLibraryIsNew = -not $copyLibrary
if (-not $copyLibrary) {
    $copyLibrary = Invoke-MulticaApi -Method Post -Path '/api/creative/resources' -Body @{
        kind = 'copy_library'
        name = 'AdaKami Indonesia 文案库'
        description = '当前市场已审核的投放文案和还款计划表。'
        config = @{ schema_version = 4; market = 'Indonesia'; locale = 'id-ID' }
    }
}

$numHeadlines = @(
    'Selamat! Anda Berkesempatan Ajukan Pinjaman!',
    'Cairinnya Sekarang, Bayarnya Nanti~',
    'Pinjaman untuk Semua Kebutuhan',
    'Butuh Pembiayaan Resmi?',
    'Pendanaan dengan Proses Simple',
    "Ubah rencana jadi nyata!`nDengan limit s.d. Rp80.000.000",
    "Punya kebutuhan mendesak?`nTenang, cairin AdaKami aja!"
)
$numRecipeIntentTags = @(
    @{ tags = @('机会','获批','成功','berkesempatan','approved') },
    @{ tags = @('现在','稍后','bayar nanti') },
    @{ tags = @('需求','生活场景','kebutuhan') },
    @{ tags = @('正规','许可','resmi','ojk') },
    @{ tags = @('简单','流程','simple','mudah') },
    @{ tags = @('计划','愿望','额度','rencana') },
    @{ tags = @('紧急','应急','需求','mendesak') }
)
$numCoreBenefits = @(
    @{ key = 'limit'; group = 'limit'; name = '最高额度'; text = 'Limit hingga Rp80.000.000'; tags = @('limit','额度','jumlah','rp') },
    @{ key = 'tenor'; group = 'tenor'; name = '灵活期限'; text = 'Pilihan Tenor 3-12 Bulan'; tags = @('tenor','期限','bulan') },
    @{ key = 'rate'; group = 'interest_rate'; name = '起始利率'; text = 'Bunga mulai dari 0,03%*'; tags = @('bunga','利率','interest') }
)
$numSupporting = @(
    @{ key = 'no-collateral'; name = '无需抵押'; text = 'Tanpa Jaminan'; tags = @('无需抵押','jaminan','collateral') },
    @{ key = 'ojk'; name = 'OJK 许可'; text = 'Berizin & Diawasi OJK'; tags = @('正规','许可','ojk','resmi') },
    @{ key = 'flat-installment'; name = '固定分期'; text = 'Cicilan FLAT sepanjang Tenor'; tags = @('固定分期','cicilan','tenor') },
    @{ key = 'no-upfront-fee'; name = '无前期费用'; text = 'Cairin Tanpa Biaya Awal'; tags = @('无前期费用','biaya awal') },
    @{ key = 'no-initial-cost'; name = '无初始费用'; text = 'Tanpa Biaya Awal'; tags = @('无初始费用','biaya awal') },
    @{ key = 'simple-process'; name = '流程简单'; text = 'Tanpa Proses Rumit'; tags = @('简单','流程','simple','mudah') }
)
$numCtas = @(
    @{ text = 'Ajukan Sekarang'; tags = @('ajukan','apply','申请') },
    @{ text = 'Ambil Sekarang'; tags = @('ambil','take','领取') },
    @{ text = 'Download Sekarang!'; tags = @('download','下载') },
    @{ text = 'Ajukan Segera!'; tags = @('ajukan','apply','申请') }
)
$repaymentHeadlines = @(
    'Pembiayaan Fleksibel, Bunga mulai dari 0,03%*',
    'Cairin Sekarang, Bayar Nanti~',
    'Bebas Cairin Sesuai Kebutuhan Anda',
    "Bunga Ringan & Terjangkau mulai`ndari 0,03%*",
    'Cicil Buat Kebutuhan Rumah Tanpa Worry~',
    'Nikmati Pinjaman Cicilan Ringan!'
)
$copyFragments = @()
for ($index = 0; $index -lt $numHeadlines.Count; $index += 1) {
    $copyFragments += @{ id = "fragment-num-headline-$($index + 1)"; key = "num-headline-$($index + 1)"; name = "常规标题 $($index + 1)"; content_group = 'standard_headline'; creative_types = @('num','repayment_plan'); role = 'headline'; usage = 'core'; text = $numHeadlines[$index]; tags = @('标题') + $numRecipeIntentTags[$index].tags; status = 'approved' }
}
foreach ($benefit in $numCoreBenefits) {
    $copyFragments += @{ id = "fragment-num-benefit-$($benefit.key)"; key = "num-benefit-$($benefit.key)"; name = $benefit.name; content_group = 'core_benefit'; creative_types = @('num','repayment_plan'); role = 'benefit'; semantic_group = $benefit.group; usage = 'core'; text = $benefit.text; tags = @('num','核心卖点') + $benefit.tags; status = 'approved' }
}
foreach ($supporting in $numSupporting) {
    $copyFragments += @{ id = "fragment-num-supporting-$($supporting.key)"; key = "num-supporting-$($supporting.key)"; name = $supporting.name; content_group = 'other_benefit'; creative_types = @('num','repayment_plan'); role = 'supporting'; usage = 'fallback'; text = $supporting.text; tags = @('其他卖点') + $supporting.tags; status = 'approved' }
}
for ($index = 0; $index -lt $numCtas.Count; $index += 1) {
    $copyFragments += @{ id = "fragment-cta-$($index + 1)"; key = "cta-$($index + 1)"; name = "CTA $($index + 1)"; content_group = 'call_to_action'; creative_types = @('num','repayment_plan'); role = 'cta'; usage = 'fallback'; text = $numCtas[$index].text; tags = @('cta','行动') + $numCtas[$index].tags; status = 'approved' }
}
for ($index = 0; $index -lt $repaymentHeadlines.Count; $index += 1) {
    $copyFragments += @{ id = "fragment-plan-headline-$($index + 1)"; key = "plan-headline-$($index + 1)"; name = "还款计划标题 $($index + 1)"; content_group = 'repayment_headline'; creative_types = @('num','repayment_plan'); role = 'headline'; usage = 'core'; text = $repaymentHeadlines[$index]; tags = @('还款','分期','cicilan','tenor'); status = 'approved' }
}

$copyRecipes = @()

$repaymentPlanRows = @(
    @{ principal = 4000000; tenor_months = 3; monthly_installment = 1369333 }, @{ principal = 4000000; tenor_months = 6; monthly_installment = 702667 }, @{ principal = 4000000; tenor_months = 12; monthly_installment = 369333 },
    @{ principal = 5000000; tenor_months = 3; monthly_installment = 1711667 }, @{ principal = 5000000; tenor_months = 6; monthly_installment = 878333 }, @{ principal = 5000000; tenor_months = 12; monthly_installment = 461667 },
    @{ principal = 6000000; tenor_months = 3; monthly_installment = 2054000 }, @{ principal = 6000000; tenor_months = 6; monthly_installment = 1054000 }, @{ principal = 6000000; tenor_months = 12; monthly_installment = 554000 },
    @{ principal = 7000000; tenor_months = 3; monthly_installment = 2396333 }, @{ principal = 7000000; tenor_months = 6; monthly_installment = 1229667 }, @{ principal = 7000000; tenor_months = 12; monthly_installment = 646333 },
    @{ principal = 8000000; tenor_months = 3; monthly_installment = 2738667 }, @{ principal = 8000000; tenor_months = 6; monthly_installment = 1405333 }, @{ principal = 8000000; tenor_months = 12; monthly_installment = 738667 },
    @{ principal = 9000000; tenor_months = 3; monthly_installment = 3081000 }, @{ principal = 9000000; tenor_months = 6; monthly_installment = 1581000 }, @{ principal = 9000000; tenor_months = 12; monthly_installment = 831000 },
    @{ principal = 10000000; tenor_months = 3; monthly_installment = 3423333 }, @{ principal = 10000000; tenor_months = 6; monthly_installment = 1756667 }, @{ principal = 10000000; tenor_months = 12; monthly_installment = 923333 }
)
$repaymentPlanEntries = foreach ($row in $repaymentPlanRows) {
    $totalRepayment = [int64]$row.monthly_installment * [int64]$row.tenor_months
    @{ id = "plan-$($row.principal)-$($row.tenor_months)"; key = "plan-$($row.principal)-$($row.tenor_months)"; principal = $row.principal; tenor_months = $row.tenor_months; monthly_installment = $row.monthly_installment; total_interest = $totalRepayment - [int64]$row.principal; total_repayment = $totalRepayment; source = '0805-AdaKami文案库与图片示例.xlsx / 还款计划案例'; status = 'approved' }
}
$copyLibraryConfig = @{
    schema_version = 4
    business_schema = 6
    market = 'Indonesia'
    locale = 'id-ID'
    source = @{
        name = '飞书图片案例 / 文案分组'
        url = 'https://my.feishu.cn/wiki/YKUqwgHNEibzw0k5RVUcZsW2nJh?sheet=elM4tt'
        sync_status = 'synced'
        note = '已核对图片案例、文案分组和已审核还款计划表。重复的 Tanpa Jaminan 已合并；中文仅用于后台理解，不进入投放文案。'
    }
    fragments = $copyFragments
    recipes = $copyRecipes
    repayment_plan = @{ labels = @{ principal = 'Jumlah Pinjaman'; tenor = 'Periode Cicilan'; monthly_installment = 'Cicilan per Bulan'; total_interest = 'Total Bunga'; total_repayment = 'Total Pembayaran' }; entries = @($repaymentPlanEntries) }
}
$copyLibraryHasBusinessStructure = $false
if ($copyLibrary -and $copyLibrary.config -and $copyLibrary.config.PSObject.Properties.Name -contains 'business_schema') {
    $copyLibraryHasBusinessStructure = [int]$copyLibrary.config.business_schema -ge 6
}
$seedCopyLibrary = $copyLibraryIsNew -or $ResetBusinessConfig -or ($copyLibrary.status -ne 'published') -or ([int]$copyLibrary.published_version -lt 1) -or -not $copyLibraryHasBusinessStructure
if ($seedCopyLibrary) {
    $copyLibrary = Invoke-MulticaApi -Method Put -Path "/api/creative/resources/$($copyLibrary.id)" -Body @{
        name = $copyLibrary.name
        description = '直接维护完整投放文案和已审核还款计划表；AI 只从现有文案与金额/期限组合中选择。'
        config = $copyLibraryConfig
    }
    $copyLibrary = Invoke-MulticaApi -Method Post -Path "/api/creative/resources/$($copyLibrary.id)/publish"
}

$resources = Get-Items (Invoke-MulticaApi -Method Get -Path '/api/creative/resources') 'resources'
$marketPack = $resources | Where-Object { $_.kind -eq 'market_pack' -and $_.name -eq 'AdaKami Indonesia 市场资源包' } | Select-Object -First 1
$marketPackIsNew = -not $marketPack
$marketPackHasPreAdaptationDefault = $false
if ($marketPack -and $marketPack.config -and $marketPack.config.PSObject.Properties.Name -contains 'pre_adaptation_default') {
    $marketPackHasPreAdaptationDefault = $marketPack.config.pre_adaptation_default -eq $true
}
$marketPackHasCalculationRules = $false
if ($marketPack -and $marketPack.config -and $marketPack.config.PSObject.Properties.Name -contains 'calculation_rules') {
    $marketPackHasCalculationRules = @($marketPack.config.calculation_rules).Count -gt 0
}
$seedMarketPack = $marketPackIsNew -or $ResetBusinessConfig -or ([int]$marketPack.published_version -lt 1) -or -not $marketPackHasPreAdaptationDefault -or -not $marketPackHasCalculationRules
$primeComponentDirectory = Join-Path $PrimeDirectory '.multica-prime-components-v2'
$primeExtractionManifestPath = Join-Path $primeComponentDirectory 'extract-manifest.json'
$primeLogoPath = Join-Path $primeComponentDirectory 'adakami-prime-logo.png'
$primeTermsPath = Join-Path $primeComponentDirectory 'adakami-prime-terms.png'
$primeQRPath = Join-Path $primeComponentDirectory 'adakami-prime-qr.png'
$primeStoreBadgesPath = Join-Path $primeComponentDirectory 'adakami-prime-store-badges.png'
$primeRegulatoryPath = Join-Path $primeComponentDirectory 'adakami-prime-regulatory.png'
$primeAFPIPath = Join-Path $primeComponentDirectory 'adakami-prime-afpi.png'
$primePindaiLegalPath = Join-Path $primeComponentDirectory 'adakami-prime-pindai-legal.png'
$primeExtractionManifest = @{
    source = Join-Path $PrimeDirectory '11-01.png'
    authored_size = @(1080, 1080)
    components = @(
        @{ role = 'prime_logo'; source_rect = @(30, 28, 314, 100); output = $primeLogoPath; background = 'transparent' },
        @{ role = 'prime_terms'; source_rect = @(777, 32, 982, 95); output = $primeTermsPath; background = 'transparent' },
        @{ role = 'prime_qr'; source_rect = @(983, 29, 1053, 99); output = $primeQRPath; background = 'opaque'; normalize_qr = $true; expected_payload = 'https://www.adakami.id/termsandconditions' },
        @{ role = 'prime_store_badges'; source_rect = @(30, 1013, 284, 1051); output = $primeStoreBadgesPath; background = 'transparent' },
        @{ role = 'prime_regulatory'; source_rect = @(378, 1010, 913, 1053); output = $primeRegulatoryPath; background = 'transparent' },
        @{ role = 'prime_afpi'; source_rect = @(918, 1008, 982, 1050); output = $primeAFPIPath; background = 'transparent' },
        @{ role = 'prime_pindai_legal'; source_rect = @(988, 984, 1054, 1057); output = $primePindaiLegalPath; background = 'transparent' }
    )
}
if ($seedMarketPack) {
    New-Item -ItemType Directory -Force -Path $primeComponentDirectory | Out-Null
    $primeExtractionManifest | ConvertTo-Json -Depth 20 | Set-Content -LiteralPath $primeExtractionManifestPath -Encoding utf8
    $primeExtractor = Join-Path $skillTemplateRoot 'ad-creative-prime-compose\references\extract_prime_components.py'
    $primeExtractionResult = & python $primeExtractor --manifest $primeExtractionManifestPath
    if ($LASTEXITCODE -ne 0) { throw "Prime component extraction failed: $primeExtractionResult" }
}
$marketConfig = @{
    contract_authority = 'published_market_pack_snapshot'
    rule_precedence = @('structured_config','versioned_attachments')
    brand = 'AdaKami'
    market = 'Indonesia'
    locale = 'id-ID'
    currency = 'IDR'
    copy_library_id = $copyLibrary.id
    pre_adaptation_default = $true
    calculation_rules = @(
        @{
            key = 'id_simple_daily_interest_v1'
            label = '印尼日息与还款计算'
            source = 'AK interest Calculator .xlsx'
            inputs = @{
                principal = @{ source = 'selected_repayment_plan.principal'; required = $true }
                tenor_months = @{ source = 'selected_repayment_plan.tenor_months'; required = $true }
                daily_rate = @{ value = 0.0003; display = '0,03%'; source = 'approved_market_calculator'; required = $true }
            }
            constants = @{ days_per_month = 30 }
            formulas = @{
                daily_interest_amount = 'principal × daily_rate'
                total_interest = 'principal × daily_rate × 30 × tenor_months'
                total_repayment = 'principal + total_interest'
                monthly_installment = 'total_repayment ÷ tenor_months'
            }
            display = @{ currency = 'IDR'; currency_prefix = 'Rp'; group_separator = '.'; monthly_rounding = 'nearest_integer' }
        }
    )
    benefit_taxonomy = @('额度','低利率','费用减免','免息','灵活期限','快速放款','低门槛','还款优惠')
    theme_presets = @('世界杯 / 足球赛事','斋月','开斋节','发薪日','开学季','年末','日常生活')
    qr_payload = 'https://www.adakami.id/termsandconditions'
    qr_canonical_payload = 'https://www.adakami.id/termsandconditions'
    qr_allowed_domains = @('www.adakami.id')
    qr_approval_status = 'approved'
    qr_approval_note = '独立静态二维码资源机器解码为该条款地址，并在三个尺寸中复用。'
    compliance_rules = '只能使用文案库已审核的金融事实；不得复制竞品品牌、金额、法律文字或二维码。启用的 Prime 组件必须完整呈现，最终二维码必须机器可解码。'
    naming_rule = '{month}_{brand}_{market}_{candidate}_{variant}_{size}_v{revision}.png'
    output_sizes = @('1080x1080','1200x628','800x1000')
    prime_composition = @{
        schema_version = 2
        qr_mode = 'static'
        components = @(
            @{ id = 'logo'; label = '品牌 Logo'; kind = 'image'; enabled = $true; source_role = 'prime_logo'; content = ''; backdrop_rule = 'none' },
            @{ id = 'terms'; label = '条款文字'; kind = 'image'; enabled = $true; source_role = 'prime_terms'; content = ''; backdrop_rule = 'none' },
            @{ id = 'qr'; label = '二维码'; kind = 'qr'; enabled = $true; source_role = 'prime_qr'; content = ''; backdrop_rule = 'light' },
            @{ id = 'store_badges'; label = '应用商店标识'; kind = 'image'; enabled = $true; source_role = 'prime_store_badges'; content = ''; backdrop_rule = 'none' },
            @{ id = 'regulatory'; label = '监管说明'; kind = 'image'; enabled = $true; source_role = 'prime_regulatory'; content = ''; backdrop_rule = 'none' },
            @{ id = 'afpi'; label = 'AFPI 标识'; kind = 'image'; enabled = $true; source_role = 'prime_afpi'; content = ''; backdrop_rule = 'none' },
            @{ id = 'pindai_legal'; label = 'Pindai Legal'; kind = 'image'; enabled = $true; source_role = 'prime_pindai_legal'; content = ''; backdrop_rule = 'none' }
        )
        layouts = @{
            '1080x1080' = @{
                components = @{
                    logo = @{ destination_rect = @(30, 28, 314, 100) }
                    terms = @{ destination_rect = @(777, 32, 982, 95) }
                    qr = @{ destination_rect = @(983, 29, 1053, 99) }
                    store_badges = @{ destination_rect = @(30, 1013, 284, 1051) }
                    regulatory = @{ destination_rect = @(378, 1010, 913, 1053) }
                    afpi = @{ destination_rect = @(918, 1008, 982, 1053) }
                    pindai_legal = @{ destination_rect = @(988, 984, 1054, 1057) }
                }
            }
            '1200x628' = @{
                components = @{
                    logo = @{ destination_rect = @(21, 22, 214, 70) }
                    terms = @{ destination_rect = @(946, 22, 1116, 80) }
                    qr = @{ destination_rect = @(1120, 16, 1192, 88) }
                    store_badges = @{ destination_rect = @(21, 582, 187, 607) }
                    regulatory = @{ destination_rect = @(793, 572, 1090, 608) }
                    afpi = @{ destination_rect = @(1093, 565, 1138, 609) }
                    pindai_legal = @{ destination_rect = @(1140, 552, 1182, 610) }
                }
            }
            '800x1000' = @{
                components = @{
                    logo = @{ destination_rect = @(25, 23, 234, 76) }
                    terms = @{ destination_rect = @(540, 24, 718, 82) }
                    qr = @{ destination_rect = @(722, 18, 790, 86) }
                    store_badges = @{ destination_rect = @(25, 946, 230, 976) }
                    regulatory = @{ destination_rect = @(281, 947, 630, 969) }
                    afpi = @{ destination_rect = @(635, 940, 710, 976) }
                    pindai_legal = @{ destination_rect = @(727, 922, 775, 976) }
                }
            }
        }
    }
}
if (-not $marketPack) {
    $marketPack = Invoke-MulticaApi -Method Post -Path '/api/creative/resources' -Body @{
        kind = 'market_pack'
        name = 'AdaKami Indonesia 市场资源包'
        description = '印尼市场可编辑配置与版本化附件；新市场可在平台复制后替换文案库、规则和资源槽位。'
        config = $marketConfig
    }
}

if ($seedMarketPack) {
    Add-MarketFile -MarketPack $marketPack -Role 'prime_logo' -Label 'Prime · AdaKami Logo' -Path $primeLogoPath -Metadata @{ source_type = 'standalone_component'; reused_across_sizes = $true }
    Add-MarketFile -MarketPack $marketPack -Role 'prime_terms' -Label 'Prime · 条款文字贴片' -Path $primeTermsPath -Metadata @{ source_type = 'standalone_component'; reused_across_sizes = $true }
    Add-MarketFile -MarketPack $marketPack -Role 'prime_qr' -Label 'Prime · 条款二维码' -Path $primeQRPath -Metadata @{ source_type = 'standalone_component'; reused_across_sizes = $true }
    Add-MarketFile -MarketPack $marketPack -Role 'prime_store_badges' -Label 'Prime · 应用商店标识' -Path $primeStoreBadgesPath -Metadata @{ source_type = 'standalone_component'; reused_across_sizes = $true }
    Add-MarketFile -MarketPack $marketPack -Role 'prime_regulatory' -Label 'Prime · 监管说明贴片' -Path $primeRegulatoryPath -Metadata @{ source_type = 'standalone_component'; reused_across_sizes = $true }
    Add-MarketFile -MarketPack $marketPack -Role 'prime_afpi' -Label 'Prime · AFPI 标识' -Path $primeAFPIPath -Metadata @{ source_type = 'standalone_component'; reused_across_sizes = $true }
    Add-MarketFile -MarketPack $marketPack -Role 'prime_pindai_legal' -Label 'Prime · Pindai Legal' -Path $primePindaiLegalPath -Metadata @{ source_type = 'standalone_component'; reused_across_sizes = $true }
    $marketFiles = Get-Items (Invoke-MulticaApi -Method Get -Path "/api/creative/resources/$($marketPack.id)/files") 'files'
    foreach ($obsoleteFile in @($marketFiles | Where-Object { @('compose_script','prime_square','prime_landscape','prime_portrait','brand_guideline') -contains $_.role })) {
        Invoke-MulticaApi -Method Delete -Path "/api/creative/resources/$($marketPack.id)/files/$($obsoleteFile.id)" | Out-Null
    }
    foreach ($appUIPath in $AppUIReferencePaths) {
        Add-MarketFile -MarketPack $marketPack -Role 'app_ui_reference' -Label "AdaKami App UI - $([IO.Path]::GetFileNameWithoutExtension($appUIPath))" -Path $appUIPath -Multiple -Metadata @{
            dimensions = '1080x2160'
            market = 'Indonesia'
            locale = 'id-ID'
            tags = @('app-ui', 'homepage', 'new-customer', 'loan-limit')
            description = '参考分析只识别通用 App UI 类型；Planner 根据订单冻结的市场快照从全部 App UI 参考图中选择最匹配的一张。图像编辑必须替换为 AdaKami 自有界面，不得保留或仿造竞品 UI。'
        }
    }
    $marketPack = Invoke-MulticaApi -Method Put -Path "/api/creative/resources/$($marketPack.id)" -Body @{
        name = 'AdaKami Indonesia 市场资源包'
        description = '印尼市场可编辑配置与版本化附件；新市场可在平台复制后替换文案库、规则和资源槽位。'
        config = $marketConfig
    }
    $marketPack = Invoke-MulticaApi -Method Post -Path "/api/creative/resources/$($marketPack.id)/publish"
}

$autopilots = Get-Items (Invoke-MulticaApi -Method Get -Path '/api/autopilots') 'autopilots'
$autopilot = $autopilots | Where-Object { $_.title -eq '印尼竞品素材周度采集' -or $_.title -eq '印尼与马来竞品素材周度采集' } | Select-Object -First 1
$autopilotDescription = @"
每周抓取 AppGrowing 印度尼西亚的现金贷/金融竞品素材，并创建一个可追踪的 Crawl Run。

市场资源包 ID：$($marketPack.id)
执行小队 ID：$($squad.id)
参考分析智能体 ID：$($analyst.id)

竞品：Easycash、Kredit Pintar、Adapundi、BantuSaku、Rupiah Cepat、UATAS、JULO
优先竞品：Easycash、Kredit Pintar、Adapundi
地区：印度尼西亚
语言：印度尼西亚语
设备：Android、iOS
媒体：未限定
时间范围：最近 30 天
选材：新素材 40%，投放少于 7 天且曝光估算大于 1K；跑量素材 60%，投放超过 30 天且曝光估算不低于 10M
素材类型：仅图片广告（asset_type=image）。视频、非图片和无法识别类型必须在选材前排除，不占用 25 条配额，也不进入 Crawl Run 或素材库。
最多输出：25 张图片

执行真实 AppGrowing 多页图片采集，结果进入创意工厂素材库并关联当前 Crawl Run；图片入库后立即用原生 task fanout 对本次新增图片并发执行逐图创意分析，归档未完成时允许分析读取真实源图片。分析完成后在创意工厂提醒用户选图、确认主题与文案。整个采集与预分析阶段不创建 Issue。授权失效时将 Crawl Run 标为 action_required，并提示用户前往“设置 - 集成”重新绑定，不能用测试数据替代。
"@
if (-not $autopilot) {
    $autopilot = Invoke-MulticaApi -Method Post -Path '/api/autopilots' -Body @{
        title = '印尼竞品素材周度采集'
        description = $autopilotDescription
        assignee_type = 'agent'
        assignee_id = $collector.id
        execution_mode = 'run_only'
    }
} else {
    $autopilot = Invoke-MulticaApi -Method Patch -Path "/api/autopilots/$($autopilot.id)" -Body @{
        title = '印尼竞品素材周度采集'
        description = $autopilotDescription
        assignee_type = 'agent'
        assignee_id = $collector.id
        execution_mode = 'run_only'
        status = 'active'
    }
}
$autopilotDetail = Invoke-MulticaApi -Method Get -Path "/api/autopilots/$($autopilot.id)"
$triggers = if ($autopilotDetail.PSObject.Properties.Name -contains 'triggers') { @($autopilotDetail.triggers) } else { @() }
$scheduleTrigger = $triggers | Where-Object kind -eq 'schedule' | Select-Object -First 1
$scheduleConfig = @{
    cron_expression = '0 9 * * 1'
    timezone = 'Asia/Shanghai'
    label = '每周一 09:00'
}
if (-not $scheduleTrigger) {
    Invoke-MulticaApi -Method Post -Path "/api/autopilots/$($autopilot.id)/triggers" -Body @{
        kind = 'schedule'
        cron_expression = $scheduleConfig.cron_expression
        timezone = $scheduleConfig.timezone
        label = $scheduleConfig.label
    } | Out-Null
} elseif (-not $scheduleTrigger.enabled -or
    $scheduleTrigger.cron_expression -ne $scheduleConfig.cron_expression -or
    $scheduleTrigger.timezone -ne $scheduleConfig.timezone -or
    $scheduleTrigger.label -ne $scheduleConfig.label) {
    Invoke-MulticaApi -Method Patch -Path "/api/autopilots/$($autopilot.id)/triggers/$($scheduleTrigger.id)" -Body @{
        enabled = $true
        cron_expression = $scheduleConfig.cron_expression
        timezone = $scheduleConfig.timezone
        label = $scheduleConfig.label
    } | Out-Null
}

[pscustomobject]@{
    CopyLibraryID = $copyLibrary.id
    CopyFragments = @($copyLibraryConfig.fragments).Count
    CopyRecipes = @($copyLibraryConfig.recipes).Count
    MarketPackID = $marketPack.id
    SquadID = $squad.id
    LeaderSkillID = $leaderSkill.id
    AutopilotID = $autopilot.id
    ImageCredentialConfigured = $imageCredentialConfigured
}
