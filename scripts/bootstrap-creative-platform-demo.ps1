param(
    [string]$ApiUrl = 'http://127.0.0.1:8080',
    [string]$WorkspaceSlug = 'ad-creative-direct-pilot',
    [string]$Token = $env:MULTICA_BOOTSTRAP_TOKEN,
    [string]$ImageApiKey = $env:MULTICA_IMAGE_API_KEY,
    [string]$PrimeDirectory = 'E:\Documents\WXWork\1688853548483782\Cache\File\2026-07\Prime Template - PNG file',
    [string[]]$AppUIReferencePaths = @('E:\Documents\WXWork\1688853548483782\Cache\Image\2026-07\首页-新客未戳额(1).jpg'),
    [switch]$ResetBusinessConfig
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

if (-not $Token) {
    throw 'Set MULTICA_BOOTSTRAP_TOKEN or pass -Token. The script never stores the token.'
}

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
        [string]$RuntimeID,
        [string]$Model = 'gpt-5.6-luna',
        [string]$ThinkingLevel = 'low',
        [ValidateRange(1, 12)][int]$MaxConcurrentTasks = 3
    )
    $agents = Get-Items (Invoke-MulticaApi -Method Get -Path '/api/agents') ''
    $agent = $agents | Where-Object name -eq $Name | Select-Object -First 1
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
    '广告成图验收'
)
$existingSkills = Get-Items (Invoke-MulticaApi -Method Get -Path '/api/skills') ''
foreach ($legacy in $existingSkills | Where-Object {
    $legacySkillNames -contains $_.name -or $_.name -like '广告创意生产（*'
}) {
    Invoke-MulticaApi -Method Delete -Path "/api/skills/$($legacy.id)" | Out-Null
}

$collectorSkill = Set-WorkspaceSkill -Name 'AppGrowing 素材采集' -Description '创建 Crawl Run，执行真实 AppGrowing 多页采集，并用原生 task fanout 自动预分析新增图片。' -Directory (Join-Path $skillTemplateRoot 'appgrowing-material-collector') -Config @{ kind = 'creative_role'; capability = 'material_collection'; version = 9 }
$analysisSkill = Set-WorkspaceSkill -Name '广告参考分析' -Description '市场中立地读取真实图片，识别主题、金融利益点、原图语义锚点、App UI 类型和通用布局。' -Directory (Join-Path $skillTemplateRoot 'ad-creative-analysis') -Config @{ kind = 'creative_role'; capability = 'reference_analysis'; version = 11 }
$marketPackExtractionSkill = Set-WorkspaceSkill -Name '市场包组件识别' -Description '读取一张完整成图，发现数量不定的品牌、二维码、商店与合规组件并回写待确认候选。' -Directory (Join-Path $skillTemplateRoot 'ad-creative-market-pack-extraction') -Config @{ kind = 'creative_role'; capability = 'market_pack_component_extraction'; version = 1 }
$planSkill = Set-WorkspaceSkill -Name '广告生成方案' -Description '消费已确认文案和冻结市场快照，选择品牌 UI 并规划 3 个同题创意变体。' -Directory (Join-Path $skillTemplateRoot 'ad-creative-plan') -Config @{ kind = 'creative_role'; capability = 'generation_plan'; version = 21 }
$productionSkill = Set-WorkspaceSkill -Name '广告图像编辑' -Description '一个变体 task 内生成方形母版并发重排横竖版；三尺寸共享内容族和修订。' -Directory (Join-Path $skillTemplateRoot 'ad-creative-production') -Config @{ kind = 'creative_role'; capability = 'image_edit'; version = 23 }
$directEditSkill = Set-WorkspaceSkill -Name '广告图片直接修改' -Description '基于用户自然语言和指定底图执行自由修改；正式交付按 expected_sizes 补 Prime 与独立 QC。' -Directory (Join-Path $skillTemplateRoot 'ad-creative-direct-edit') -Config @{ kind = 'creative_role'; capability = 'direct_image_edit'; version = 5 }
$composeSkill = Set-WorkspaceSkill -Name 'Prime 完整贴图' -Description '按变体一次包装 expected_sizes，并把最终图和逐图机器证据写入领域资产。' -Directory (Join-Path $skillTemplateRoot 'ad-creative-prime-compose') -Config @{ kind = 'creative_role'; capability = 'prime_compose'; version = 19 }
$qcSkill = Set-WorkspaceSkill -Name '广告成图验收' -Description '通过并发技术 QC 与视觉 QC 检查 expected_sizes 的四角、画质和内容一致性。' -Directory (Join-Path $skillTemplateRoot 'ad-creative-qc') -Config @{ kind = 'creative_role'; capability = 'quality_control'; version = 22 }

$agents = Get-Items (Invoke-MulticaApi -Method Get -Path '/api/agents') ''
$leaderSeed = $agents | Where-Object name -eq '素材小队 Leader' | Select-Object -First 1
if (-not $leaderSeed) { throw '素材小队 Leader does not exist' }
$runtimeID = $leaderSeed.runtime_id

$specialistHandoff = '只处理 task context 明确指定的领域对象、修订和作用域。结构化交付与证据写回 Creative Order、Variant、Asset、Source Analysis 或 QC Report；不要新建或修改 Issue，不要 @Leader，不要用评论代替领域数据。出现凭证、输入或工具问题时写入当前对象的 error_code/error_message 并让 task 失败；兄弟对象继续执行。'
$analyst = Set-AgentDefinition -Name '广告参考分析智能体' -Description '读取真实图片；按任务类型执行竞品参考分析或市场包品牌组件发现。' -Instructions "全程使用中文，严格按 task context 的 workflow 选择一个 Skill。creative_reference_analysis 只分析指定 candidate_id 和 Crawl Run，市场中立地写入 Source Analysis，不得读取市场包；creative_market_pack_component_extraction 只分析指定完整成图，发现全部可复用品牌与合规组件并写回待确认候选，不得修改或发布市场包。两个流程不得混用输入或产物，不生成图片。$specialistHandoff" -SkillIDs @($analysisSkill.id, $marketPackExtractionSkill.id) -RuntimeID $runtimeID -Model 'gpt-5.6-luna' -ThinkingLevel 'low' -MaxConcurrentTasks 6
$collector = Set-AgentDefinition -Name 'AppGrowing 素材采集智能体' -Description '运行 Crawl Run，导入真实素材并并发委派新增图片预分析。' -Instructions "全程使用中文。只执行 task context 指定的真实 AppGrowing 查询。广告参考分析智能体 ID 固定为 $($analyst.id)，必须作为 analysis_agent_id 写入 Crawl Run 参数并用于 fanout，不得按名称猜测。逐家核对普通竞品至少 3 页、优先竞品至少 5 页；过滤工作区历史重复后继续轮询翻页，直到凑足平台新素材、达到预算或没有更多结果。单家接口为空或失败时对该家启用 Playwright 补查。结果、逐页证据和失败原因写入 Crawl Run；采集入库后立即用原生 task fanout 委派本次新增图片，分析优先读取平台归档，归档未完成时回退到真实源图片。不得创建 Issue 或使用测试数据。" -SkillIDs @($collectorSkill.id) -RuntimeID $runtimeID -MaxConcurrentTasks 2
$planner = Set-AgentDefinition -Name '生成方案智能体' -Description '消费已确认文案和冻结市场快照，把订单项转成 3 个同题创意变体。' -Instructions "全程使用中文。读取 task context 指定的 Order Item、Source Analysis、copy_snapshot 和冻结市场资源快照；不得重新推荐、选择、拼接或改写文案。需要替换 App UI 时，由本角色根据分析的通用 UI 类型从冻结市场附件中选择最匹配的品牌 UI。保持原图业务语义、信息机制、关键视觉和主色家族，写入 V01、V02、V03 三个版式与信息组织差异明确的 Variant 规格。三个变体就绪后放入同一个 manifest 一次 fanout 给图像编辑智能体，不得串行委派。只有用户明确放开时才能换场景或换色系。不生成图片。$specialistHandoff" -SkillIDs @($planSkill.id) -RuntimeID $runtimeID -Model 'gpt-5.6-terra' -ThinkingLevel 'medium' -MaxConcurrentTasks 3
$producer = Set-AgentDefinition -Name '图像编辑智能体' -Description '执行标准变体三尺寸生产和精准返工。' -Instructions "全程使用中文。只执行广告图像编辑 Skill 的 creative_production task。初次生产时一个 task 负责一个 Variant：生成 1080x1080 方形母版后，立即把它作为第一参考并发原生重排 1200x628 和 800x1000；三张共享 asset_family_id、批准文案、业务语义、人物和产品。只恢复 missing_sizes，禁止重做已通过尺寸。底图硬区风险交给 Prime/QC 对最终图判断。普通返工使用上一版对应无品牌底图，replan 使用原候选图；不得把带 Prime 的最终图作为第一输入。不得处理 direct_edit，不得触发采集、参考分析或三变体规划。$specialistHandoff" -SkillIDs @($productionSkill.id) -RuntimeID $runtimeID -Model 'gpt-5.6-terra' -ThinkingLevel 'low' -MaxConcurrentTasks 3
$directEditor = Set-AgentDefinition -Name '图片直接修改智能体' -Description '只按用户自然语言修改指定 expected_sizes 的固定来源底图。' -Instructions "全程使用中文。只执行广告图片直接修改 Skill 的 creative_direct_edit task。读取 task context 固定的 source_asset_id、source_attachment_id、user_request、target_size、expected_sizes、delivery_mode、prime_agent_id、reviewer_agent_id 和 source revision；源资产不可覆盖，输出必须写为 source revision + 1，并以 source asset 为 derived_from_asset_id。不得执行采集、参考分析、三变体规划或普通生产。preview 只写 generated asset；publish 才按同一 expected_sizes 委派 Prime，Prime 再委派两路 QC。$specialistHandoff" -SkillIDs @($directEditSkill.id) -RuntimeID $runtimeID -Model 'gpt-5.6-terra' -ThinkingLevel 'low' -MaxConcurrentTasks 2
$composer = Set-AgentDefinition -Name 'Prime 包装智能体' -Description '按变体一次包装 expected_sizes 并写入逐图机器证据。' -Instructions "全程使用中文。每个 task 只处理 context 指定的 Variant、revision、expected_sizes 和对应 1-3 张无品牌底图；一次 batch 应用冻结市场快照中的版本化 Prime、条款、二维码和商店徽章。逐张写入来源 Asset、模板、manifest、最终附件、二维码解码和四角/底部校验证据。单项失败不得丢弃其他成功项，不创建 Issue。$specialistHandoff" -SkillIDs @($composeSkill.id) -RuntimeID $runtimeID -Model 'gpt-5.6-terra' -ThinkingLevel 'low' -MaxConcurrentTasks 3
$reviewer = Set-AgentDefinition -Name '广告验收智能体' -Description '并发执行技术 QC 和视觉 QC，重点检查四角、画质和 expected_sizes 内容一致性。' -Instructions "全程使用中文。严格按 task context 的 workflow、revision 和 expected_sizes 执行技术 QC 或视觉 QC。技术 QC 检查尺寸、文件、四角/底部 Prime、二维码和真实硬区遮挡；视觉 QC 检查清晰度、伪影、批准文案、原图语义，并在多尺寸时检查同变体一致性。原图本来存在的人物或装饰不能仅因几何预测判失败。blocking_failures 非空时 status 必须为 failed。写入独立 QC Report 后必须调用 qc-finalize，由服务端事务化收口；失败只进入 action_required，不自动返工。只指出实际失败尺寸，不影响其他变体。$specialistHandoff" -SkillIDs @($qcSkill.id) -RuntimeID $runtimeID -Model 'gpt-5.6-terra' -ThinkingLevel 'medium' -MaxConcurrentTasks 6

$producerImageCredentialConfigured = Set-AgentImageCredential -AgentID $producer.id -ApiKey $ImageApiKey
$directEditorImageCredentialConfigured = Set-AgentImageCredential -AgentID $directEditor.id -ApiKey $ImageApiKey
$imageCredentialConfigured = $producerImageCredentialConfigured -and $directEditorImageCredentialConfigured

$squads = Get-Items (Invoke-MulticaApi -Method Get -Path '/api/squads') ''
$squad = $squads | Where-Object name -eq 'AdaKami 素材小队' | Select-Object -First 1
if (-not $squad) {
    $squad = Invoke-MulticaApi -Method Post -Path '/api/squads' -Body @{
        name = 'AdaKami 素材小队'
        description = '从 AppGrowing 候选采集、逐图文案确认到每张素材 3 个创意、每创意 3 个尺寸的修图交付。'
        leader_id = $leaderSeed.id
    }
}
Invoke-MulticaApi -Method Put -Path "/api/squads/$($squad.id)" -Body @{
    instructions = 'Leader 读取 Creative Order 领域状态以及名册中每个智能体的职责和平台 Skill，动态选择成员。一个订单只关联一个用户可见 Issue；分析、方案、变体生成、Prime 和 QC 通过原生 task fanout 委派，不创建子 Issue。领域对象保存过程与证据，Issue 只保留用户目标、决定、真实阻塞和最终验收。'
} | Out-Null

$memberDefinitions = @(
    @{ agent = $collector; role = '素材采集' },
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
$leaderSkill = Set-WorkspaceSkill -Name '创意素材协作' -Aliases @('素材小队 Leader 编排') -Description '使用原生 task fanout 串联标准三变体生产或独立直接改图、Prime、并发 QC 与增量发布。' -Directory $leaderSkillDirectory -Config @{ kind = 'creative_role'; capability = 'creative_leadership'; version = 46 }
$leader = Set-AgentDefinition -Name '素材小队 Leader' -Description '管理 Creative Order 的并发生产、独立直接改图和结构化结果发布。' -Instructions '全程使用中文。只负责判断、批量委派、状态收口和发布，不代替专业成员执行。严格遵循创意素材协作 Skill：一个订单只关联一个用户 Issue；所有机器阶段使用原生 task fanout，禁止创建分析、变体、Prime 或 QC 子 Issue。Leader 只发起方案并在人工重试或异常恢复时补齐任务；Planner、Production、Prime 各自只委派直接下一阶段。按 target agent + source + item_key 去重，不能因 source 下已有其他 item 就跳过。direct_edit 订单只能委派“图片直接修改”角色，绝不触发采集、参考分析、三变体规划或普通生产；preview 不进入正式交付，publish 才按 expected_sizes 继续 Prime 与双路 QC。每次唤醒读取订单领域状态和冻结 squad snapshot，一次创建所有已就绪任务后立即结束，不轮询、不重复委派。' -SkillIDs @($leaderSkill.id) -RuntimeID $runtimeID -Model 'gpt-5.6-terra' -ThinkingLevel 'medium' -MaxConcurrentTasks 2

$resources = Get-Items (Invoke-MulticaApi -Method Get -Path '/api/creative/resources') 'resources'
$copyLibrary = $resources | Where-Object { $_.kind -eq 'copy_library' -and $_.name -eq 'AdaKami Indonesia 文案库' } | Select-Object -First 1
$copyLibraryIsNew = -not $copyLibrary
if (-not $copyLibrary) {
    $copyLibrary = Invoke-MulticaApi -Method Post -Path '/api/creative/resources' -Body @{
        kind = 'copy_library'
        name = 'AdaKami Indonesia 文案库'
        description = '可组合印尼语文案：业务维护片段、组合方案、产品事实和计算规则。'
        config = @{ schema_version = 2; market = 'Indonesia'; locale = 'id-ID' }
    }
}

# The copy library is a versioned business configuration. Recipes assemble reusable
# Indonesian fragments with approved product facts; production consumes only the
# frozen order-item snapshot and never reads mutable library data.
$copyLibraryConfig = @{
    schema_version = 2
    market = 'Indonesia'
    locale = 'id-ID'
    source = @{
        name = '飞书图片案例 / 文案分组'
        url = 'https://my.feishu.cn/wiki/YKUqwgHNEibzw0k5RVUcZsW2nJh?sheet=elM4tt'
        sync_status = 'pending'
        note = '飞书 MCP 当前无法读取 Sheet 子块；还款 Excel 公式尚未验证。当前仅使用已确认的 Prime 文案，未猜测任何公式。'
    }
    fragments = @(
        @{ id = '10000000-0000-4000-8000-000000000001'; key = 'flexible-loan-headline'; name = '灵活借款主标题'; creative_types = @('num','repayment_plan'); role = 'headline'; text = 'Pinjaman Fleksibel Tanpa Ribet'; tags = @('pinjaman','fleksibel'); status = 'approved' }
        @{ id = '10000000-0000-4000-8000-000000000002'; key = 'num-limit'; name = '额度信息'; creative_types = @('num'); role = 'benefit'; text = "Limit hingga`n{{fact.limit_max.copy_text}}"; tags = @('num','limit','额度'); status = 'approved' }
        @{ id = '10000000-0000-4000-8000-000000000003'; key = 'repayment-plan'; name = '期限与利率'; creative_types = @('repayment_plan'); role = 'benefit'; text = "Pilihan Tenor`n{{fact.tenor_range.copy_text}}`nBunga mulai dari`n{{fact.interest_rate_from.copy_text}}"; tags = @('repayment_plan','tenor','bunga','分期'); status = 'approved' }
        @{ id = '10000000-0000-4000-8000-000000000004'; key = 'apply-now'; name = '立即申请'; creative_types = @('num','repayment_plan'); role = 'cta'; text = 'AJUKAN SEKARANG'; tags = @('cta'); status = 'approved' }
    )
    recipes = @(
        @{ id = '20000000-0000-4000-8000-000000000001'; key = 'num-standard'; name = 'NUM · 额度'; creative_type = 'num'; description = '突出可申请额度，用于数字利益点素材。'; fragment_ids = @{ headline = @('10000000-0000-4000-8000-000000000001'); benefit = @('10000000-0000-4000-8000-000000000002'); cta = @('10000000-0000-4000-8000-000000000004') }; match_tags = @('NUM','额度','limit','jumlah','Rp'); status = 'approved' }
        @{ id = '20000000-0000-4000-8000-000000000002'; key = 'repayment-plan-standard'; name = '还款计划 · 期限与利率'; creative_type = 'repayment_plan'; description = '突出期限和起始利率，用于还款计划素材。'; fragment_ids = @{ headline = @('10000000-0000-4000-8000-000000000001'); benefit = @('10000000-0000-4000-8000-000000000003'); cta = @('10000000-0000-4000-8000-000000000004') }; match_tags = @('还款','分期','tenor','cicilan','angsuran','bunga'); status = 'approved' }
    )
    product_facts = @(
        @{ id = '30000000-0000-4000-8000-000000000001'; key = 'limit_max'; label = '最高额度'; value = '80000000'; copy_text = 'Rp80.000.000'; source = 'Prime Template 11-01.png'; status = 'approved' }
        @{ id = '30000000-0000-4000-8000-000000000002'; key = 'tenor_range'; label = '期限范围'; value = '3-12'; copy_text = '3-12 Bulan'; source = 'Prime Template 11-01.png'; status = 'approved' }
        @{ id = '30000000-0000-4000-8000-000000000003'; key = 'interest_rate_from'; label = '起始利率'; value = '0.03%'; copy_text = '0,03%*'; source = 'Prime Template 11-01.png'; status = 'approved' }
    )
    calculation_rules = @()
    recommendation_policy = @{ type_weight = 1000; tag_weight = 80; concise_weight = 1; default_creative_type = 'num' }
}
$seedCopyLibrary = $copyLibraryIsNew -or $ResetBusinessConfig -or ([int]$copyLibrary.published_version -lt 1)
if ($seedCopyLibrary) {
    $copyLibrary = Invoke-MulticaApi -Method Put -Path "/api/creative/resources/$($copyLibrary.id)" -Body @{
        name = $copyLibrary.name
        description = '可组合印尼语文案：业务维护片段、组合方案、产品事实和计算规则。'
        config = $copyLibraryConfig
    }
    $copyLibrary = Invoke-MulticaApi -Method Post -Path "/api/creative/resources/$($copyLibrary.id)/publish"
}

$resources = Get-Items (Invoke-MulticaApi -Method Get -Path '/api/creative/resources') 'resources'
$marketPack = $resources | Where-Object { $_.kind -eq 'market_pack' -and $_.name -eq 'AdaKami Indonesia 市场资源包' } | Select-Object -First 1
$marketPackIsNew = -not $marketPack
$seedMarketPack = $marketPackIsNew -or $ResetBusinessConfig -or ([int]$marketPack.published_version -lt 1)
$primeComponentDirectory = Join-Path $PrimeDirectory '.multica-prime-components-v2'
$primeExtractionManifestPath = Join-Path $primeComponentDirectory 'extract-manifest.json'
$primeLogoPath = Join-Path $primeComponentDirectory 'adakami-prime-logo.png'
$primeQRPath = Join-Path $primeComponentDirectory 'adakami-prime-qr.png'
$primeStoreBadgesPath = Join-Path $primeComponentDirectory 'adakami-prime-store-badges.png'
$primeAFPIPath = Join-Path $primeComponentDirectory 'adakami-prime-afpi.png'
$primePindaiLegalPath = Join-Path $primeComponentDirectory 'adakami-prime-pindai-legal.png'
$primeExtractionManifest = @{
    source = Join-Path $PrimeDirectory '11-01.png'
    authored_size = @(1080, 1080)
    components = @(
        @{ role = 'prime_logo'; source_rect = @(30, 28, 314, 100); output = $primeLogoPath; background = 'transparent' },
        @{ role = 'prime_qr'; source_rect = @(983, 29, 1053, 99); output = $primeQRPath; background = 'opaque'; normalize_qr = $true; expected_payload = 'https://www.adakami.id/termsandconditions' },
        @{ role = 'prime_store_badges'; source_rect = @(30, 1013, 284, 1051); output = $primeStoreBadgesPath; background = 'transparent' },
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
            @{ id = 'terms'; label = '条款文字'; kind = 'text'; enabled = $true; source_role = ''; content = "Syarat dan ketentuan berlaku`nScan QR untuk membaca`nsyarat & ketentuan"; backdrop_rule = 'quiet' },
            @{ id = 'qr'; label = '二维码'; kind = 'qr'; enabled = $true; source_role = 'prime_qr'; content = ''; backdrop_rule = 'light' },
            @{ id = 'store_badges'; label = '应用商店标识'; kind = 'image'; enabled = $true; source_role = 'prime_store_badges'; content = ''; backdrop_rule = 'quiet' },
            @{ id = 'regulatory'; label = '监管说明'; kind = 'text'; enabled = $true; source_role = ''; content = 'AdaKami berizin dan diawasi oleh OJK  •  AdaKami merupakan anggota'; backdrop_rule = 'quiet' },
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
                    terms = @{ destination_rect = @(989, 24, 1130, 68) }
                    qr = @{ destination_rect = @(1133, 20, 1185, 73) }
                    store_badges = @{ destination_rect = @(21, 582, 187, 607) }
                    regulatory = @{ destination_rect = @(793, 572, 1090, 608) }
                    afpi = @{ destination_rect = @(1093, 565, 1138, 609) }
                    pindai_legal = @{ destination_rect = @(1140, 552, 1182, 610) }
                }
            }
            '800x1000' = @{
                components = @{
                    logo = @{ destination_rect = @(25, 23, 234, 76) }
                    terms = @{ destination_rect = @(573, 26, 724, 73) }
                    qr = @{ destination_rect = @(724, 22, 779, 76) }
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
    Add-MarketFile -MarketPack $marketPack -Role 'prime_qr' -Label 'Prime · 条款二维码' -Path $primeQRPath -Metadata @{ source_type = 'standalone_component'; reused_across_sizes = $true }
    Add-MarketFile -MarketPack $marketPack -Role 'prime_store_badges' -Label 'Prime · 应用商店标识' -Path $primeStoreBadgesPath -Metadata @{ source_type = 'standalone_component'; reused_across_sizes = $true }
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
最多输出：25 条

执行真实 AppGrowing 多页采集，结果进入创意工厂素材库并关联当前 Crawl Run；素材入库后立即用原生 task fanout 对本次新增图片并发执行逐图创意分析，归档未完成时允许分析读取真实源图片。分析完成后在创意工厂提醒用户选图、确认主题与文案。整个采集与预分析阶段不创建 Issue。授权失效时将 Crawl Run 标为 action_required，并提示用户前往“设置 - 集成”重新绑定，不能用测试数据替代。
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
if (-not ($triggers | Where-Object kind -eq 'schedule')) {
    Invoke-MulticaApi -Method Post -Path "/api/autopilots/$($autopilot.id)/triggers" -Body @{
        kind = 'schedule'
        cron_expression = '0 9 * * 1'
        timezone = 'Asia/Shanghai'
        label = '每周一 09:00'
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
