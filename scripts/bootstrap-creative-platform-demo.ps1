param(
    [string]$ApiUrl = 'http://127.0.0.1:8080',
    [string]$WorkspaceSlug = 'ad-creative-direct-pilot',
    [string]$Token = $env:MULTICA_BOOTSTRAP_TOKEN,
    [string]$CatalogPath = 'C:\Users\zhangzhenyu\ad-creative-factory-lean\profiles\id-adakami\catalog\copy_catalog.json',
    [string]$PrimeDirectory = 'C:\Users\zhangzhenyu\ad-creative-factory\refference\Prime Template - PNG file',
    [string]$BrandGuidelinePath = 'C:\Users\zhangzhenyu\ad-creative-factory\docs\AdaKami成图规则.md',
    [string[]]$AppUIReferencePaths = @('E:\Documents\WXWork\1688853548483782\Cache\Image\2026-07\首页-新客未戳额(1).jpg')
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
        [ValidateRange(1, 8)][int]$MaxConcurrentTasks = 3
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
            model = ''
            thinking_level = ''
        }
    } else {
        $agent = Invoke-MulticaApi -Method Put -Path "/api/agents/$($agent.id)" -Body @{
            description = $Description
            instructions = $Instructions
            visibility = 'workspace'
            max_concurrent_tasks = $MaxConcurrentTasks
        }
    }
    Invoke-MulticaApi -Method Put -Path "/api/agents/$($agent.id)/skills" -Body @{ skill_ids = $SkillIDs } | Out-Null
    return $agent
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
    $roleFiles = @($files | Where-Object role -eq $Role)
    $matching = $roleFiles | Where-Object {
        $_.filename -eq $filename -and $_.metadata.sha256 -eq $hash
    } | Select-Object -First 1
    if ($matching -and ($Multiple -or $roleFiles.Count -eq 1)) { return }
    if (-not $Multiple) {
        foreach ($existing in $roleFiles) {
            Invoke-MulticaApi -Method Delete -Path "/api/creative/resources/$($MarketPack.id)/files/$($existing.id)" | Out-Null
        }
    }
    $uploaded = Invoke-RestMethod -Method Post -Uri "$ApiUrl/api/upload-file" -Headers $headers -Form @{ file = Get-Item -LiteralPath $Path }
    $fileMetadata = @{} + $Metadata
    $fileMetadata.sha256 = $hash
    $fileMetadata.source_filename = $filename
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

$collectorSkill = Set-WorkspaceSkill -Name 'AppGrowing 素材采集' -Description '将自然语言筛选条件转换成真实 AppGrowing 多页查询并写入父 Issue 候选池。' -Directory (Join-Path $skillTemplateRoot 'appgrowing-material-collector') -Config @{ kind = 'creative_role'; capability = 'material_collection'; version = 1 }
$analysisSkill = Set-WorkspaceSkill -Name '广告参考分析' -Description '读取真实图片，识别主题、金融利益点、文字证据、App UI 和构图风险，并回写平台创意简报。' -Directory (Join-Path $skillTemplateRoot 'ad-creative-analysis') -Config @{ kind = 'creative_role'; capability = 'reference_analysis'; version = 2 }
$planSkill = Set-WorkspaceSkill -Name '广告生成方案' -Description '把“主题 × 主利益点”、参考分析和逐图文案快照转成 3 个创意变体及其原生三尺寸生成规格。' -Directory (Join-Path $skillTemplateRoot 'ad-creative-plan') -Config @{ kind = 'creative_role'; capability = 'generation_plan'; version = 3 }
$productionSkill = Set-WorkspaceSkill -Name '广告图像编辑' -Description '按创意母版和原生尺寸重排调用平台图像接口，最多并发执行 5 个图片任务。' -Directory (Join-Path $skillTemplateRoot 'ad-creative-production') -Config @{ kind = 'creative_role'; capability = 'image_edit'; version = 2 }
$composeSkill = Set-WorkspaceSkill -Name 'Prime 完整贴图' -Description '从市场资源包读取完整 Prime 资产，按创意批量完成三尺寸贴图和二维码校验。' -Directory (Join-Path $skillTemplateRoot 'ad-creative-prime-compose') -Config @{ kind = 'creative_role'; capability = 'prime_compose'; version = 2 }
$qcSkill = Set-WorkspaceSkill -Name '广告成图验收' -Description '按创意核验三尺寸成图、文案、二维码、贴图资产和合规风险。' -Directory (Join-Path $skillTemplateRoot 'ad-creative-qc') -Config @{ kind = 'creative_role'; capability = 'quality_control'; version = 2 }

$agents = Get-Items (Invoke-MulticaApi -Method Get -Path '/api/agents') ''
$leaderSeed = $agents | Where-Object name -eq '素材小队 Leader' | Select-Object -First 1
if (-not $leaderSeed) { throw '素材小队 Leader does not exist' }
$runtimeID = $leaderSeed.runtime_id

$collector = Set-AgentDefinition -Name 'AppGrowing 素材采集智能体' -Description '逐竞品多页采集并将真实结果导入父 Issue 候选池。' -Instructions '全程使用中文。只执行当前采集子 Issue 指定的真实 AppGrowing 查询。逐家核对普通竞品至少 3 页、优先竞品至少 5 页；单家接口为空或失败时对该家启用 Playwright 补查。结果只进入父 Issue 原生候选池，逐页证据与失败原因留在当前子 Issue。' -SkillIDs @($collectorSkill.id) -RuntimeID $runtimeID -MaxConcurrentTasks 2
$specialistHandoff = '完成后把全部过程、附件和证据留在当前专业子 Issue并将其置为 done。平台会自动向直接父 Issue 发送最小完成回执并唤醒小队；不要手工评论父 Issue，也不要 @Leader，以免重复触发。'
$analyst = Set-AgentDefinition -Name '广告参考分析智能体' -Description '逐图读取真实像素，识别主题、利益点、文字证据和构图风险。' -Instructions "全程使用中文。只分析当前专业子 Issue 指定的候选图和市场资源快照。必须读取真实图片像素，分别识别视觉主题与金融主利益点，把结构化创意简报回写目标候选池；采集标题、标签和媒体只能弱辅助。详细证据留在当前子 Issue，不生成图片。$specialistHandoff" -SkillIDs @($analysisSkill.id) -RuntimeID $runtimeID -MaxConcurrentTasks 4
$planner = Set-AgentDefinition -Name '生成方案智能体' -Description '把逐图输入快照转成 3 个创意变体和三尺寸生成规格。' -Instructions "全程使用中文。读取当前专业子 Issue 引用的参考分析、逐图文案和市场资源快照，输出 V01、V02、V03 三个差异明确的创意母版规格，以及每个母版的横版和竖版原生重排约束。只在当前子 Issue 讨论提示词和取舍，不生成图片。$specialistHandoff" -SkillIDs @($planSkill.id) -RuntimeID $runtimeID -MaxConcurrentTasks 3
$producer = Set-AgentDefinition -Name '图像编辑智能体' -Description '最多 5 路并发执行创意母版或单个尺寸的原生图像编辑。' -Instructions "全程使用中文。只执行当前专业子 Issue 引用的一个创意母版或一个尺寸重排规格，使用平台 image edit 能力生成满版底图，回传来源、变体编号、提示词、模型、请求 ID、尝试次数和附件。V01-V03 的方形母版相互独立；横版和竖版必须把已通过的对应母版作为第一参考原生重排。不得裁切、加边、拉伸，不添加二维码或合规贴图。候选图含竞品 App UI 时，必须同时输入分析成员选中的 AdaKami App UI 参考图并完成替换。$specialistHandoff" -SkillIDs @($productionSkill.id) -RuntimeID $runtimeID -MaxConcurrentTasks 5
$composer = Set-AgentDefinition -Name 'Prime 包装智能体' -Description '按单个创意批量完成三个尺寸的完整贴图和二维码校验。' -Instructions "全程使用中文。一个专业子 Issue 处理一个创意变体的三个尺寸。只读取当前专业子 Issue 引用的版本化资源附件，使用对应尺寸完整 Prime 贴图和 Skill 自带确定性合成脚本完成包装。不得使用本机固定素材路径。三个最终附件和二维码机器校验证据只留在当前子 Issue。$specialistHandoff" -SkillIDs @($composeSkill.id) -RuntimeID $runtimeID -MaxConcurrentTasks 4
$reviewer = Set-AgentDefinition -Name '广告验收智能体' -Description '按单个创意验收三个尺寸最终成图并给出通过或返工。' -Instructions "全程使用中文。一个专业子 Issue 验收一个创意变体的三个尺寸最终成图与快照证据。逐项检查尺寸、满版构图、批准文案、完整 Prime 资产、二维码解码和合规风险；失败时明确指出该变体中仅需重开的受影响尺寸。$specialistHandoff" -SkillIDs @($qcSkill.id) -RuntimeID $runtimeID -MaxConcurrentTasks 3

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
    instructions = 'Leader 读取当前 Issue 证据以及名册中每个智能体的职责和平台 Skill，动态选择合适成员。父 Issue 只保留候选池、结果看板和必须让用户看到的结论；每张选图一个创意工作 Issue。每个专业任务必须新建并分配直接子 Issue，禁止在创意工作 Issue 内用 @mention 委派成员；专业讨论和附件证据都留在专业子 Issue。'
} | Out-Null

$memberDefinitions = @(
    @{ agent = $collector; role = '素材采集' },
    @{ agent = $analyst; role = '参考分析' },
    @{ agent = $planner; role = '生成方案' },
    @{ agent = $producer; role = '图像编辑' },
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
$leaderSkill = Set-WorkspaceSkill -Name '创意素材协作' -Aliases @('素材小队 Leader 编排') -Description '帮助素材小队 Leader 批量委派 3 个创意变体的母版、尺寸重排、包装、验收和发布。' -Directory $leaderSkillDirectory -Config @{ kind = 'creative_role'; capability = 'creative_leadership'; version = 7 }
$leader = Set-AgentDefinition -Name '素材小队 Leader' -Description '管理候选池素材理解、3×3 创意委派、返工和最终结果发布。' -Instructions '全程使用中文。你只负责判断、批量委派、验收汇总和发布。每次读取小队名册中成员的职责和 Skill，再按当前证据挑选合适成员。候选池发起的素材理解请求只委派参考分析成员读取真实像素并回写结构化创意简报，不启动成图链路。父 Issue 对用户只展示候选池、结果看板和必要结论；每张选中素材创建一个独立创意工作 Issue，每个工作 Issue 必须交付 V01、V02、V03 三个创意变体，每个变体原生交付 1080x1080、1200x628、800x1000。每次唤醒先读取全部直接子 Issue，再一次创建所有已经满足依赖且尚不存在的专业子 Issue；禁止一次只派一个可并行任务。先并发三个方形母版，某个母版通过后立即并发该变体的横版和竖版；一个 Prime 子 Issue 批量包装一个变体的三个尺寸，一个 QC 子 Issue批量验收一个变体的三个尺寸。所有角色对话、提示词、模型证据和返工都留在专业子 Issue。创建前按候选 ID、修订、V01-V03、尺寸和能力检查已有子 Issue，禁止重复创建。只有三个变体各自三尺寸全部通过才能发布 9 张图；文件名必须包含 V01-V03，发布目标必须是快照中的 parent_issue_id。调整只影响目标变体和尺寸，其他通过结果沿用。发布确认后将当前创意工作 Issue 置为 done；最外层候选池 Issue 不自动关闭。' -SkillIDs @($leaderSkill.id) -RuntimeID $runtimeID -MaxConcurrentTasks 5

$resources = Get-Items (Invoke-MulticaApi -Method Get -Path '/api/creative/resources') 'resources'
$copyLibrary = $resources | Where-Object { $_.kind -eq 'copy_library' -and $_.name -eq 'AdaKami Indonesia 文案库' } | Select-Object -First 1
if (-not $copyLibrary) {
    $copyLibrary = Invoke-MulticaApi -Method Post -Path '/api/creative/resources' -Body @{
        kind = 'copy_library'
        name = 'AdaKami Indonesia 文案库'
        description = '来自 NEW SCRIPT DESIGN CONTENT - 2026.xlsx；平台内可筛选、编辑、审核和重新导入。'
        config = @{ market = 'Indonesia'; locale = 'id-ID'; source_filename = 'NEW SCRIPT DESIGN CONTENT - 2026.xlsx' }
    }
}

$catalog = Get-Content -Raw -LiteralPath $CatalogPath | ConvertFrom-Json -Depth 100
$copyEntries = foreach ($record in @($catalog.records)) {
    $lines = @([regex]::Split([string]$record.copy_text, '\r?\n') | ForEach-Object { $_.Trim() } | Where-Object { $_ })
    $headline = if ($lines.Count) { $lines[0] } else { [string]$record.copy_text }
    $benefit = if ($lines.Count -gt 1) { ($lines | Select-Object -Skip 1) -join "`n" } else { '' }
    $approved = $record.eligible_for_static_image -and $record.selectable_for_generation -and $record.ready_without_inputs
    $tags = @($record.month, $record.content_keyword, $record.suggested_copy_type, $record.set_label) | Where-Object { $_ } | Select-Object -Unique
    @{
        external_key = [string]$record.record_id
        headline = $headline
        subheadline = ''
        benefit = $benefit
        cta = ''
        legal_text = ''
        copy_role = [string]$record.content_type
        market = 'Indonesia'
        locale = 'id-ID'
        tags = @($tags)
        status = if ($approved) { 'approved' } else { 'draft' }
        metadata = @{
            source = $record.source
            original_copy = [string]$record.copy_text
            content_keyword = [string]$record.content_keyword
            suggested_copy_type = [string]$record.suggested_copy_type
            required_inputs = @($record.required_inputs)
            notes = $record.notes
            references = @($record.references)
            eligible_for_static_image = [bool]$record.eligible_for_static_image
            selectable_for_generation = [bool]$record.selectable_for_generation
            ready_without_inputs = [bool]$record.ready_without_inputs
        }
    }
}
$copyImport = Invoke-MulticaApi -Method Post -Path "/api/creative/copy-libraries/$($copyLibrary.id)/entries/import" -Body @{
    mode = 'upsert'
    source_filename = 'NEW SCRIPT DESIGN CONTENT - 2026.xlsx'
    mapping = @{
        adapter = 'profile_copy_catalog_v1'
        records = @($copyEntries).Count
        source_sha256 = [string]$catalog.source.sha256
        source_path = [string]$catalog.source.file_path
    }
    entries = @($copyEntries)
}
$copyLibrary = Invoke-MulticaApi -Method Post -Path "/api/creative/resources/$($copyLibrary.id)/publish"

$resources = Get-Items (Invoke-MulticaApi -Method Get -Path '/api/creative/resources') 'resources'
$marketPack = $resources | Where-Object { $_.kind -eq 'market_pack' -and $_.name -eq 'AdaKami Indonesia 市场资源包' } | Select-Object -First 1
$marketConfig = @{
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
    qr_approval_note = '三个原始 Prime 模板与旧 main 配置均机器解码为该条款地址。'
    compliance_rules = '只能使用文案库已审核的金融事实；不得复制竞品品牌、金额、法律文字或二维码。完整 Prime 资产必须原样保留，最终二维码必须机器可解码。'
    naming_rule = '{month}_{brand}_{market}_{candidate}_{size}_v{revision}.png'
    output_sizes = @('1080x1080','1200x628','800x1000')
    composition = 'full_prime_overlay'
}
if (-not $marketPack) {
    $marketPack = Invoke-MulticaApi -Method Post -Path '/api/creative/resources' -Body @{
        kind = 'market_pack'
        name = 'AdaKami Indonesia 市场资源包'
        description = '印尼市场可编辑配置与版本化附件；新市场可在平台复制后替换文案库、规则和资源槽位。'
        config = $marketConfig
    }
}

Add-MarketFile -MarketPack $marketPack -Role 'prime_square' -Label 'Prime 1080x1080' -Path (Join-Path $PrimeDirectory '11-01.png')
Add-MarketFile -MarketPack $marketPack -Role 'prime_landscape' -Label 'Prime 1200x628' -Path (Join-Path $PrimeDirectory '191-01.png')
Add-MarketFile -MarketPack $marketPack -Role 'prime_portrait' -Label 'Prime 800x1000' -Path (Join-Path $PrimeDirectory '45-01.png')
Add-MarketFile -MarketPack $marketPack -Role 'brand_guideline' -Label 'AdaKami 印尼成图规则' -Path $BrandGuidelinePath
$marketFiles = Get-Items (Invoke-MulticaApi -Method Get -Path "/api/creative/resources/$($marketPack.id)/files") 'files'
foreach ($obsoleteFile in @($marketFiles | Where-Object role -eq 'compose_script')) {
    Invoke-MulticaApi -Method Delete -Path "/api/creative/resources/$($marketPack.id)/files/$($obsoleteFile.id)" | Out-Null
}
foreach ($appUIPath in $AppUIReferencePaths) {
    Add-MarketFile -MarketPack $marketPack -Role 'app_ui_reference' -Label "AdaKami App UI - $([IO.Path]::GetFileNameWithoutExtension($appUIPath))" -Path $appUIPath -Multiple -Metadata @{
        dimensions = '1080x2160'
        market = 'Indonesia'
        locale = 'id-ID'
        tags = @('app-ui', 'homepage', 'new-customer', 'loan-limit')
        description = '当竞品素材包含 App 界面时，分析智能体从全部 App UI 参考图中判断并选择最匹配的一张；图像编辑必须替换为 AdaKami 自有界面，不得保留或仿造竞品 UI。'
    }
}
$marketPack = Invoke-MulticaApi -Method Put -Path "/api/creative/resources/$($marketPack.id)" -Body @{
    name = 'AdaKami Indonesia 市场资源包'
    description = '印尼市场可编辑配置与版本化附件；新市场可在平台复制后替换文案库、规则和资源槽位。'
    config = $marketConfig
}
$marketPack = Invoke-MulticaApi -Method Post -Path "/api/creative/resources/$($marketPack.id)/publish"

$autopilots = Get-Items (Invoke-MulticaApi -Method Get -Path '/api/autopilots') 'autopilots'
$autopilot = $autopilots | Where-Object { $_.title -eq '印尼竞品素材周度采集' -or $_.title -eq '印尼与马来竞品素材周度采集' } | Select-Object -First 1
$autopilotDescription = @'
每周抓取 AppGrowing 印度尼西亚的现金贷/金融竞品素材，并创建一个创意批次父 Issue。

竞品：Easycash、Kredit Pintar、Adapundi、BantuSaku、Rupiah Cepat、UATAS、JULO
优先竞品：Easycash、Kredit Pintar、Adapundi
地区：印度尼西亚
语言：印度尼西亚语
设备：Android、iOS
媒体：未限定
时间范围：最近 30 天
选材：新素材 40%，投放少于 7 天且曝光估算大于 1K；跑量素材 60%，投放超过 30 天且曝光估算不低于 10M
最多输出：25 条

先委派素材采集智能体执行真实 AppGrowing 多页采集。素材必须进入父 Issue 原生候选池；完成后等待用户逐图选择文案并开始修图。授权失效时提示用户前往“设置 - 集成”重新绑定，不能用测试数据替代。
'@
if (-not $autopilot) {
    $autopilot = Invoke-MulticaApi -Method Post -Path '/api/autopilots' -Body @{
        title = '印尼竞品素材周度采集'
        description = $autopilotDescription
        assignee_type = 'squad'
        assignee_id = $squad.id
        execution_mode = 'create_issue'
        issue_title_template = '{{date}} 竞品素材抓取'
    }
} else {
    $autopilot = Invoke-MulticaApi -Method Patch -Path "/api/autopilots/$($autopilot.id)" -Body @{
        title = '印尼竞品素材周度采集'
        description = $autopilotDescription
        assignee_type = 'squad'
        assignee_id = $squad.id
        execution_mode = 'create_issue'
        issue_title_template = '{{date}} 竞品素材抓取'
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
    CopyEntries = @($copyEntries).Count
    MarketPackID = $marketPack.id
    SquadID = $squad.id
    LeaderSkillID = $leaderSkill.id
    AutopilotID = $autopilot.id
}
