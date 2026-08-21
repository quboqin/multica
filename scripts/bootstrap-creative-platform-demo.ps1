param(
    [string]$ApiUrl = 'http://127.0.0.1:8080',
    [string]$AppUrl = 'http://localhost:3000',
    [string]$WorkspaceSlug = 'ad-creative-direct-pilot',
    [string]$Token = $env:MULTICA_BOOTSTRAP_TOKEN,
    [string]$CliPath = $env:MULTICA_CLI,
    [string]$CliProfile = $env:MULTICA_BOOTSTRAP_PROFILE,
    [string]$ImageApiKey = $env:MULTICA_IMAGE_API_KEY,
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

$collectorSkill = Set-WorkspaceSkill -Name '素材_技能_采集' -Aliases @('AppGrowing 素材采集') -Description '创建 Crawl Run，只采集真实图片广告，平台最多采集 2 张，并用原生 task fanout 自动预分析新增图片。' -Directory (Join-Path $skillTemplateRoot 'appgrowing-material-collector') -Config @{ kind = 'creative_role'; capability = 'material_collection'; version = 16 }
$diagnosisSkill = Set-WorkspaceSkill -Name '素材_技能_诊断' -Aliases @('创意流程诊断', '出图诊断', 'AppGrowing 采集诊断') -Description '读取创意采集、出图、品牌组件、QC、订单和 daemon/runtime 证据，在允许范围内恢复或给出明确动作。' -Directory (Join-Path $skillTemplateRoot 'creative-flow-diagnostician') -Config @{ kind = 'creative_role'; capability = 'crawl_diagnosis'; version = 5 }
$analysisSkill = Set-WorkspaceSkill -Name '素材_技能_分析' -Aliases @('广告参考分析') -Description '市场中立地读取真实图片，识别可变视觉区域、原图文字及坐标、主题、利益点、语义锚点、App UI 类型和布局约束；numeric 区域只包含可重排还款字段，混合区域必须拆分。' -Directory (Join-Path $skillTemplateRoot 'ad-creative-analysis') -Config @{ kind = 'creative_role'; capability = 'reference_analysis'; version = 17 }
$preAdaptationSkill = Set-WorkspaceSkill -Name '素材_技能_文案适配' -Aliases @('广告预适配') -Description '按冻结资源完成可生产文案与数值适配；我方已审核还款方案优先，按我方可用数量落表，超出的原图数值默认移除，不借用其他期限金额；数值布局说明必须列出每个冻结展示值。' -Directory (Join-Path $skillTemplateRoot 'ad-creative-pre-adaptation') -Config @{ kind = 'creative_role'; capability = 'pre_adaptation'; version = 25 }
$planSkill = Set-WorkspaceSkill -Name '素材_技能_方案' -Aliases @('广告生成方案') -Description '消费冻结分析、逐块文案与市场快照，严格继承顶层非空文案字段，规划 3 个同题变体及品牌组件视觉关系。' -Directory (Join-Path $skillTemplateRoot 'ad-creative-plan') -Config @{ kind = 'creative_role'; capability = 'generation_plan'; version = 34 }
$productionSkill = Set-WorkspaceSkill -Name '素材_技能_出图' -Aliases @('广告图像编辑') -Description '使用 GPT Image 2 提示词模板和冻结业务结构生成无品牌三尺寸底图，严格继承所有非空 approved copy，明确 Input 1/Input 2 角色，候选图通过 candidate_id 受控下载，Prime context 按当前 revision 绑定，横版用 Y 轴压缩避开上下 Prime 组件带，provider 画布使用 16px 合法尺寸再归一化为交付尺寸，使用 canonical generated asset 写回并保存完整 trace 与 parent_direction_sha256；每个尺寸同步登记 Prime context、模型原图和规范化底图，比例异常保留一次压缩归一化，视觉遮挡最多执行一次定向 Image2 重排；缺尺寸不提前 complete，由平台自动续跑，完成底图后调用贴片 Skill。' -Directory (Join-Path $skillTemplateRoot 'ad-creative-production') -Config @{ kind = 'creative_role'; capability = 'image_edit'; version = 87 }
$primeComposeSkill = Set-WorkspaceSkill -Name '素材_技能_贴片' -Aliases @('广告品牌组件合成') -Description '调用后端唯一的确定性 Prime 合成入口，校验合成 JSON，并由后端登记贴片完成过程图、primed 资产和标准 QC/交付交接；不创建 Prime Agent 或 Prime task。' -Directory (Join-Path $skillTemplateRoot 'ad-creative-prime-compose') -Config @{ kind = 'creative_role'; capability = 'prime_compose'; version = 2 }
$directEditSkill = Set-WorkspaceSkill -Name '素材_技能_改图' -Aliases @('广告图片直接修改') -Description '按用户原话和最终图标注 brief 修改固定无品牌底图，再由贴片 Skill 调用平台确定性合成并直接交付，不执行 QC；协议错误复用同一回图修复写回。' -Directory (Join-Path $skillTemplateRoot 'ad-creative-direct-edit') -Config @{ kind = 'creative_role'; capability = 'direct_image_edit'; version = 13 }
$qcSkill = Set-WorkspaceSkill -Name '素材_技能_质检' -Aliases @('广告成图验收') -Description '独立执行 technical 或 visual QC，只记录成图检测与调整建议；证据契约失败由平台自动复用 Prime 资产重跑双 QC。' -Directory (Join-Path $skillTemplateRoot 'ad-creative-qc') -Config @{ kind = 'creative_role'; capability = 'quality_control'; version = 31 }

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

$specialistHandoff = '只处理 task context 指定的对象、revision 和 scope；按绑定 Skill 写结构化领域结果和机器证据。不得创建或修改 Issue，不得用评论代替领域数据。输入、凭证、工具或写回失败时保留已成功对象，写真实 error_code/error_message 并让当前 task 失败；兄弟对象继续。'
$directEditAgentInstructions = @'
全程使用中文。只执行 creative_direct_edit；source asset 不可覆盖，task context 的 source_revision 是上一版，revision 是平台已锁定的输出 revision。精准调整不得再次调用 variant-put 或把 revision 再加一。只下载并使用 source_asset_id/source_attachment_id 指向的同尺寸无品牌底图；若存在 annotation_guide_attachment_id，再把用户最终成图标注 brief 作为第二输入。Input 1 是唯一可编辑无品牌底图，Input 2 只用于读取红框编号、评论位置和固定贴片/标题/Logo 遮挡关系；不得复制红框、编号、Prime 组件、Logo、二维码、商店徽章或官方条款，也不得把 Prime 成图当作可编辑来源。reference_asset_id/reference_attachment_id 仅用于协作对照。

任务号只使用运行时注入的 MULTICA_TASK_ID；不得把 issue_id、adjustment_issue_id、variant_id 或 item_key 当 task_id。使用 Image Edit 返回的完整 JSON 作为 image-edit-result.json，prompt 和 prompt_sha256 只取该 JSON，prompt.txt 仅供展示且末尾换行不能参与 hash。提示词保留用户原话并追加约束：只编辑 Input 1，Input 2 仅为标注/遮挡参考，平台会重新贴回固定组件，不能把标注或 Prime 组件画进底图。人物替换必须是肉眼可见的 replacement，现有人物是移除目标，不是身份、五官、发型、服装、姿势、手势、身形轮廓或构图参考；给出具体不同的新人物属性。只处理 target_size，未修改尺寸沿用平台复制的上一 revision 底图和过程证据，不重新生成。

每一次 Image Edit 实际回图，无论采用还是拒绝，都必须上传并用 diagnostic-asset-put 登记当前尺寸过程图；拒绝回图使用直接改图尝试编号和未采用原因，不能丢失方图或失败图片。若诊断写回遇到 task ownership 错误，修正 task_id 为 MULTICA_TASK_ID；仅在直接改图诊断允许的情况下省略 task_id 重试，不得重新生成图片。只有采用的无品牌底图写入 generated/completed。asset-put 必须同时传 model-result-file、prompt-contract-file、copy-validation-file、normalization-evidence-file；edited-asset.json 不得带 metadata/evidence，CLI 会从四份证据生成它们。出现协议、归属、prompt/hash、证据或上传错误时，复用同一回图和同一模型 JSON 修复后重试；只有没有有效回图或真实视觉失败才执行每尺寸最多一次的模型重试。

最后一个 expected size 写回后调用绑定的素材_技能_贴片，由后端重新贴回官方透明组件并登记 primed/delivered；随后回读订单确认所有尺寸和过程图完整。只有回读成功才允许调用 task complete；否则按协议错误复用同一回图和模型 JSON 修复写回，不重新生成。不创建贴片 task，不创建 QC task，不调用 QC。不得触发采集、分析、方案或标准生产。
'@
$producerInstructions = '全程使用中文。根据 task context.workflow 选择唯一执行分支。' +
    "`n`ncreative_production 分支只执行 creative_production；使用冻结 brief/copy_snapshot、实际 Prime context 和无品牌来源，严格按生产 Skill 的 GPT Image 2 模板写每个尺寸的 prompt：先写 COMPOSITION GATE，再写明确的 Input 1/Input 2 角色；横版拥挤时沿 Y 轴压缩留白、模块间距和行距，不删 approved copy 或表格结构。写 generated assets、lineage 与 CLI 原始模型证据，只补当前 revision 的缺失尺寸。保持 prompt 1800-2800 字符、无坐标、无审计重复，并记录 prompt/hash；prompt-contract 必须带当前 brief 的 parent_direction_sha256。调用 multica image edit/edit-batch 时必须给 Bash 工具设置 timeout_ms 至少 1500000（25 分钟），等待 CLI 返回完整 JSON 后，立即按生产 Skill 调用 register_process_assets.py 登记当前尺寸的 Prime context、模型原图和规范化底图，并回读订单确认 registered 数量和归属。遇到 qc_visual_rework 时只对失败尺寸执行一次 Image2 重排，仍失败就写 action_required。比例重试仍失败时保留最后回图并按 Skill 的 aspect fallback 规则归一，不得丢弃尺寸。creative_production 分支不得处理 direct_edit、品牌组件、QC、采集或分析；只有当前 revision 的全部 expected_sizes 写回 generated/completed、过程证据登记回读成功且绑定的素材_技能_贴片返回后端合成完成后才调用 task complete，缺尺寸不能提前完成，平台会自动创建有上限的 fresh continuation。调用素材_技能_贴片执行后端唯一 Prime 合成入口，不得直接创建 primed 资产或伪造贴片结果。" +
    "`n`ncreative_direct_edit 分支严格执行现有素材_技能_改图契约：`n" + $directEditAgentInstructions +
    "`n`n" + $specialistHandoff
$analyst = Set-AgentDefinition -Name '素材_分析' -Aliases @('广告参考分析智能体') -Description '按 task workflow 读取真实像素、识别可变视觉区域、写市场中立分析，并在后台完成可确认的文案与数值预适配。' -Instructions "全程使用中文。creative_reference_analysis 只写指定 candidate/version 的市场中立 Source Analysis；每个可变原图文字区块必须归入唯一 copy 或 numeric 视觉区域，不能把同一画面组件拆入两条处理路径。creative_pre_adaptation 只消费指定的冻结市场包和文案库，逐区域优先绑定已审核内容；没有兼容已审核片段的普通 headline、subheadline、benefit、supporting 或 cta 区块，必须按市场语言、区块职责、原图语义和可读长度生成一个新的 recommended 文案，source_keys 必须为空数组，recommendation_basis 必须说明依据，页面会标记待用户确认；不得引用不存在的 fragment key。本金、期限、月供、总利息、总还款、利率、法律文字和品牌事实没有审核来源或冻结计算时不得凭空生成，逐项写 missing replacement。多行数值表不要求原图行数与我方方案数量相等：按表格语义和期限从冻结 approved repayment plan 取我方兼容方案，实际渲染行数取原图可渲染行数与我方可用方案数的较小值；每个实际渲染行写 numeric_layouts，且每个 layout 的 render_instruction 必须逐字列出其 scenario_ids 对应 selection.values 中每个 target_columns 的完整冻结展示值，不能只写按行展示；源图多出的数值块逐项写空 missing 作为默认移除项。没有我方某一期限方案时不得借用其他期限金额；整体重构为我方支持的期限列，或让该列源块留空移除。不能把整表硬塞进一个 layout，也不能伪装成已绑定或改写原图事实。输入和产物不得混用，不生成图片，不修改市场包或文案库。$specialistHandoff" -SkillIDs @($analysisSkill.id, $preAdaptationSkill.id) -RuntimeID $runtimeID -Model 'gpt-5.6-terra' -ThinkingLevel 'medium' -MaxConcurrentTasks 6
$collector = Set-AgentDefinition -Name '素材_采集' -Aliases @('AppGrowing 素材采集智能体') -Description '按 task 配置创建 Crawl Run，只导入真实图片广告并委派新增图片分析。' -Instructions "全程使用中文。只执行 task context 和 AutoPilot 明确的 AppGrowing 查询；只导入 asset_type=image，视频、非图片和未知类型不占用采集配额。analysis_agent_id、工作区资源和数量限制由平台按当前 workspace installation 注入，不从名称、评论或历史说明猜测。筛选、分页、预算和 fallback 由 task/平台配置决定。结果、证据和失败写 Crawl Run；导入后用原生 fanout 委派本次新增图片，不创建 Issue，不使用测试数据。" -SkillIDs @($collectorSkill.id) -RuntimeID $runtimeID -MaxConcurrentTasks 1
$diagnostician = Set-AgentDefinition -Name '素材_诊断' -Aliases @('创意流程诊断智能体', '出图诊断智能体', 'AppGrowing 采集诊断智能体') -Description '诊断创意采集、出图、品牌组件、QC、订单状态和 daemon/runtime 异常，并通过平台入口执行受控恢复。' -Instructions '全程使用中文。处理 creative_crawl_diagnosis、订单短 ID、Variant 标签、页面卡片文案、报错文本和用户明确指向的创意流程诊断。先定位当前订单、order item、Variant、revision、task、daemon/runtime 与 Skill 快照证据，再给结论；需要恢复时只通过 multica CLI 或平台 API 重试、取消、fanout、推进明确授权的 Variant revision 或调用现有修复入口。不得直接写 DB、修改凭证、业务筛选、市场包、文案库或生产代码，不得把诊断图当成交付资产；修改前说明对象和原因，修改后回读验证。' -SkillIDs @($diagnosisSkill.id) -RuntimeID $runtimeID -Model 'gpt-5.6-luna' -ThinkingLevel 'medium' -MaxConcurrentTasks 2
$planner = Set-AgentDefinition -Name '素材_方案' -Aliases @('生成方案智能体') -Description '消费冻结分析、文案与市场快照，写 3 个同题 Variant 并委派生产。' -Instructions "全程使用中文。只执行 creative_plan。copy_snapshot 与 market snapshot 是唯一文案、事实和资源真值；不得重选或改写。写 V01-V03 结构化 brief，保留语义与主体，只改变表达；将缺失 production items 一次 fanout。需要输入时写 needs_input/action_required，不生成图片。$specialistHandoff" -SkillIDs @($planSkill.id) -RuntimeID $runtimeID -Model 'gpt-5.6-terra' -ThinkingLevel 'medium' -MaxConcurrentTasks 6
$producer = Set-AgentDefinition -Name '素材_出图' -Aliases @('图像编辑智能体') -Description '按 GPT Image 2 提示词模板执行标准出图或用户标注精准改图；分别生成或修改无品牌底图，调用贴片 Skill 完成后端合成，标准出图进入 QC，精准改图直接交付。' -Instructions $producerInstructions -SkillIDs @($productionSkill.id, $directEditSkill.id, $primeComposeSkill.id) -RuntimeID $runtimeID -Model 'gpt-5.6-terra' -ThinkingLevel 'low' -MaxConcurrentTasks 10
$reviewer = Set-AgentDefinition -Name '素材_质检' -Aliases @('广告验收智能体') -Description '独立执行一个 technical 或 visual lane，按三道闸门验收成图并输出尺寸级阻断或建议；证据契约错误由平台自动恢复双 QC。' -Instructions "全程使用中文。只执行 context 指定 QC lane，读取同 Variant/revision/expected_sizes 的完整品牌组件包。technical 检查文件、尺寸、品牌组件、模板布局和机器可见性证据；visual 必须逐张检查冻结文案和关键组件是否完整、顶部和底部 Prime 是否遮挡正文、Logo/条款是否可读，以及中部是否出现空框或内容缺失，并核对 generated evidence 的 parent_direction_sha256 与当前 brief 一致。写独立 QC Report 后调用 qc-finalize 完成归档。对 actual_prime_obstruction、official_prime_text_unreadable、generated_content_missing 写 failed 和每个失败尺寸一个 blocking_failure；证据缺失、manifest/compose 不匹配等 delegation/contract 错误也要写结构化 blocking_failure，平台会自动复用已完成 Prime 资产重跑 technical 和 visual 一次。其他发现写 warning。服务端仅对真实 Prime 遮挡或官方 Prime 文字不可读最多自动返工当前 Variant 一轮，不能改 Prime，不能影响兄弟 Variant；预测遮挡和关键内容缺失保留为人工阻断。$specialistHandoff" -SkillIDs @($qcSkill.id) -RuntimeID $runtimeID -Model 'gpt-5.6-terra' -ThinkingLevel 'medium' -MaxConcurrentTasks 6

$imageCredentialConfigured = Set-AgentImageCredential -AgentID $producer.id -ApiKey $ImageApiKey

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
$leaderSkill = Set-WorkspaceSkill -Name '素材_技能_流程' -Aliases @('素材_技能_统筹', '创意素材协作', '素材小队 Leader 编排') -Description '使用原生 task fanout 启动并恢复标准生产或直接改图，汇总结构化结果。' -Directory $leaderSkillDirectory -Config @{ kind = 'creative_role'; capability = 'creative_leadership'; version = 49 }
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
$seedMarketPack = $marketPackIsNew -or $ResetBusinessConfig -or $marketPackHasLegacyQRConfig -or -not $marketPackIsCurrentPublished -or -not $marketPackHasPreAdaptationDefault -or -not $marketPackHasCalculationRules -or -not $marketPackHasPrimeTemplateSet -or -not $marketPackHasExactPrimeFiles
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
    compliance_rules = '只能使用文案库已审核的金融事实；不得复制竞品品牌、金额、法律文字或二维码。完整 Prime 模板按上传文件原样叠加，平台不生成或修改品牌与合规贴片。'
    naming_rule = '{month}_P_AK_MY_{date}_{type}_{theme}_{device}_{designer}_{size}'
    video_naming_rule = '{month}_V_AK_MY_{date}_{type}_{theme}_{device}_{designer}_{size}_{duration}'
    naming_size_abbreviations = @{
        '1080x1080' = '11'
        '1920x1080' = '169'
        '1200x628' = '191'
        '1080x1920' = '916'
        '800x1000' = '45'
    }
    naming_defaults = @{ device = 'SX'; designer = 'AI'; brand_abbreviation = 'AK'; market_abbreviation = 'MY' }
    output_sizes = @('1080x1080','1200x628','800x1000')
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
        name = 'AdaKami Indonesia 市场资源包'
        description = '印尼市场可编辑配置与版本化附件；新市场可在平台复制后替换文案库、规则和资源槽位。'
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

工作区创意工厂安装记录负责解析市场资源包、执行小队和参考分析智能体；不要在 AutoPilot 说明中保存 UUID。

竞品：Easycash、Kredit Pintar、Adapundi、BantuSaku、Rupiah Cepat、UATAS、JULO
优先竞品：Easycash、Kredit Pintar、Adapundi
地区：印度尼西亚
语言：印度尼西亚语
设备：Android、iOS
媒体：未限定
时间范围：最近 30 天
选材：新素材 40%，投放少于 7 天且曝光估算大于 1K；跑量素材 60%，投放超过 30 天且曝光估算不低于 10M
素材类型：仅图片广告（asset_type=image）。视频、非图片和无法识别类型必须在选材前排除，不占用 2 条配额，也不进入 Crawl Run 或素材库。
最多输出：2 张图片

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
