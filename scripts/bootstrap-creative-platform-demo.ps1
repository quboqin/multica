param(
    [string]$ApiUrl = 'http://127.0.0.1:8080',
    [string]$AppUrl = 'http://localhost:3000',
    [string]$WorkspaceSlug = 'ad-creative-direct-pilot',
    [string]$Token = $env:MULTICA_BOOTSTRAP_TOKEN,
    [string]$CliPath = $env:MULTICA_CLI,
    [string]$CliProfile = $env:MULTICA_BOOTSTRAP_PROFILE,
    [string]$ImageApiKey = $env:MULTICA_IMAGE_API_KEY,
    [string]$Brand = 'AdaKami',
    [ValidateSet('Indonesia','Malaysia')]
    [string]$Market = 'Indonesia',
    [string]$Locale = '',
    [string]$Currency = '',
    [string[]]$Competitors = @(),
    [string[]]$PriorityCompetitors = @(),
    [string]$PrimeDirectory = 'C:\Users\zhangzhenyu\market-ad-samples\Prime Template - PNG file\Prime Template - PNG file',
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
    $userHome = [Environment]::GetFolderPath('UserProfile')
    if ([string]::IsNullOrWhiteSpace($userHome)) { $userHome = $env:USERPROFILE }
    if ([string]::IsNullOrWhiteSpace($userHome)) { return $null }

    $base = Join-Path $userHome '.multica'
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

    $defaultCli = Join-Path $env:USERPROFILE '.multica\bin\multica.exe'
    if (Test-Path -LiteralPath $defaultCli) {
        return $defaultCli
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

function Get-CreativeFactoryMarketProfile {
    param(
        [Parameter(Mandatory)][string]$Brand,
        [Parameter(Mandatory)][string]$Market,
        [string]$Locale,
        [string]$Currency,
        [string[]]$Competitors,
        [string[]]$PriorityCompetitors
    )
    $brandValue = if ([string]::IsNullOrWhiteSpace($Brand)) { 'AdaKami' } else { $Brand.Trim() }
    $marketValue = if ([string]::IsNullOrWhiteSpace($Market)) { 'Indonesia' } else { $Market.Trim() }
    $localeValue = if ([string]::IsNullOrWhiteSpace($Locale)) { 'en-US' } else { $Locale.Trim() }
    $currencyValue = if ([string]::IsNullOrWhiteSpace($Currency)) { 'USD' } else { $Currency.Trim().ToUpperInvariant() }
    $marketAbbreviation = $marketValue.Substring(0, [Math]::Min(2, $marketValue.Length)).ToUpperInvariant().PadRight(2, 'K')
    $profile = @{
        brand = $brandValue
        market = $marketValue
        market_label = $marketValue
        locale = $localeValue
        currency = $currencyValue
        currency_prefix = '$'
        language_label = '英语'
        market_abbreviation = $marketAbbreviation
        competitors = @($Competitors | Where-Object { -not [string]::IsNullOrWhiteSpace($_) } | ForEach-Object { $_.Trim() } | Select-Object -Unique)
        priority_competitors = @($PriorityCompetitors | Where-Object { -not [string]::IsNullOrWhiteSpace($_) } | ForEach-Object { $_.Trim() } | Select-Object -Unique)
    }
    if ($marketValue -eq 'Indonesia') {
        $profile.market_label = '印度尼西亚'
        $profile.locale = if ([string]::IsNullOrWhiteSpace($Locale)) { 'id-ID' } else { $Locale.Trim() }
        $profile.currency = if ([string]::IsNullOrWhiteSpace($Currency)) { 'IDR' } else { $Currency.Trim().ToUpperInvariant() }
        $profile.currency_prefix = 'Rp'
        $profile.language_label = '印度尼西亚语'
        $profile.market_abbreviation = 'ID'
        if (@($profile.competitors).Count -eq 0) {
            $profile.competitors = @('Easycash','Kredit Pintar','Adapundi','BantuSaku','Rupiah Cepat','UATAS','JULO')
        }
        if (@($profile.priority_competitors).Count -eq 0) {
            $profile.priority_competitors = @('Easycash','Kredit Pintar','Adapundi')
        }
    } elseif ($marketValue -eq 'Malaysia') {
        $profile.market_label = '马来西亚'
        $profile.locale = if ([string]::IsNullOrWhiteSpace($Locale)) { 'ms-MY' } else { $Locale.Trim() }
        $profile.currency = if ([string]::IsNullOrWhiteSpace($Currency)) { 'MYR' } else { $Currency.Trim().ToUpperInvariant() }
        $profile.currency_prefix = 'RM'
        $profile.language_label = '马来语'
        $profile.market_abbreviation = 'MY'
    }
    if (@($profile.priority_competitors).Count -eq 0 -and @($profile.competitors).Count -gt 0) {
        $profile.priority_competitors = @($profile.competitors | Select-Object -First 3)
    }
    return $profile
}

$marketProfile = Get-CreativeFactoryMarketProfile -Brand $Brand -Market $Market -Locale $Locale -Currency $Currency -Competitors $Competitors -PriorityCompetitors $PriorityCompetitors
$useIndonesiaSeedData = $marketProfile.brand -eq 'AdaKami' -and $marketProfile.market -eq 'Indonesia'
$copyLibraryName = "$($marketProfile.brand) $($marketProfile.market) 文案库"
$marketPackName = "$($marketProfile.brand) $($marketProfile.market) 市场资源包"
$marketPackDescription = "$($marketProfile.brand) $($marketProfile.market_label) 市场规则、品牌组件和交付配置。"
$autopilotTitle = if ($useIndonesiaSeedData) { '印尼竞品素材周度采集' } else { "$($marketProfile.market_label)竞品素材周度采集" }
$competitorText = if (@($marketProfile.competitors).Count -gt 0) { @($marketProfile.competitors) -join '、' } else { '待配置' }
$priorityCompetitorText = if (@($marketProfile.priority_competitors).Count -gt 0) { @($marketProfile.priority_competitors) -join '、' } else { '待配置' }

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
    '广告参考布局分析',
    '素材_技能_组件识别',
    '市场包组件识别'
)
$existingSkills = Get-Items (Invoke-MulticaApi -Method Get -Path '/api/skills') ''
foreach ($legacy in $existingSkills | Where-Object {
    $legacySkillNames -contains $_.name -or $_.name -like '广告创意生产（*'
}) {
    Invoke-MulticaApi -Method Delete -Path "/api/skills/$($legacy.id)" | Out-Null
}

$collectorSkill = Set-WorkspaceSkill -Name '素材_技能_采集' -Aliases @('AppGrowing 素材采集') -Description '创建 Crawl Run，只采集真实图片广告，并用原生 task fanout 自动预分析新增图片。' -Directory (Join-Path $skillTemplateRoot 'appgrowing-material-collector') -Config @{ kind = 'creative_role'; capability = 'material_collection'; version = 17 }
$diagnosisSkill = Set-WorkspaceSkill -Name '素材_技能_诊断' -Aliases @('创意流程诊断', '出图诊断', 'AppGrowing 采集诊断') -Description '读取创意采集、候选、尺寸调用、版本、Prime、QC 和 daemon/runtime 证据，只通过平台领域入口恢复或给出明确动作。' -Directory (Join-Path $skillTemplateRoot 'creative-flow-diagnostician') -Config @{ kind = 'creative_role'; capability = 'crawl_diagnosis'; version = 7 }
$analysisSkill = Set-WorkspaceSkill -Name '素材_技能_分析' -Aliases @('广告参考分析') -Description '市场中立地读取真实图片，识别可变视觉区域、原图文字及坐标、主题、利益点、语义锚点、App UI 类型、屏幕边界和布局约束；App UI 只做通用检测，不选择品牌附件；只有明显的还款结构才锁定为 numeric，单独金额或核心利益点不得因为带数字就被卡死。' -Directory (Join-Path $skillTemplateRoot 'ad-creative-analysis') -Config @{ kind = 'creative_role'; capability = 'reference_analysis'; version = 19 }
$preAdaptationSkill = Set-WorkspaceSkill -Name '素材_技能_文案适配' -Aliases @('广告预适配') -Description '按冻结资源完成可生产文案与数值适配；只有明显的还款结构才生成 repayment 选择和 numeric layout，单独金额、核心利益点或促销额度默认保留为可编辑文案，保留后续可手动改写空间；数值布局说明必须列出每个冻结展示值。' -Directory (Join-Path $skillTemplateRoot 'ad-creative-pre-adaptation') -Config @{ kind = 'creative_role'; capability = 'pre_adaptation'; version = 26 }
$planSkill = Set-WorkspaceSkill -Name '素材_技能_方案' -Aliases @('广告生成方案') -Description '消费冻结分析、文案与市场快照，规划 4-5 个统一方形首轮候选及三尺寸 LayoutPlan；只有用户明确选择时才替换 App UI，非空核心利益点必须作为可见文字，独立质检原子晋级 3 个。' -Directory (Join-Path $skillTemplateRoot 'ad-creative-plan') -Config @{ kind = 'creative_role'; capability = 'generation_plan'; version = 40 }
$productionSkill = Set-WorkspaceSkill -Name '素材_技能_出图' -Aliases @('广告图像编辑') -Description '按唯一模型提示词合同生成统一方形候选主视觉或 selected 缺失尺寸；三尺寸共享 DesignDNA、文案与血缘但从冻结附件和各自 LayoutPlan 独立生成，附件失败结构化分类，按冻结 Prime 模式仅在明确选择时替换 App UI，并把批准利益点渲染为可见文字。' -Directory (Join-Path $skillTemplateRoot 'ad-creative-production') -Config @{ kind = 'creative_role'; capability = 'image_edit'; version = 111 }
$primeComposeSkill = Set-WorkspaceSkill -Name '素材_技能_贴片' -Aliases @('广告品牌组件合成') -Description '调用后端唯一的 Prime 交接入口：默认确定性合成，或登记冻结的无二维码模型融入结果；校验 JSON，并由后端登记过程图、primed 资产和标准 QC/交付交接；不创建 Prime Agent 或 Prime task。' -Directory (Join-Path $skillTemplateRoot 'ad-creative-prime-compose') -Config @{ kind = 'creative_role'; capability = 'prime_compose'; version = 6 }
$directEditSkill = Set-WorkspaceSkill -Name '素材_技能_改图' -Aliases @('广告图片直接修改') -Description '将用户反馈编译为带输入角色、锁定/可编辑集合和 target masks 的多目标调整；按冻结 Prime 模式保留官方组件关系，必要时仅重排或缩放点名内容组，阻塞等待模型回执后全部目标同时通过交接并终检。' -Directory (Join-Path $skillTemplateRoot 'ad-creative-direct-edit') -Config @{ kind = 'creative_role'; capability = 'direct_image_edit'; version = 30 }
$qcSkill = Set-WorkspaceSkill -Name '素材_技能_质检' -Aliases @('广告成图验收') -Description '独立比较 4-5 个候选主尺寸并原子晋级 3 个，或对标准/精准改图的实际交付尺寸执行 Prime 与 DesignDNA 联合视觉终检；附件失败结构化分类，qc-finalize 瞬态失败由服务端持久化恢复。' -Directory (Join-Path $skillTemplateRoot 'ad-creative-qc') -Config @{ kind = 'creative_role'; capability = 'quality_control'; version = 43 }

$agents = Get-Items (Invoke-MulticaApi -Method Get -Path '/api/agents') ''
foreach ($legacyPrimeAgent in @($agents | Where-Object {
    ($_.name -eq '素材_贴片' -or $_.name -eq '广告贴片智能体') -and
    -not $_.archived_at
})) {
    Invoke-MulticaApi -Method Post -Path "/api/agents/$($legacyPrimeAgent.id)/archive" | Out-Null
}
$leaderSeed = $agents | Where-Object { $_.name -eq '素材_流程' -or $_.name -eq '素材_统筹' -or $_.name -eq '素材小队 Leader' } | Select-Object -First 1
if (-not $leaderSeed) { throw '素材_流程 does not exist' }
$runtimeID = $leaderSeed.runtime_id

$specialistHandoff = '只处理 task context 指定的对象、revision 和 scope；按绑定 Skill 写结构化领域结果和机器证据。图像任务按绑定 Skill 的统一比例阈值处理：阈值内归一，10%-25% canvas repair，只有超过 25% 才重生当前尺寸；不能把像素绝对尺寸差当成模型失败。不得创建或修改 Issue，不得用评论代替领域数据。输入、凭证、工具或写回失败时保留已成功对象，写真实 error_code/error_message 并让当前 task 失败；兄弟对象继续。'
$directEditAgentInstructions = @'
creative_direct_edit 分支严格执行绑定的素材_技能_改图；按 Variant 冻结的 prime_composition 编辑 task context 指向的 source asset，保留批准文案、附件血缘、归一化、完整模型回执、过程图登记和多目标同时验收，再调用绑定的贴片 Skill 并进入最终视觉 QC。绑定 Skill 是直接改图合同的唯一执行真值，不从 Agent instructions 或历史 prompt 补另一套协议。
'@
$producerInstructions = @'
全程使用中文。根据 task context.workflow 选择唯一分支，并在执行前完整读取对应绑定 Skill；Skill 及其 references 是提示词、证据、归一化、Prime 和恢复规则的唯一执行真值。

creative_production 只执行绑定的素材_技能_出图：候选阶段全部生成 1080x1080 方图作公平比较；selected 阶段按订单当前 candidate_state、primary_size、production_stage 和 expected_sizes 补齐原生横竖尺寸，复用已有主图和成功回执，按 CreativeIntent、DesignDNA、LayoutPlan 独立生成尺寸，不把候选方图 raster 当硬依赖。候选源图、Prime 模板和已选 App UI 只使用订单冻结的 attachment_snapshot；附件失败按 auth_expired、attachment_not_found、storage_timeout 或 cli_contract_mismatch 记录，不猜 URL 或复用旧文件。严格按 Variant 冻结的 prime_composition 选择无品牌底图或 QR-free 模板融入，不按市场名猜模式；只在当前阶段尺寸与过程证据齐全后调用绑定的贴片 Skill。
'@
$producerInstructions += "`n`n" + $directEditAgentInstructions + @'

两个分支都只处理 task context 指定对象和平台锁定 revision，不创建或修改 Issue，不跨 workflow。模型 prompt 只包含改变像素的视觉指令；task、revision、文件、哈希、上传、登记、重试和状态说明留在模型调用外。协议或登记失败复用已有回图与证据；只有没有有效回图或真实视觉失败才按 Skill 的有界规则再次调用模型。
'@
$producerInstructions += "`n`n" + $specialistHandoff
$analyst = Set-AgentDefinition -Name '素材_分析' -Aliases @('广告参考分析智能体') -Description '按 task workflow 读取真实像素、识别可变视觉区域、写市场中立分析，并在后台完成可确认的文案与数值预适配。' -Instructions "全程使用中文。creative_reference_analysis 只写指定 candidate/version 的市场中立 Source Analysis；每个可变原图文字区块必须归入唯一 copy 或 numeric 视觉区域，不能把同一画面组件拆入两条处理路径；App UI 只输出通用页面类型、屏幕边界和 replacement_needed，不读取、选择或引用 app_ui_reference 附件。creative_pre_adaptation 只消费指定的冻结市场包和文案库，逐区域优先绑定已审核内容；没有兼容已审核片段的普通 headline、subheadline、benefit、supporting 或 cta 区块，必须按市场语言、区块职责、原图语义和可读长度生成一个新的 recommended 文案，source_keys 必须为空数组，recommendation_basis 必须说明依据，页面会标记待用户确认；不得引用不存在的 fragment key。本金、期限、月供、总利息、总还款、利率、法律文字和品牌事实没有审核来源或冻结计算时不得凭空生成，逐项写 missing replacement。多行数值表不要求原图行数与我方方案数量相等：按表格语义和期限从冻结 approved repayment plan 取我方兼容方案，实际渲染行数取原图可渲染行数与我方可用方案数的较小值；每个实际渲染行写 numeric_layouts，且每个 layout 的 render_instruction 必须逐字列出其 scenario_ids 对应 selection.values 中每个 target_columns 的完整冻结展示值，不能只写按行展示；源图多出的数值块逐项写空 missing 作为默认移除项。没有我方某一期限方案时不得借用其他期限金额；整体重构为我方支持的期限列，或让该列源块留空移除。只有明显的还款结构才锁定，单独金额、核心利益点或促销额度不得因为带数字就卡死成还款计划。不能把整表硬塞进一个 layout，也不能伪装成已绑定或改写原图事实。输入和产物不得混用，不生成图片，不修改市场包或文案库。$specialistHandoff" -SkillIDs @($analysisSkill.id, $preAdaptationSkill.id) -RuntimeID $runtimeID -Model 'gpt-5.6-terra' -ThinkingLevel 'medium' -MaxConcurrentTasks 6
$collector = Set-AgentDefinition -Name '素材_采集' -Aliases @('AppGrowing 素材采集智能体') -Description '按 task 配置创建 Crawl Run，只导入真实图片广告并委派新增图片分析。' -Instructions "全程使用中文。只执行 task context 和 AutoPilot 明确的 AppGrowing 查询；只导入 asset_type=image，视频、非图片和未知类型不占用名额。analysis_agent_id 由平台按当前 workspace installation 注入，不从名称、评论或历史说明猜测。筛选、分页、预算、目标数量和 fallback 来自 AutoPilot/task 的业务描述。结果、证据和失败写 Crawl Run；导入后用原生 fanout 委派本次新增图片，不创建 Issue，不使用测试数据。" -SkillIDs @($collectorSkill.id) -RuntimeID $runtimeID -MaxConcurrentTasks 1
$diagnostician = Set-AgentDefinition -Name '素材_诊断' -Aliases @('创意流程诊断智能体', '出图诊断智能体', 'AppGrowing 采集诊断智能体') -Description '诊断创意采集、候选、尺寸调用、版本、Prime、QC 和 daemon/runtime 异常，并通过平台入口执行受控恢复。' -Instructions '全程使用中文。处理 creative_crawl_diagnosis、订单短 ID、Variant 标签、页面卡片文案、报错文本和用户明确指向的创意流程诊断。先定位当前订单、order item、候选状态、active/staging revision、尺寸 operation、task、daemon/runtime 与 Skill 快照证据，再给结论；需要恢复时只通过 multica CLI 或平台 API 的重试、取消、fanout 或现有领域修复入口，revision 只能由平台事务创建，不得用 variant-put 自行推进。具备 crawl_diagnosis 能力的运行诊断任务可对同工作区、已验证的创意对象使用受审计的高额度恢复预算；不得跨工作区、直接写 DB、修改凭证、业务筛选、市场包或生产代码。只有用户明确要求维护文案库时，才可先回读文案库并使用 multica creative copy-library 的 add-fragment、update-fragment、upsert-repayment-plan 保存草稿；只有用户明确要求发布时才加 --publish。不得把诊断图当成交付资产；修改前说明对象和原因，修改后回读验证。' -SkillIDs @($diagnosisSkill.id) -RuntimeID $runtimeID -Model 'gpt-5.6-luna' -ThinkingLevel 'medium' -MaxConcurrentTasks 2
$planner = Set-AgentDefinition -Name '素材_方案' -Aliases @('生成方案智能体') -Description '消费冻结输入，为标准订单建立 4-5 个主视觉候选。' -Instructions '全程使用中文。只执行 creative_plan，完整执行绑定的素材_技能_方案；该 Skill 及 Creative Intent reference 是候选数量、主尺寸、CreativeIntent、DesignDNA、LayoutPlan、App UI、金融文案和生产 fanout 的唯一真值。只接受平台冻结的 candidate_v1，写 4-5 个 candidate Variant，所有候选首轮固定 1080x1080 并委派方形主视觉；不预选 3 个、不生成图片，selected 后再按各尺寸原生重排且不把候选方图当母版。只有页面冻结的明确选择才使用 App UI，非空核心利益点必须列为可见文字。流程版本缺失或不符时写真实错误，不推断或切换流程。只处理 task context 指定对象，失败保留已写候选和真实错误。' -SkillIDs @($planSkill.id) -RuntimeID $runtimeID -Model 'gpt-5.6-terra' -ThinkingLevel 'medium' -MaxConcurrentTasks 6
$producer = Set-AgentDefinition -Name '素材_出图' -Aliases @('图像编辑智能体') -Description '按唯一提示词合同执行候选主视觉、selected 扩尺寸或用户标注精准改图；只用订单冻结附件，结构化记录下载失败，再由贴片 Skill 交接终检。' -Instructions $producerInstructions -SkillIDs @($productionSkill.id, $directEditSkill.id, $primeComposeSkill.id) -RuntimeID $runtimeID -Model 'gpt-5.6-terra' -ThinkingLevel 'low' -MaxConcurrentTasks 10
$reviewer = Set-AgentDefinition -Name '素材_质检' -Aliases @('广告验收智能体') -Description '独立晋级候选主视觉，或联合验收当前实际交付尺寸的 Prime 成图。' -Instructions '全程使用中文。根据 task context.workflow 只执行 creative_candidate_selection 或 creative_qc_visual，并完整执行绑定的素材_技能_质检；Skill 是评分、candidate-select、结构化 Prime 极性、实际交付尺寸联合验收、多目标检查和 qc-finalize 的唯一真值。候选分支恰选 3 个并使用原子 candidate-select，回读平台自动排队结果，不自行重复 fanout；终检分支不得硬编码背景极性、逐图自报通过或使用旧 revision 资产，单尺寸精准改图也必须完成最终视觉质检。附件下载失败按 Skill 写精确错误码；qc-finalize 瞬态失败保留报告并结束当前任务，由服务端复用 Prime 资产恢复。' -SkillIDs @($qcSkill.id) -RuntimeID $runtimeID -Model 'gpt-5.6-terra' -ThinkingLevel 'medium' -MaxConcurrentTasks 6

$imageCredentialConfigured = Set-AgentImageCredential -AgentID $producer.id -ApiKey $ImageApiKey

$squads = Get-Items (Invoke-MulticaApi -Method Get -Path '/api/squads') ''
$squad = $squads | Where-Object { $_.name -eq '素材流程小队' -or $_.name -eq 'AdaKami 素材小队' } | Select-Object -First 1
if (-not $squad) {
    $squad = Invoke-MulticaApi -Method Post -Path '/api/squads' -Body @{
        name = '素材流程小队'
        description = '从 AppGrowing 候选采集、逐图文案确认到 4-5 个主视觉候选统一比较，最终交付 3 个创意、每个创意 3 个尺寸。'
        leader_id = $leaderSeed.id
    }
}
Invoke-MulticaApi -Method Put -Path "/api/squads/$($squad.id)" -Body @{
    name = '素材流程小队'
    instructions = 'Leader 读取 Creative Order 领域状态以及名册中每个智能体的职责和平台 Skill，动态选择成员。一个订单只关联一个用户可见 Issue；分析、方案、变体生成和 QC 通过原生 task fanout 委派，品牌组件由后端确定性合成，不创建子 Issue。领域对象保存过程与证据，Issue 只保留用户目标、决定、真实阻塞和最终验收。'
} | Out-Null

$memberDefinitions = @(
    @{ agent = $collector; role = '素材采集' },
    @{ agent = $diagnostician; role = '流程诊断' },
	@{ agent = $analyst; role = '参考分析' },
	@{ agent = $planner; role = '生成方案' },
	@{ agent = $producer; role = '图像编辑' },
	@{ agent = $reviewer; role = '质量验收' }
)
$members = Get-Items (Invoke-MulticaApi -Method Get -Path "/api/squads/$($squad.id)/members") ''
$legacyDirectAgents = @($agents | Where-Object {
	($_.name -eq '素材_改图' -or $_.name -eq '图片直接修改智能体') -and
	-not $_.archived_at -and $_.id -ne $producer.id
})
foreach ($legacyDirectAgent in $legacyDirectAgents) {
	$legacyMember = $members | Where-Object {
		$_.member_type -eq 'agent' -and $_.member_id -eq $legacyDirectAgent.id
	} | Select-Object -First 1
	if ($legacyMember) {
		Invoke-MulticaApi -Method Delete -Path "/api/squads/$($squad.id)/members" -Body @{
			member_type = 'agent'; member_id = $legacyDirectAgent.id
		} | Out-Null
	}
	Invoke-MulticaApi -Method Post -Path "/api/agents/$($legacyDirectAgent.id)/archive" | Out-Null
}
foreach ($legacyMember in @($members | Where-Object { $_.member_type -eq 'agent' -and $_.role -eq '完整贴图' })) {
    Invoke-MulticaApi -Method Delete -Path "/api/squads/$($squad.id)/members" -Body @{
        member_type = 'agent'; member_id = $legacyMember.member_id
    } | Out-Null
}
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
$leaderSkill = Set-WorkspaceSkill -Name '素材_技能_流程' -Aliases @('素材_技能_统筹', '创意素材协作', '素材小队 Leader 编排') -Description '使用原生 task fanout 启动并恢复候选生产、晋级扩尺寸、直接改图和最终验收，汇总结构化结果。' -Directory $leaderSkillDirectory -Config @{ kind = 'creative_role'; capability = 'creative_leadership'; version = 51 }
$leader = Set-AgentDefinition -Name '素材_流程' -Aliases @('素材_统筹', '素材小队 Leader') -Description '按冻结能力映射启动和恢复 Creative Order，并负责用户汇总。' -Instructions '全程使用中文。只执行判断、原生 fanout、异常恢复和用户汇总，不代替专业角色。标准订单只创建方案 task；Planner 建候选并委派主尺寸，候选晋级与 selected 扩尺寸由阶段 owner 和平台续链；初始 direct_edit 由平台原子创建 revision 和 task，Leader 只恢复领域状态确认缺失的当前 revision task。每次唤醒回读订单、task 与冻结 squad snapshot，按 target/source/item_key 只补真正缺失项。严格执行绑定流程 Skill，不按名称猜 Agent，不轮询，不创建阶段子 Issue。' -SkillIDs @($leaderSkill.id) -RuntimeID $runtimeID -Model 'gpt-5.6-terra' -ThinkingLevel 'medium' -MaxConcurrentTasks 6

$resources = Get-Items (Invoke-MulticaApi -Method Get -Path '/api/creative/resources') 'resources'
$copyLibrary = $resources | Where-Object { $_.kind -eq 'copy_library' -and $_.name -eq $copyLibraryName } | Select-Object -First 1
$copyLibraryIsNew = -not $copyLibrary
if (-not $copyLibrary) {
    $copyLibrary = Invoke-MulticaApi -Method Post -Path '/api/creative/resources' -Body @{
        kind = 'copy_library'
        name = $copyLibraryName
        description = '当前市场已审核的投放文案和还款计划表。'
        config = @{ schema_version = 4; market = $marketProfile.market; locale = $marketProfile.locale }
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
if ($useIndonesiaSeedData) {
    $copyLibrarySource = @{
        name = '飞书图片案例 / 文案分组'
        url = 'https://my.feishu.cn/wiki/YKUqwgHNEibzw0k5RVUcZsW2nJh?sheet=elM4tt'
        sync_status = 'synced'
        note = '已核对图片案例、文案分组和已审核还款计划表。重复的 Tanpa Jaminan 已合并；中文仅用于后台理解，不进入投放文案。'
    }
    $repaymentPlanLabels = @{ principal = 'Jumlah Pinjaman'; tenor = 'Periode Cicilan'; monthly_installment = 'Cicilan per Bulan'; total_interest = 'Total Bunga'; total_repayment = 'Total Pembayaran' }
} else {
    $copyFragments = @()
    $copyRecipes = @()
    $repaymentPlanEntries = @()
    $copyLibrarySource = @{
        name = ''
        url = ''
        sync_status = 'pending'
        note = '待补充当前市场已审核文案与还款计划。'
    }
    $repaymentPlanLabels = @{ principal = 'Principal'; tenor = 'Tenor'; monthly_installment = 'Monthly Installment'; total_interest = 'Total Interest'; total_repayment = 'Total Repayment' }
}
$copyLibraryConfig = @{
    schema_version = 4
    business_schema = 6
    market = $marketProfile.market
    locale = $marketProfile.locale
    source = $copyLibrarySource
    fragments = $copyFragments
    recipes = $copyRecipes
    repayment_plan = @{ labels = $repaymentPlanLabels; entries = @($repaymentPlanEntries) }
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
    if ($useIndonesiaSeedData) {
        $copyLibrary = Invoke-MulticaApi -Method Post -Path "/api/creative/resources/$($copyLibrary.id)/publish"
    }
}

$resources = Get-Items (Invoke-MulticaApi -Method Get -Path '/api/creative/resources') 'resources'
$marketPack = $resources | Where-Object { $_.kind -eq 'market_pack' -and $_.name -eq $marketPackName } | Select-Object -First 1
$marketPackIsNew = -not $marketPack
$marketPackHasPreAdaptationDefault = $false
if ($marketPack -and $marketPack.config -and $marketPack.config.PSObject.Properties.Name -contains 'pre_adaptation_default') {
    $marketPackHasPreAdaptationDefault = $marketPack.config.pre_adaptation_default -eq $true
}
$marketPackHasCalculationRules = $false
if ($marketPack -and $marketPack.config -and $marketPack.config.PSObject.Properties.Name -contains 'calculation_rules') {
    $marketPackHasCalculationRules = @($marketPack.config.calculation_rules).Count -gt 0
}
$legacyPrimeQRConfigFields = @('qr_payload', 'qr_canonical_payload', 'qr_allowed_domains', 'qr_approval_status', 'qr_approval_note')
$marketPackHasLegacyQRConfig = $false
if ($marketPack -and $marketPack.config) {
    $marketPackHasLegacyQRConfig = @($legacyPrimeQRConfigFields | Where-Object { $marketPack.config.PSObject.Properties.Name -contains $_ }).Count -gt 0
}
$primeTemplates = @(
    @{ role = 'prime_light_square'; label = '明亮底图方案 · 方形 · 11-01.png'; path = (Join-Path $PrimeDirectory '11-01.png'); size = '1080x1080'; family = 'light_background' },
    @{ role = 'prime_light_landscape'; label = '明亮底图方案 · 横版 · 191-01.png'; path = (Join-Path $PrimeDirectory '191-01.png'); size = '1200x628'; family = 'light_background' },
    @{ role = 'prime_light_portrait'; label = '明亮底图方案 · 竖版 · 45-01.png'; path = (Join-Path $PrimeDirectory '45-01.png'); size = '800x1000'; family = 'light_background' },
    @{ role = 'prime_dark_square'; label = '深色底图方案 · 方形 · 11-02.png'; path = (Join-Path $PrimeDirectory '11-02.png'); size = '1080x1080'; family = 'dark_background' },
    @{ role = 'prime_dark_landscape'; label = '深色底图方案 · 横版 · 191-03.png'; path = (Join-Path $PrimeDirectory '191-03.png'); size = '1200x628'; family = 'dark_background' },
    @{ role = 'prime_dark_portrait'; label = '深色底图方案 · 竖版 · 45-03.png'; path = (Join-Path $PrimeDirectory '45-03.png'); size = '800x1000'; family = 'dark_background' }
)
$requiredPrimeTemplateRoles = @($primeTemplates | ForEach-Object { $_.role })
$marketPackHasPrimeTemplateSet = $false
if ($marketPack -and $marketPack.config -and $marketPack.config.PSObject.Properties.Name -contains 'prime_template_set') {
    $templateSet = $marketPack.config.prime_template_set
    if ($templateSet -and $templateSet.PSObject.Properties.Name -contains 'families') {
        $configuredRoles = @($templateSet.families | ForEach-Object { $_.templates.PSObject.Properties | ForEach-Object { $_.Value.source_role } })
        $marketPackHasPrimeTemplateSet = $templateSet.schema_version -eq 2 -and $templateSet.selection_mode -eq 'automatic_family_contrast' -and @($templateSet.families).Count -eq 2 -and $configuredRoles.Count -eq $requiredPrimeTemplateRoles.Count -and @($configuredRoles | Where-Object { $_ -notin $requiredPrimeTemplateRoles }).Count -eq 0
    }
}
$marketPackHasExactPrimeFiles = $false
if ($marketPack) {
    $marketFiles = Get-Items (Invoke-MulticaApi -Method Get -Path "/api/creative/resources/$($marketPack.id)/files") 'files'
    $primeFiles = @($marketFiles | Where-Object { $_.role -like 'prime_*' })
    $primeRoles = @($primeFiles | ForEach-Object { $_.role } | Select-Object -Unique)
    $marketPackHasExactPrimeFiles = $primeFiles.Count -eq $requiredPrimeTemplateRoles.Count -and $primeRoles.Count -eq $requiredPrimeTemplateRoles.Count -and @($primeRoles | Where-Object { $_ -notin $requiredPrimeTemplateRoles }).Count -eq 0
}
$marketPackIsCurrentPublished = $marketPack -and ([int]$marketPack.version -eq [int]$marketPack.published_version) -and $marketPack.config.prime_template_set_validation.status -eq 'passed'
$seedMarketPack = $marketPackIsNew -or $ResetBusinessConfig -or $marketPackHasLegacyQRConfig -or -not $marketPackIsCurrentPublished -or -not $marketPackHasPreAdaptationDefault -or ($useIndonesiaSeedData -and -not $marketPackHasCalculationRules) -or -not $marketPackHasPrimeTemplateSet -or -not $marketPackHasExactPrimeFiles
$calculationRules = @()
$complianceRules = '只能使用当前市场已审核的金融事实、文案和品牌组件；不得复制竞品品牌、金额、法律文字或专属页面元素。'
if ($useIndonesiaSeedData) {
    $calculationRules = @(
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
            display = @{ currency = $marketProfile.currency; currency_prefix = $marketProfile.currency_prefix; group_separator = '.'; monthly_rounding = 'nearest_integer' }
        }
    )
    $complianceRules = '只能使用文案库已审核的金融事实；不得复制竞品品牌、金额、法律文字或二维码。完整 Prime 模板按上传文件原样叠加，平台不生成或修改品牌与合规贴片。'
}
$marketConfig = @{
    contract_authority = 'published_market_pack_snapshot'
    rule_precedence = @('structured_config','versioned_attachments')
    brand = $marketProfile.brand
    market = $marketProfile.market
    locale = $marketProfile.locale
    currency = $marketProfile.currency
    copy_library_id = $copyLibrary.id
    pre_adaptation_default = $true
    calculation_rules = $calculationRules
    compliance_rules = $complianceRules
    naming_rule = '{month}_P_AK_MY_{date}_{type}_{theme}_{device}_{designer}_{size}'
    video_naming_rule = '{month}_V_AK_MY_{date}_{type}_{theme}_{device}_{designer}_{size}_{duration}'
    naming_size_abbreviations = @{
        '1080x1080' = '11'
        '1920x1080' = '169'
        '1200x628' = '191'
        '1080x1920' = '916'
        '800x1000' = '45'
    }
    naming_defaults = @{ device = 'SX'; designer = 'AI'; brand_abbreviation = 'AK'; market_abbreviation = $marketProfile.market_abbreviation }
    output_sizes = @('1080x1080','1200x628','800x1000')
    prime_composition_mode = 'deterministic'
    prime_template_set = @{
        schema_version = 2
        selection_mode = 'automatic_family_contrast'
        families = @(
            @{ id = 'light_background'; label = '明亮底图方案'; description = '绿色 AdaKami 标识，适合浅色或明亮的画面。'; templates = @{ '1080x1080' = @{ source_role = 'prime_light_square' }; '1200x628' = @{ source_role = 'prime_light_landscape' }; '800x1000' = @{ source_role = 'prime_light_portrait' } } },
            @{ id = 'dark_background'; label = '深色底图方案'; description = '白色 AdaKami 标识，适合深色或低明度的画面。'; templates = @{ '1080x1080' = @{ source_role = 'prime_dark_square' }; '1200x628' = @{ source_role = 'prime_dark_landscape' }; '800x1000' = @{ source_role = 'prime_dark_portrait' } } }
        )
    }
}
if (-not $marketPack) {
    $marketPack = Invoke-MulticaApi -Method Post -Path '/api/creative/resources' -Body @{
        kind = 'market_pack'
        name = $marketPackName
        description = $marketPackDescription
        config = $marketConfig
    }
}

if ($seedMarketPack) {
    foreach ($template in $primeTemplates) {
        Add-MarketFile -MarketPack $marketPack -Role $template.role -Label $template.label -Path $template.path -Metadata @{ source_type = 'full_transparent_prime_template'; output_size = $template.size; family = $template.family }
    }
    $marketFiles = Get-Items (Invoke-MulticaApi -Method Get -Path "/api/creative/resources/$($marketPack.id)/files") 'files'
    foreach ($obsoleteFile in @($marketFiles | Where-Object { $_.role -like 'prime_*' -and $_.role -notin $requiredPrimeTemplateRoles })) {
        Invoke-MulticaApi -Method Delete -Path "/api/creative/resources/$($marketPack.id)/files/$($obsoleteFile.id)" | Out-Null
    }
    foreach ($appUIPath in $AppUIReferencePaths) {
        Add-MarketFile -MarketPack $marketPack -Role 'app_ui_reference' -Label "$($marketProfile.brand) App UI - $([IO.Path]::GetFileNameWithoutExtension($appUIPath))" -Path $appUIPath -Multiple -Metadata @{
            dimensions = '1080x2160'
            market = $marketProfile.market
            locale = $marketProfile.locale
            tags = @('app-ui', 'homepage', 'new-customer', 'loan-limit')
            description = "参考分析只识别通用 App UI 类型；Planner 根据订单冻结的市场快照从全部 App UI 参考图中选择最匹配的一张。图像编辑必须替换为 $($marketProfile.brand) 自有界面，不得保留或仿造竞品 UI。"
        }
    }
    $marketPack = Invoke-MulticaApi -Method Put -Path "/api/creative/resources/$($marketPack.id)" -Body @{
        name = $marketPackName
        description = $marketPackDescription
        config = $marketConfig
    }
    $marketPack = Invoke-MulticaApi -Method Post -Path "/api/creative/resources/$($marketPack.id)/publish"
}

$autopilots = Get-Items (Invoke-MulticaApi -Method Get -Path '/api/autopilots') 'autopilots'
$autopilot = $autopilots | Where-Object { $_.title -eq $autopilotTitle -or $_.title -eq '印尼竞品素材周度采集' -or $_.title -eq '印尼与马来竞品素材周度采集' } | Select-Object -First 1
$autopilotDescription = @"
每周抓取 AppGrowing $($marketProfile.market_label)的现金贷/金融竞品素材，并创建一个可追踪的 Crawl Run。

工作区创意工厂安装记录负责解析市场资源包、执行小队和参考分析智能体；不要在 AutoPilot 说明中保存 UUID。

竞品：$competitorText
优先竞品：$priorityCompetitorText
地区：$($marketProfile.market_label)
语言：$($marketProfile.language_label)
设备：Android、iOS
媒体：未限定
时间范围：最近 30 天
选材：新素材 40%，投放少于 7 天且曝光估算大于 1K；跑量素材 60%，投放超过 30 天且曝光估算不低于 10M
最多输出：5 张图片
素材类型：仅图片广告（asset_type=image）。视频、非图片和无法识别类型必须在选材前排除，不占用名额，也不进入 Crawl Run 或素材库。

执行真实 AppGrowing 多页图片采集，结果进入创意工厂素材库并关联当前 Crawl Run；图片入库后立即用原生 task fanout 对本次新增图片并发执行逐图创意分析，归档未完成时允许分析读取真实源图片。分析完成后在创意工厂提醒用户选图、确认主题与文案。整个采集与预分析阶段不创建 Issue。授权失效时将 Crawl Run 标为 action_required，并提示用户前往“设置 - 集成”重新绑定，不能用测试数据替代。
"@
if (-not $autopilot) {
    $autopilot = Invoke-MulticaApi -Method Post -Path '/api/autopilots' -Body @{
        title = $autopilotTitle
        description = $autopilotDescription
        assignee_type = 'agent'
        assignee_id = $collector.id
        execution_mode = 'run_only'
    }
} else {
    $autopilot = Invoke-MulticaApi -Method Patch -Path "/api/autopilots/$($autopilot.id)" -Body @{
        title = $autopilotTitle
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
