package handler

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

const creativeFactoryTemplateVersion = 22

//go:embed creative_factory_defaults/resources.json
var creativeFactoryDefaultResourcesJSON []byte

//go:embed creative_factory_defaults/assets/*
var creativeFactoryDefaultAssets embed.FS

const (
	creativeFactoryDefaultAutopilotTitle       = "印尼竞品素材周度采集"
	creativeFactoryLegacyAutopilotTitle        = "创意工厂自动化"
	creativeFactoryDefaultCollectionTargetLine = "最多输出：5 张图片"
	creativeFactoryDefaultAutopilotDescription = `每周抓取 AppGrowing 印度尼西亚的现金贷/金融竞品素材，并创建一个可追踪的 Crawl Run。

工作区创意工厂安装记录负责解析市场资源包、执行小队和参考分析智能体；不要在 AutoPilot 说明中保存 UUID。

竞品：Easycash、Kredit Pintar、Adapundi、BantuSaku、Rupiah Cepat、UATAS、JULO
优先竞品：Easycash、Kredit Pintar、Adapundi
地区：印度尼西亚
语言：印度尼西亚语
设备：Android、iOS
媒体：未限定
时间范围：最近 30 天
选材：新素材 40%，投放少于 7 天且曝光估算大于 1K；跑量素材 60%，投放超过 30 天且曝光估算不低于 10M
` + creativeFactoryDefaultCollectionTargetLine + `
素材类型：仅图片广告（asset_type=image）。视频、非图片和无法识别类型必须在选材前排除，不占用名额，也不进入 Crawl Run 或素材库。

执行真实 AppGrowing 多页图片采集，结果进入创意工厂素材库并关联当前 Crawl Run；图片入库后立即用原生 task fanout 对本次新增图片并发执行逐图创意分析，归档未完成时允许分析读取真实源图片。分析完成后在创意工厂提醒用户选图、确认主题与文案。整个采集与预分析阶段不创建 Issue。授权失效时将 Crawl Run 标为 action_required，并提示用户前往“设置 - 集成”重新绑定，不能用测试数据替代。`
)

type creativeFactorySkillSpec struct {
	Role        string
	Name        string
	Aliases     []string
	Directory   string
	Description string
	Capability  string
	Version     int
}

type creativeFactoryAgentSpec struct {
	Role          string
	Name          string
	Aliases       []string
	Description   string
	Instructions  string
	SkillRoles    []string
	Model         string
	ThinkingLevel string
	MaxConcurrent int32
}

type creativeFactoryTemplate struct {
	Content string
	Files   []CreateSkillFileRequest
}

type creativeFactoryResourceDefault struct {
	Description string                            `json:"description"`
	Config      map[string]any                    `json:"config"`
	Files       []creativeFactoryResourceFileSeed `json:"files,omitempty"`
}

type creativeFactoryResourceFileSeed struct {
	Role      string          `json:"role"`
	Label     string          `json:"label"`
	Metadata  json.RawMessage `json:"metadata"`
	AssetPath string          `json:"asset_path"`
}

type creativeFactoryInstallationRecord struct {
	WorkspaceID          pgtype.UUID
	Status               string
	SchemaVersion        int
	TemplateVersion      int
	RuntimeID            pgtype.UUID
	MarketPackID         pgtype.UUID
	CopyLibraryID        pgtype.UUID
	SquadID              pgtype.UUID
	OrchestrationSkillID pgtype.UUID
	RoleAgents           map[string]string
	RoleSkills           map[string]string
	Config               map[string]any
}

type creativeFactoryInitializationInput struct {
	Brand               string   `json:"brand"`
	Market              string   `json:"market"`
	Locale              string   `json:"locale"`
	Currency            string   `json:"currency"`
	Competitors         []string `json:"competitors"`
	PriorityCompetitors []string `json:"priority_competitors"`
}

type creativeFactoryMarketProfile struct {
	Explicit            bool
	Brand               string
	Market              string
	MarketLabel         string
	Locale              string
	Currency            string
	CurrencyPrefix      string
	LanguageLabel       string
	MarketAbbreviation  string
	Competitors         []string
	PriorityCompetitors []string
}

const creativeFactoryDirectEditAgentInstructions = `creative_direct_edit 分支严格执行绑定的素材_技能_改图；按 Variant 冻结的 prime_composition 编辑 task context 指向的 source asset，保留批准文案、附件血缘、归一化、完整模型回执、过程图登记和多目标同时验收，再调用绑定的贴片 Skill 并进入最终视觉 QC。run_image_edit_job.py 只可 start 一次（每个 provider operation attempt）；返回 running 或 waiting 时在同一 task 反复 wait，只报告进度，不输出最终答复、不调用 task complete、不更新 operation，直到 completed 且资产已登记。runtime 提前终止时依赖 late receipt recovery，禁止重复模型调用。唯一例外是 task context.provider_receipt_reconciliation 明确为 provider_receipt_not_found 且 allow_fresh_provider_attempt=true：平台已核验旧 attempt 没有 provider request、回执或输出；这是一次新的 provider attempt，先按 Skill 登记 operation 的 next attempt，再且只再 start 一次，不伪造旧回执或 canonical 资产。绑定 Skill 是直接改图合同的唯一执行真值，不从 Agent instructions 或历史 prompt 补另一套协议。`
const creativeFactorySpecialistHandoff = `只处理 task context 指定的对象、revision 和 scope；按绑定 Skill 写结构化领域结果和机器证据。图像任务按绑定 Skill 的统一比例阈值处理：阈值内归一，10%-25% canvas repair，只有超过 25% 才重生当前尺寸；不能把像素绝对尺寸差当成模型失败。不得创建或修改 Issue，不得用评论代替领域数据。输入、凭证、工具或写回失败时保留已成功对象，写真实 error_code/error_message 并让当前 task 失败；兄弟对象继续。`

func creativeFactoryImageEditAgentInstructions() string {
	return fmt.Sprintf("创意工厂出图能力池合同版本：%d。\n\n", creativeFactoryTemplateVersion) + `全程使用中文。根据 task context.workflow 选择唯一分支，并在执行前完整读取对应绑定 Skill；Skill 及其 references 是提示词、证据、归一化、Prime 和恢复规则的唯一执行真值。

creative_production 只执行绑定的素材_技能_出图：候选阶段全部生成 1080x1080 方图作公平比较；selected 阶段按订单当前 candidate_state、primary_size、production_stage 和 expected_sizes 补齐原生横竖尺寸，复用已有主图和成功回执，按 CreativeIntent、DesignDNA、LayoutPlan 独立生成尺寸，不把候选方图 raster 当硬依赖。候选源图、Prime 模板和已选 App UI 只使用订单冻结的 attachment_snapshot；附件失败按 auth_expired、attachment_not_found、storage_timeout 或 cli_contract_mismatch 记录，不猜 URL 或复用旧文件。严格按 Variant 冻结的 prime_composition 选择无品牌底图或 QR-free 模板融入，不按市场名猜模式；只在当前阶段尺寸与过程证据齐全后调用绑定的贴片 Skill。

` + creativeFactoryDirectEditAgentInstructions + `

两个分支都只处理 task context 指定对象和平台锁定 revision，不创建或修改 Issue，不跨 workflow。模型 prompt 只包含改变像素的视觉指令；task、revision、文件、哈希、上传、登记、重试和状态说明留在模型调用外。协议或登记失败复用已有回图与证据；只有没有有效回图或真实视觉失败才按 Skill 的有界规则再次调用模型。

` + creativeFactorySpecialistHandoff
}

var creativeFactorySkillSpecs = []creativeFactorySkillSpec{
	{Role: "collection", Name: "素材_技能_采集", Aliases: []string{"AppGrowing 素材采集"}, Directory: "appgrowing-material-collector", Description: "创建 Crawl Run，只采集真实图片广告，并用原生 task fanout 自动预分析新增图片。", Capability: "material_collection", Version: 17},
	{Role: "diagnostics", Name: "素材_技能_诊断", Aliases: []string{"创意流程诊断", "出图诊断", "AppGrowing 采集诊断"}, Directory: "creative-flow-diagnostician", Description: "读取创意采集、候选、尺寸调用、版本、Prime、QC 和 daemon/runtime 证据，只通过平台领域入口恢复或给出明确动作。", Capability: "crawl_diagnosis", Version: 7},
	{Role: "reference_analysis", Name: "素材_技能_分析", Aliases: []string{"广告参考分析"}, Directory: "ad-creative-analysis", Description: "市场中立地读取真实图片，识别可变视觉区域、原图文字及坐标、主题、利益点、语义锚点、App UI 类型、屏幕边界和布局约束；App UI 只做通用检测，不选择品牌附件；只有明显的还款结构才锁定为 numeric，单独金额或核心利益点不得因为带数字就被卡死。", Capability: "reference_analysis", Version: 19},
	{Role: "pre_adaptation", Name: "素材_技能_文案适配", Aliases: []string{"广告预适配"}, Directory: "ad-creative-pre-adaptation", Description: "按冻结资源完成可生产文案与数值适配；只有明显的还款结构才生成 repayment 选择和 numeric layout，单独金额、核心利益点或促销额度默认保留为可编辑文案，保留后续可手动改写空间；数值布局说明必须列出每个冻结展示值。", Capability: "pre_adaptation", Version: 26},
	{Role: "generation_plan", Name: "素材_技能_方案", Aliases: []string{"广告生成方案"}, Directory: "ad-creative-plan", Description: "消费冻结分析、文案与市场快照，规划 4-5 个统一方形首轮候选及三尺寸 LayoutPlan；只有用户明确选择时才替换 App UI，非空核心利益点必须作为可见文字，独立质检原子晋级 3 个。", Capability: "generation_plan", Version: 40},
	{Role: "image_edit", Name: "素材_技能_出图", Aliases: []string{"广告图像编辑"}, Directory: "ad-creative-production", Description: "按唯一模型提示词合同生成统一方形候选主视觉或 selected 缺失尺寸；三尺寸共享 DesignDNA、文案与血缘但从冻结附件和各自 LayoutPlan 独立生成，附件失败结构化分类，按冻结 Prime 模式仅在明确选择时替换 App UI，并把批准利益点渲染为可见文字。", Capability: "image_edit", Version: 111},
	{Role: "prime_compose", Name: "素材_技能_贴片", Aliases: []string{"广告品牌组件合成"}, Directory: "ad-creative-prime-compose", Description: "调用后端唯一的 Prime 交接入口：默认确定性合成，或登记冻结的无二维码模型融入结果；校验 JSON，并由后端登记过程图、primed 资产和标准 QC/交付交接；不创建 Prime Agent 或 Prime task。", Capability: "prime_compose", Version: 6},
	{Role: "direct_image_edit", Name: "素材_技能_改图", Aliases: []string{"广告图片直接修改"}, Directory: "ad-creative-direct-edit", Description: "将用户反馈编译为带输入角色、锁定/可编辑集合和 target masks 的多目标调整；布局微调保留原图主体占比和适度留白，不把模糊的空间要求放大为大面积空白；按冻结 Prime 模式完成终检。", Capability: "direct_image_edit", Version: 32},
	{Role: "quality_control", Name: "素材_技能_质检", Aliases: []string{"广告成图验收"}, Directory: "ad-creative-qc", Description: "独立比较 4-5 个候选主尺寸并原子晋级 3 个，或对标准/精准改图的实际交付尺寸执行 Prime 与 DesignDNA 联合视觉终检；附件失败结构化分类，qc-finalize 瞬态失败由服务端持久化恢复。", Capability: "quality_control", Version: 43},
	{Role: "creative_leadership", Name: "素材_技能_流程", Aliases: []string{"素材_技能_统筹", "创意素材协作", "素材小队 Leader 编排"}, Directory: "ad-creative-leadership", Description: "使用原生 task fanout 启动并恢复候选生产、晋级扩尺寸、直接改图和最终验收，汇总结构化结果。", Capability: "creative_leadership", Version: 51},
}

var creativeFactoryAgentSpecs = []creativeFactoryAgentSpec{
	{Role: "leadership", Name: "素材_流程", Aliases: []string{"素材_统筹", "素材小队 Leader"}, Description: "按冻结能力映射启动和恢复 Creative Order，并负责用户汇总。", Instructions: "全程使用中文。只执行判断、原生 fanout、异常恢复和用户汇总，不代替专业角色。标准订单只创建方案 task；Planner 建候选并委派主尺寸，候选晋级与 selected 扩尺寸由阶段 owner 和平台续链；初始 direct_edit 由平台原子创建 revision 和 task，Leader 只恢复领域状态确认缺失的当前 revision task。每次唤醒回读订单、task 与冻结 squad snapshot，按 target/source/item_key 只补真正缺失项。严格执行绑定流程 Skill，不按名称猜 Agent，不轮询，不创建阶段子 Issue。", SkillRoles: []string{"creative_leadership"}, Model: "gpt-5.6-terra", ThinkingLevel: "medium", MaxConcurrent: 6},
	{Role: "reference_analysis", Name: "素材_分析", Aliases: []string{"广告参考分析智能体"}, Description: "按 task workflow 读取真实像素、识别可变视觉区域、写市场中立分析，并在后台完成可确认的文案与数值预适配。", Instructions: "全程使用中文。creative_reference_analysis 只写指定 candidate/version 的市场中立 Source Analysis；每个可变原图文字区块必须归入唯一 copy 或 numeric 视觉区域，不能把同一画面组件拆入两条处理路径；App UI 只输出通用页面类型、屏幕边界和 replacement_needed，不读取、选择或引用 app_ui_reference 附件。creative_pre_adaptation 只消费指定的冻结市场包和文案库，逐区域优先绑定已审核内容；没有兼容已审核片段的普通 headline、subheadline、benefit、supporting 或 cta 区块，必须按市场语言、区块职责、原图语义和可读长度生成一个新的 recommended 文案，source_keys 必须为空数组，recommendation_basis 必须说明依据，页面会标记待用户确认；不得引用不存在的 fragment key。本金、期限、月供、总利息、总还款、利率、法律文字和品牌事实没有审核来源或冻结计算时不得凭空生成，逐项写 missing replacement。多行数值表不要求原图行数与我方方案数量相等：按表格语义和期限从冻结 approved repayment plan 取我方兼容方案，实际渲染行数取原图可渲染行数与我方可用方案数的较小值；每个实际渲染行写 numeric_layouts，且每个 layout 的 render_instruction 必须逐字列出其 scenario_ids 对应 selection.values 中每个 target_columns 的完整冻结展示值，不能只写按行展示；源图多出的数值块逐项写空 missing 作为默认移除项。没有我方某一期限方案时不得借用其他期限金额；整体重构为我方支持的期限列，或让该列源块留空移除。只有明显的还款结构才锁定，单独金额、核心利益点或促销额度不得因为带数字就卡死成还款计划。不能把整表硬塞进一个 layout，也不能伪装成已绑定或改写原图事实。输入和产物不得混用，不生成图片，不修改市场包或文案库。只处理 task context 指定的对象、revision 和 scope；按绑定 Skill 写结构化领域结果和机器证据。不得创建或修改 Issue，不得用评论代替领域数据。输入、凭证、工具或写回失败时保留已成功对象，写真实 error_code/error_message 并让当前 task 失败；兄弟对象继续。", SkillRoles: []string{"reference_analysis", "pre_adaptation"}, Model: "gpt-5.6-terra", ThinkingLevel: "medium", MaxConcurrent: 6},
	{Role: "collection", Name: "素材_采集", Aliases: []string{"AppGrowing 素材采集智能体"}, Description: "按 task 配置创建 Crawl Run，只导入真实图片广告并委派新增图片分析。", Instructions: "全程使用中文。只执行 task context 和 AutoPilot 明确的 AppGrowing 查询；只导入 asset_type=image，视频、非图片和未知类型不占用名额。使用注入的 analysis_agent_id=%s，不得按名称猜测。筛选、分页、预算和 fallback 由 task/平台配置决定。结果、证据和失败写 Crawl Run；导入后用原生 fanout 委派本次新增图片，不创建 Issue，不使用测试数据。", SkillRoles: []string{"collection"}, Model: "gpt-5.6-luna", ThinkingLevel: "low", MaxConcurrent: 1},
	{Role: "generation_plan", Name: "素材_方案", Aliases: []string{"生成方案智能体"}, Description: "消费冻结输入，为标准订单建立 4-5 个主视觉候选。", Instructions: "全程使用中文。只执行 creative_plan，完整执行绑定的素材_技能_方案；该 Skill 及 Creative Intent reference 是候选数量、主尺寸、CreativeIntent、DesignDNA、LayoutPlan、App UI、金融文案和生产 fanout 的唯一真值。只接受平台冻结的 candidate_v1，写 4-5 个 candidate Variant，所有候选首轮固定 `1080x1080` 并委派方形主视觉；不预选 3 个、不生成图片，selected 后再按各尺寸原生重排且不把候选方图当母版。只有页面冻结的明确选择才使用 App UI，非空核心利益点必须列为可见文字。流程版本缺失或不符时写真实错误，不推断或切换流程。只处理 task context 指定对象，失败保留已写候选和真实错误。", SkillRoles: []string{"generation_plan"}, Model: "gpt-5.6-terra", ThinkingLevel: "medium", MaxConcurrent: 6},
	{Role: "image_edit", Name: "素材_出图", Aliases: []string{"图像编辑智能体"}, Description: "按唯一提示词合同执行候选主视觉、selected 扩尺寸或用户标注精准改图；只用订单冻结附件，结构化记录下载失败，再由贴片 Skill 交接终检。", Instructions: creativeFactoryImageEditAgentInstructions(), SkillRoles: []string{"image_edit", "direct_image_edit", "prime_compose"}, Model: "gpt-5.6-terra", ThinkingLevel: "low", MaxConcurrent: 10},
	{Role: "quality_control", Name: "素材_质检", Aliases: []string{"广告验收智能体"}, Description: "独立晋级候选主视觉，或联合验收当前实际交付尺寸的 Prime 成图。", Instructions: "全程使用中文。根据 task context.workflow 只执行 creative_candidate_selection 或 creative_qc_visual，并完整执行绑定的素材_技能_质检；Skill 是评分、candidate-select、结构化 Prime 极性、实际交付尺寸联合验收、多目标检查和 qc-finalize 的唯一真值。候选分支恰选 3 个并使用原子 candidate-select，回读平台自动排队结果，不自行重复 fanout；终检分支不得硬编码背景极性、逐图自报通过或使用旧 revision 资产，单尺寸精准改图也必须完成最终视觉质检。附件下载失败按 Skill 写精确错误码；qc-finalize 瞬态失败保留报告并结束当前任务，由服务端复用 Prime 资产恢复。", SkillRoles: []string{"quality_control"}, Model: "gpt-5.6-terra", ThinkingLevel: "medium", MaxConcurrent: 6},
	{Role: "diagnostics", Name: "素材_诊断", Aliases: []string{"创意流程诊断智能体", "出图诊断智能体", "AppGrowing 采集诊断智能体"}, Description: "诊断创意采集、候选、尺寸调用、版本、Prime、QC 和 daemon/runtime 异常，并通过平台入口执行受控恢复。", Instructions: "全程使用中文。处理 creative_crawl_diagnosis、订单短 ID、Variant 标签、页面卡片文案、报错文本和用户明确指向的创意流程诊断。先定位当前订单、order item、候选状态、active/staging revision、尺寸 operation、task、daemon/runtime 与 Skill 快照证据，再给结论；需要恢复时只通过 multica CLI 或平台 API 的重试、取消、fanout 或现有领域修复入口，revision 只能由平台事务创建，不得用 variant-put 自行推进。具备 crawl_diagnosis 能力的运行诊断任务可对同工作区、已验证的创意对象使用受审计的高额度恢复预算；不得跨工作区、直接写 DB、修改凭证、业务筛选、市场包或生产代码。只有用户明确要求维护文案库时，才可先回读文案库并使用 multica creative copy-library 的 add-fragment、update-fragment、upsert-repayment-plan 保存草稿；只有用户明确要求发布时才加 --publish。不得把诊断图当成交付资产；修改前说明对象和原因，修改后回读验证。", SkillRoles: []string{"diagnostics"}, Model: "gpt-5.6-luna", ThinkingLevel: "medium", MaxConcurrent: 2},
}

func creativeFactoryMarketProfileFromInput(input *creativeFactoryInitializationInput) creativeFactoryMarketProfile {
	profile := creativeFactoryIndonesiaProfile()
	if input == nil {
		return profile
	}
	profile.Explicit = true
	switch normalized := strings.ToLower(strings.TrimSpace(input.Market)); {
	case normalized == "", normalized == "indonesia", normalized == "id", strings.Contains(normalized, "印尼"), strings.Contains(normalized, "印度尼西亚"):
		profile = creativeFactoryIndonesiaProfile()
		profile.Explicit = true
	case normalized == "malaysia", normalized == "my", strings.Contains(normalized, "马来"):
		profile = creativeFactoryMalaysiaProfile()
		profile.Explicit = true
	default:
		profile = creativeFactoryGenericProfile(strings.TrimSpace(input.Market))
	}
	if brand := strings.TrimSpace(input.Brand); brand != "" {
		profile.Brand = brand
	}
	if locale := strings.TrimSpace(input.Locale); locale != "" {
		profile.Locale = locale
	}
	if currency := strings.ToUpper(strings.TrimSpace(input.Currency)); currency != "" {
		profile.Currency = currency
	}
	if competitors := cleanCreativeFactoryList(input.Competitors); len(competitors) > 0 {
		profile.Competitors = competitors
	}
	if priority := cleanCreativeFactoryList(input.PriorityCompetitors); len(priority) > 0 {
		profile.PriorityCompetitors = priority
	} else if len(profile.PriorityCompetitors) == 0 && len(profile.Competitors) > 0 {
		limit := 3
		if len(profile.Competitors) < limit {
			limit = len(profile.Competitors)
		}
		profile.PriorityCompetitors = append([]string{}, profile.Competitors[:limit]...)
	}
	return profile
}

func creativeFactoryIndonesiaProfile() creativeFactoryMarketProfile {
	return creativeFactoryMarketProfile{
		Brand:              "AdaKami",
		Market:             "Indonesia",
		MarketLabel:        "印度尼西亚",
		Locale:             "id-ID",
		Currency:           "IDR",
		CurrencyPrefix:     "Rp",
		LanguageLabel:      "印度尼西亚语",
		MarketAbbreviation: "ID",
		Competitors: []string{
			"Easycash", "Kredit Pintar", "Adapundi", "BantuSaku", "Rupiah Cepat", "UATAS", "JULO",
		},
		PriorityCompetitors: []string{"Easycash", "Kredit Pintar", "Adapundi"},
	}
}

func creativeFactoryMalaysiaProfile() creativeFactoryMarketProfile {
	return creativeFactoryMarketProfile{
		Brand:              "AdaKami",
		Market:             "Malaysia",
		MarketLabel:        "马来西亚",
		Locale:             "ms-MY",
		Currency:           "MYR",
		CurrencyPrefix:     "RM",
		LanguageLabel:      "马来语",
		MarketAbbreviation: "MY",
	}
}

func creativeFactoryGenericProfile(market string) creativeFactoryMarketProfile {
	if strings.TrimSpace(market) == "" {
		market = "Market"
	}
	return creativeFactoryMarketProfile{
		Brand:              "AdaKami",
		Market:             market,
		MarketLabel:        market,
		Locale:             "en-US",
		Currency:           "USD",
		CurrencyPrefix:     "$",
		LanguageLabel:      "英语",
		MarketAbbreviation: strings.ToUpper(marketAbbreviationFromName(market)),
	}
}

func marketAbbreviationFromName(market string) string {
	trimmed := strings.TrimSpace(market)
	if trimmed == "" {
		return "MK"
	}
	letters := []rune{}
	for _, char := range trimmed {
		if (char >= 'A' && char <= 'Z') || (char >= 'a' && char <= 'z') {
			letters = append(letters, char)
		}
		if len(letters) >= 2 {
			break
		}
	}
	if len(letters) == 0 {
		return "MK"
	}
	if len(letters) == 1 {
		letters = append(letters, 'K')
	}
	return string(letters[:2])
}

func creativeFactoryBrandAbbreviation(brand string) string {
	normalized := strings.ToLower(strings.TrimSpace(brand))
	if normalized == "adakami" || normalized == "ada kami" {
		return "AK"
	}
	return strings.ToUpper(marketAbbreviationFromName(brand))
}

func cleanCreativeFactoryList(values []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		key := strings.ToLower(trimmed)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, trimmed)
	}
	return out
}

func creativeFactoryMarketProfileConfig(profile creativeFactoryMarketProfile) map[string]any {
	return map[string]any{
		"brand":                profile.Brand,
		"market":               profile.Market,
		"market_label":         profile.MarketLabel,
		"locale":               profile.Locale,
		"currency":             profile.Currency,
		"currency_prefix":      profile.CurrencyPrefix,
		"language_label":       profile.LanguageLabel,
		"market_abbreviation":  profile.MarketAbbreviation,
		"competitors":          profile.Competitors,
		"priority_competitors": profile.PriorityCompetitors,
	}
}

func creativeFactoryMarketProfileFromConfig(config map[string]any) (creativeFactoryMarketProfile, bool) {
	raw, ok := config["market_profile"].(map[string]any)
	if !ok {
		return creativeFactoryMarketProfile{}, false
	}
	profile := creativeFactoryMarketProfileFromInput(&creativeFactoryInitializationInput{
		Brand:               stringFromAny(raw["brand"]),
		Market:              stringFromAny(raw["market"]),
		Locale:              stringFromAny(raw["locale"]),
		Currency:            stringFromAny(raw["currency"]),
		Competitors:         stringSliceFromAny(raw["competitors"]),
		PriorityCompetitors: stringSliceFromAny(raw["priority_competitors"]),
	})
	if marketLabel := stringFromAny(raw["market_label"]); marketLabel != "" {
		profile.MarketLabel = marketLabel
	}
	if currencyPrefix := stringFromAny(raw["currency_prefix"]); currencyPrefix != "" {
		profile.CurrencyPrefix = currencyPrefix
	}
	if languageLabel := stringFromAny(raw["language_label"]); languageLabel != "" {
		profile.LanguageLabel = languageLabel
	}
	if marketAbbreviation := stringFromAny(raw["market_abbreviation"]); marketAbbreviation != "" {
		profile.MarketAbbreviation = marketAbbreviation
	}
	profile.Explicit = false
	return profile, strings.TrimSpace(profile.Market) != ""
}

func creativeFactoryResourceNames(profile creativeFactoryMarketProfile) (copyLibraryName string, marketPackName string) {
	return fmt.Sprintf("%s %s 文案库", profile.Brand, profile.Market), fmt.Sprintf("%s %s 市场资源包", profile.Brand, profile.Market)
}

func creativeFactoryAutopilotTitleForProfile(profile creativeFactoryMarketProfile) string {
	if profile.Market == "Indonesia" && profile.Brand == "AdaKami" {
		return creativeFactoryDefaultAutopilotTitle
	}
	return fmt.Sprintf("%s竞品素材周度采集", profile.MarketLabel)
}

func creativeFactoryAutopilotDescriptionForProfile(profile creativeFactoryMarketProfile) string {
	if profile.Market == "Indonesia" && profile.Brand == "AdaKami" && len(profile.Competitors) == 7 {
		return creativeFactoryDefaultAutopilotDescription
	}
	competitors := creativeFactoryListText(profile.Competitors)
	priorityCompetitors := creativeFactoryListText(profile.PriorityCompetitors)
	return fmt.Sprintf(`每周抓取 AppGrowing %s的现金贷/金融竞品素材，并创建一个可追踪的 Crawl Run。

工作区创意工厂安装记录负责解析市场资源包、执行小队和参考分析智能体；不要在 AutoPilot 说明中保存 UUID。

竞品：%s
优先竞品：%s
地区：%s
语言：%s
设备：Android、iOS
媒体：未限定
时间范围：最近 30 天
选材：新素材 40%%，投放少于 7 天且曝光估算大于 1K；跑量素材 60%%，投放超过 30 天且曝光估算不低于 10M
%s
素材类型：仅图片广告（asset_type=image）。视频、非图片和无法识别类型必须在选材前排除，不占用名额，也不进入 Crawl Run 或素材库。

执行真实 AppGrowing 多页图片采集，结果进入创意工厂素材库并关联当前 Crawl Run；图片入库后立即用原生 task fanout 对本次新增图片并发执行逐图创意分析，归档未完成时允许分析读取真实源图片。分析完成后在创意工厂提醒用户选图、确认主题与文案。整个采集与预分析阶段不创建 Issue。授权失效时将 Crawl Run 标为 action_required，并提示用户前往“设置 - 集成”重新绑定，不能用测试数据替代。`,
		profile.MarketLabel, competitors, priorityCompetitors, profile.MarketLabel, profile.LanguageLabel, creativeFactoryDefaultCollectionTargetLine)
}

func creativeFactoryListText(values []string) string {
	cleaned := cleanCreativeFactoryList(values)
	if len(cleaned) == 0 {
		return "待配置"
	}
	return strings.Join(cleaned, "、")
}

func (h *Handler) initializeCreativeFactory(ctx context.Context, workspaceID, userID pgtype.UUID, input *creativeFactoryInitializationInput) (creativeFactoryInstallationRecord, error) {
	if !workspaceID.Valid || !userID.Valid {
		return creativeFactoryInstallationRecord{}, errors.New("workspace and user are required")
	}
	profile := creativeFactoryMarketProfileFromInput(input)
	templates, err := loadCreativeFactoryTemplates()
	if err != nil {
		return creativeFactoryInstallationRecord{}, err
	}
	tx, err := h.TxStarter.Begin(ctx)
	if err != nil {
		return creativeFactoryInstallationRecord{}, err
	}
	defer tx.Rollback(ctx)
	var uploadedDefaultObjectKeys []string
	committed := false
	defer func() {
		if !committed && h.Storage != nil && len(uploadedDefaultObjectKeys) > 0 {
			h.Storage.DeleteKeys(context.Background(), uploadedDefaultObjectKeys)
		}
	}()
	lockKey := "creative-factory-installation:" + uuidToString(workspaceID)
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, lockKey); err != nil {
		return creativeFactoryInstallationRecord{}, err
	}
	existing, existingErr := scanCreativeFactoryInstallation(tx.QueryRow(ctx, `
SELECT workspace_id, status, schema_version, template_version,
       runtime_id, market_pack_id, copy_library_id, squad_id,
       orchestration_skill_id, role_agents, role_skills, config
FROM creative_factory_installation
WHERE workspace_id = $1
FOR UPDATE
	`, workspaceID))
	if existingErr == nil && input == nil {
		if existingProfile, ok := creativeFactoryMarketProfileFromConfig(existing.Config); ok {
			profile = existingProfile
		}
	}
	resourceDefaults, err := creativeFactoryResourceDefaultsForProfile(profile)
	if err != nil {
		return creativeFactoryInstallationRecord{}, err
	}
	if existingErr == nil && (existing.Status == "ready" || existing.Status == "needs_setup") {
		qtx := h.Queries.WithTx(tx)
		if err := h.syncCreativeFactoryManagedAssets(ctx, tx, qtx, workspaceID, userID, &existing, templates); err != nil {
			return creativeFactoryInstallationRecord{}, err
		}
		collectionAgentID, agentIDErr := creativeFactoryRoleAgentID(existing, "collection")
		if agentIDErr != nil {
			return creativeFactoryInstallationRecord{}, agentIDErr
		}
		autopilot, autopilotErr := h.creativeFactoryAutopilot(ctx, tx, qtx, workspaceID, userID, collectionAgentID, existing.SquadID, profile)
		if autopilotErr != nil {
			return creativeFactoryInstallationRecord{}, autopilotErr
		}
		if profile.Explicit {
			copyLibrary, marketPack, clonedKeys, resourceErr := h.ensureCreativeFactoryProfileResources(ctx, tx, workspaceID, userID, resourceDefaults, profile)
			if resourceErr != nil {
				return creativeFactoryInstallationRecord{}, resourceErr
			}
			uploadedDefaultObjectKeys = append(uploadedDefaultObjectKeys, clonedKeys...)
			existing.CopyLibraryID = parseUUID(copyLibrary.ID)
			existing.MarketPackID = parseUUID(marketPack.ID)
			if marketPack.Status != "published" || copyLibrary.Status != "published" || creativeFactoryMarketPackNeedsSetup(marketPack) {
				existing.Status = "needs_setup"
			} else {
				existing.Status = "ready"
			}
		}
		autopilotID := uuidToString(autopilot.ID)
		if existing.Config == nil {
			existing.Config = map[string]any{}
		}
		if existing.Config["autopilot_id"] != autopilotID {
			existing.Config["autopilot_id"] = autopilotID
		}
		existing.Config["template_version"] = creativeFactoryTemplateVersion
		existing.Config["market_profile"] = creativeFactoryMarketProfileConfig(profile)
		roleAgentsJSON, marshalErr := json.Marshal(existing.RoleAgents)
		if marshalErr != nil {
			return creativeFactoryInstallationRecord{}, marshalErr
		}
		roleSkillsJSON, marshalErr := json.Marshal(existing.RoleSkills)
		if marshalErr != nil {
			return creativeFactoryInstallationRecord{}, marshalErr
		}
		configJSON, marshalErr := json.Marshal(existing.Config)
		if marshalErr != nil {
			return creativeFactoryInstallationRecord{}, marshalErr
		}
		if _, updateErr := tx.Exec(ctx, `
UPDATE creative_factory_installation
SET template_version = $2,
    orchestration_skill_id = $3,
    role_agents = $4::jsonb,
    role_skills = $5::jsonb,
    config = $6::jsonb,
    market_pack_id = $7,
    copy_library_id = $8,
    status = $9,
    updated_at = now()
WHERE workspace_id = $1
`, workspaceID, creativeFactoryTemplateVersion, existing.OrchestrationSkillID, string(roleAgentsJSON), string(roleSkillsJSON), string(configJSON), existing.MarketPackID, existing.CopyLibraryID, existing.Status); updateErr != nil {
			return creativeFactoryInstallationRecord{}, updateErr
		}
		if commitErr := tx.Commit(ctx); commitErr != nil {
			return creativeFactoryInstallationRecord{}, commitErr
		}
		return existing, nil
	} else if existingErr != nil && !errors.Is(existingErr, pgx.ErrNoRows) {
		return creativeFactoryInstallationRecord{}, existingErr
	}
	runtimeID, runtimeMode, err := creativeFactoryRuntime(ctx, tx, workspaceID)
	if err != nil {
		return creativeFactoryInstallationRecord{}, err
	}
	qtx := h.Queries.WithTx(tx)
	roleSkills := make(map[string]string, len(creativeFactorySkillSpecs))
	skillIDs := make(map[string]pgtype.UUID, len(creativeFactorySkillSpecs))
	for _, spec := range creativeFactorySkillSpecs {
		template, ok := templates[spec.Directory]
		if !ok {
			return creativeFactoryInstallationRecord{}, fmt.Errorf("creative factory skill template %q is unavailable", spec.Directory)
		}
		skill, err := creativeFactorySkill(ctx, tx, qtx, workspaceID, userID, spec, template)
		if err != nil {
			return creativeFactoryInstallationRecord{}, err
		}
		roleSkills[spec.Role] = uuidToString(skill.ID)
		skillIDs[spec.Role] = skill.ID
	}

	roleAgents := make(map[string]string, len(creativeFactoryAgentSpecs))
	agentIDs := make(map[string]pgtype.UUID, len(creativeFactoryAgentSpecs))
	for _, spec := range creativeFactoryAgentSpecs {
		if spec.Role == "collection" {
			analysisAgentID, ok := agentIDs["reference_analysis"]
			if !ok || !analysisAgentID.Valid {
				return creativeFactoryInstallationRecord{}, errors.New("creative factory collection agent requires the workspace reference-analysis agent")
			}
			spec.Instructions = fmt.Sprintf(spec.Instructions, uuidToString(analysisAgentID))
		}
		skillUUIDs := make([]pgtype.UUID, 0, len(spec.SkillRoles))
		for _, skillRole := range spec.SkillRoles {
			skillID, ok := skillIDs[skillRole]
			if !ok {
				return creativeFactoryInstallationRecord{}, fmt.Errorf("creative factory skill role %q is unavailable", skillRole)
			}
			skillUUIDs = append(skillUUIDs, skillID)
		}
		agent, _, err := creativeFactoryAgent(ctx, tx, qtx, workspaceID, userID, runtimeID, runtimeMode, spec)
		if err != nil {
			return creativeFactoryInstallationRecord{}, err
		}
		for _, skillID := range skillUUIDs {
			if err := qtx.AddAgentSkill(ctx, db.AddAgentSkillParams{AgentID: agent.ID, SkillID: skillID}); err != nil {
				return creativeFactoryInstallationRecord{}, err
			}
		}
		roleAgents[spec.Role] = uuidToString(agent.ID)
		agentIDs[spec.Role] = agent.ID
	}
	if imageEditAgentID, ok := agentIDs["image_edit"]; ok {
		roleAgents["direct_image_edit"] = uuidToString(imageEditAgentID)
		agentIDs["direct_image_edit"] = imageEditAgentID
	}

	leaderID := agentIDs["leadership"]
	squad, squadCreated, err := creativeFactorySquad(ctx, tx, qtx, workspaceID, userID, leaderID)
	if err != nil {
		return creativeFactoryInstallationRecord{}, err
	}
	if squadCreated {
		for _, spec := range creativeFactoryAgentSpecs {
			agentID := agentIDs[spec.Role]
			if _, err := qtx.AddSquadMember(ctx, db.AddSquadMemberParams{SquadID: squad.ID, MemberType: "agent", MemberID: agentID, Role: creativeFactorySquadRole(spec.Role)}); err != nil {
				return creativeFactoryInstallationRecord{}, err
			}
		}
	}
	autopilot, err := h.creativeFactoryAutopilot(ctx, tx, qtx, workspaceID, userID, agentIDs["collection"], squad.ID, profile)
	if err != nil {
		return creativeFactoryInstallationRecord{}, err
	}

	copyLibrary, marketPack, clonedKeys, err := h.ensureCreativeFactoryProfileResources(ctx, tx, workspaceID, userID, resourceDefaults, profile)
	if err != nil {
		return creativeFactoryInstallationRecord{}, err
	}
	uploadedDefaultObjectKeys = append(uploadedDefaultObjectKeys, clonedKeys...)

	roleAgentsJSON, _ := json.Marshal(roleAgents)
	roleSkillsJSON, _ := json.Marshal(roleSkills)
	configJSON, _ := json.Marshal(map[string]any{
		"origin":            "creative_factory",
		"template_version":  creativeFactoryTemplateVersion,
		"resource_policy":   "clone_ad_creative_published_defaults_adopt_existing",
		"user_edits_policy": "never_overwrite",
		"autopilot_id":      uuidToString(autopilot.ID),
		"market_profile":    creativeFactoryMarketProfileConfig(profile),
	})
	status := "ready"
	if marketPack.Status != "published" || copyLibrary.Status != "published" || creativeFactoryMarketPackNeedsSetup(marketPack) {
		status = "needs_setup"
	}
	installation, err := scanCreativeFactoryInstallation(tx.QueryRow(ctx, `
INSERT INTO creative_factory_installation (
  workspace_id, status, schema_version, template_version, runtime_id,
  market_pack_id, copy_library_id, squad_id, orchestration_skill_id,
  role_agents, role_skills, config, initialized_by, initialized_at, last_error
) VALUES ($1, $2, 1, $3, $4, $5, $6, $7, $8, $9::jsonb, $10::jsonb,
          $11::jsonb, $12, now(), '')
ON CONFLICT (workspace_id) DO UPDATE SET
  status = EXCLUDED.status,
  schema_version = EXCLUDED.schema_version,
  template_version = EXCLUDED.template_version,
  runtime_id = EXCLUDED.runtime_id,
  market_pack_id = EXCLUDED.market_pack_id,
  copy_library_id = EXCLUDED.copy_library_id,
  squad_id = EXCLUDED.squad_id,
  orchestration_skill_id = EXCLUDED.orchestration_skill_id,
  role_agents = EXCLUDED.role_agents,
  role_skills = EXCLUDED.role_skills,
  config = EXCLUDED.config,
  initialized_by = EXCLUDED.initialized_by,
  initialized_at = now(),
  last_error = '',
  updated_at = now()
RETURNING workspace_id, status, schema_version, template_version,
          runtime_id, market_pack_id, copy_library_id, squad_id,
          orchestration_skill_id, role_agents, role_skills, config
`, workspaceID, status, creativeFactoryTemplateVersion, runtimeID, marketPack.ID, copyLibrary.ID, squad.ID, skillIDs["creative_leadership"], string(roleAgentsJSON), string(roleSkillsJSON), string(configJSON), userID))
	if err != nil {
		return creativeFactoryInstallationRecord{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return creativeFactoryInstallationRecord{}, err
	}
	committed = true
	return installation, nil
}

func (h *Handler) creativeFactoryAutopilot(ctx context.Context, tx pgx.Tx, qtx *db.Queries, workspaceID, userID, collectionAgentID, legacySquadID pgtype.UUID, profile creativeFactoryMarketProfile) (db.Autopilot, error) {
	var autopilotID pgtype.UUID
	title := creativeFactoryAutopilotTitleForProfile(profile)
	description := creativeFactoryAutopilotDescriptionForProfile(profile)
	titles := []string{title, creativeFactoryLegacyAutopilotTitle}
	if title != creativeFactoryDefaultAutopilotTitle {
		titles = append(titles, creativeFactoryDefaultAutopilotTitle)
	}
	err := tx.QueryRow(ctx, `
SELECT id
FROM autopilot
WHERE workspace_id = $1 AND title = ANY($2::text[])
ORDER BY CASE WHEN title = $3 THEN 0 ELSE 1 END, created_at ASC
LIMIT 1
`, workspaceID, titles, title).Scan(&autopilotID)
	if err == nil {
		autopilot, getErr := qtx.GetAutopilotInWorkspace(ctx, db.GetAutopilotInWorkspaceParams{ID: autopilotID, WorkspaceID: workspaceID})
		if getErr != nil {
			return db.Autopilot{}, getErr
		}
		if autopilot.Title == title {
			return autopilot, nil
		}
		if creativeFactoryAutopilotNeedsMigration(autopilot, legacySquadID) || creativeFactoryAutopilotUsesInstallerDefault(autopilot) {
			updated, updateErr := qtx.UpdateAutopilot(ctx, db.UpdateAutopilotParams{
				ID:                 autopilot.ID,
				Title:              pgtype.Text{String: title, Valid: true},
				Description:        pgtype.Text{String: description, Valid: true},
				AssigneeType:       pgtype.Text{String: "agent", Valid: true},
				AssigneeID:         collectionAgentID,
				Status:             pgtype.Text{String: "active", Valid: true},
				ExecutionMode:      pgtype.Text{String: "run_only", Valid: true},
				IssueTitleTemplate: autopilot.IssueTitleTemplate,
				ProjectID:          autopilot.ProjectID,
			})
			if updateErr != nil {
				return db.Autopilot{}, updateErr
			}
			if versionErr := h.recordAutopilotRuleVersion(ctx, qtx, updated, "member", userID); versionErr != nil {
				return db.Autopilot{}, versionErr
			}
			return updated, nil
		}
		// A user-edited legacy row remains untouched. Fall through and create
		// the workspace's standard collection plan alongside it.
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return db.Autopilot{}, err
	}
	autopilot, err := qtx.CreateAutopilot(ctx, db.CreateAutopilotParams{
		WorkspaceID:   workspaceID,
		Title:         title,
		Description:   pgtype.Text{String: description, Valid: true},
		AssigneeType:  "agent",
		AssigneeID:    collectionAgentID,
		Status:        "active",
		ExecutionMode: "run_only",
		CreatedByType: "member",
		CreatedByID:   userID,
	})
	if err != nil {
		return db.Autopilot{}, err
	}
	if err := h.recordAutopilotRuleVersion(ctx, qtx, autopilot, "member", userID); err != nil {
		return db.Autopilot{}, err
	}
	return autopilot, nil
}

func creativeFactoryRoleAgentID(installation creativeFactoryInstallationRecord, role string) (pgtype.UUID, error) {
	rawID := strings.TrimSpace(installation.RoleAgents[role])
	if rawID == "" {
		return pgtype.UUID{}, fmt.Errorf("creative factory installation is missing the %s agent", role)
	}
	agentID, err := parseUUIDString(rawID)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("creative factory installation has invalid %s agent: %w", role, err)
	}
	return agentID, nil
}

func (h *Handler) syncCreativeFactoryManagedAssets(ctx context.Context, tx pgx.Tx, qtx *db.Queries, workspaceID, userID pgtype.UUID, installation *creativeFactoryInstallationRecord, templates map[string]creativeFactoryTemplate) error {
	if installation.RoleSkills == nil {
		installation.RoleSkills = map[string]string{}
	}
	if installation.RoleAgents == nil {
		installation.RoleAgents = map[string]string{}
	}
	roleSkills := make(map[string]string, len(creativeFactorySkillSpecs))
	skillIDs := make(map[string]pgtype.UUID, len(creativeFactorySkillSpecs))
	for _, spec := range creativeFactorySkillSpecs {
		template, ok := templates[spec.Directory]
		if !ok {
			return fmt.Errorf("creative factory skill template %q is unavailable", spec.Directory)
		}
		skill, found, err := creativeFactoryRoleSkill(ctx, qtx, workspaceID, installation.RoleSkills[spec.Role])
		if err != nil {
			return err
		}
		if !found {
			skill, err = creativeFactorySkill(ctx, tx, qtx, workspaceID, userID, spec, template)
			if err != nil {
				return err
			}
		}
		skill, err = syncCreativeFactorySkillTemplate(ctx, tx, qtx, workspaceID, skill, spec, template)
		if err != nil {
			return err
		}
		roleSkills[spec.Role] = uuidToString(skill.ID)
		skillIDs[spec.Role] = skill.ID
	}

	runtimeID, runtimeMode, err := creativeFactoryRuntime(ctx, tx, workspaceID)
	if err != nil {
		return err
	}
	roleAgents := make(map[string]string, len(creativeFactoryAgentSpecs)+1)
	agentIDs := make(map[string]pgtype.UUID, len(creativeFactoryAgentSpecs)+1)
	for _, baseSpec := range creativeFactoryAgentSpecs {
		spec := baseSpec
		if spec.Role == "collection" {
			analysisAgentID, ok := agentIDs["reference_analysis"]
			if !ok || !analysisAgentID.Valid {
				return errors.New("creative factory collection agent requires the workspace reference-analysis agent")
			}
			spec.Instructions = fmt.Sprintf(spec.Instructions, uuidToString(analysisAgentID))
		}
		skillUUIDs := make([]pgtype.UUID, 0, len(spec.SkillRoles))
		for _, skillRole := range spec.SkillRoles {
			skillID, ok := skillIDs[skillRole]
			if !ok {
				return fmt.Errorf("creative factory skill role %q is unavailable", skillRole)
			}
			skillUUIDs = append(skillUUIDs, skillID)
		}
		agent, found, err := creativeFactoryRoleAgent(ctx, qtx, workspaceID, installation.RoleAgents[spec.Role])
		if err != nil {
			return err
		}
		created := false
		if !found {
			agent, created, err = creativeFactoryAgent(ctx, tx, qtx, workspaceID, userID, runtimeID, runtimeMode, spec)
			if err != nil {
				return err
			}
		}
		agent, err = syncCreativeFactoryAgentTemplate(ctx, tx, qtx, workspaceID, agent, spec)
		if err != nil {
			return err
		}
		for _, skillID := range skillUUIDs {
			if err := qtx.AddAgentSkill(ctx, db.AddAgentSkillParams{AgentID: agent.ID, SkillID: skillID}); err != nil {
				return err
			}
		}
		if created && installation.SquadID.Valid {
			if err := ensureCreativeFactorySquadMember(ctx, tx, workspaceID, installation.SquadID, agent.ID, creativeFactorySquadRole(spec.Role)); err != nil {
				return err
			}
		}
		roleAgents[spec.Role] = uuidToString(agent.ID)
		agentIDs[spec.Role] = agent.ID
	}
	if imageEditAgentID, ok := agentIDs["image_edit"]; ok {
		roleAgents["direct_image_edit"] = uuidToString(imageEditAgentID)
		agentIDs["direct_image_edit"] = imageEditAgentID
	}
	if installation.SquadID.Valid {
		for _, spec := range creativeFactoryAgentSpecs {
			agentID, ok := agentIDs[spec.Role]
			if !ok || !agentID.Valid {
				continue
			}
			if err := ensureCreativeFactorySquadMember(ctx, tx, workspaceID, installation.SquadID, agentID, creativeFactorySquadRole(spec.Role)); err != nil {
				return err
			}
		}
		for _, spec := range creativeFactoryAgentSpecs {
			if spec.Role != "image_edit" {
				continue
			}
			if err := syncCreativeFactoryImageEditPoolAgents(ctx, tx, workspaceID, installation.SquadID, skillIDs, spec); err != nil {
				return err
			}
			break
		}
		if leaderID, ok := agentIDs["leadership"]; ok && leaderID.Valid {
			if _, err := tx.Exec(ctx, `
UPDATE squad
SET leader_id = $2, updated_at = now()
WHERE id = $1 AND workspace_id = $3 AND archived_at IS NULL
`, installation.SquadID, leaderID, workspaceID); err != nil {
				return err
			}
		}
	}
	installation.RoleSkills = roleSkills
	installation.RoleAgents = roleAgents
	installation.OrchestrationSkillID = skillIDs["creative_leadership"]
	return nil
}

func creativeFactoryRoleSkill(ctx context.Context, qtx *db.Queries, workspaceID pgtype.UUID, rawID string) (db.Skill, bool, error) {
	rawID = strings.TrimSpace(rawID)
	if rawID == "" {
		return db.Skill{}, false, nil
	}
	skillID, err := parseUUIDString(rawID)
	if err != nil {
		return db.Skill{}, false, nil
	}
	skill, err := qtx.GetSkillInWorkspace(ctx, db.GetSkillInWorkspaceParams{ID: skillID, WorkspaceID: workspaceID})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.Skill{}, false, nil
	}
	return skill, err == nil, err
}

func creativeFactoryRoleAgent(ctx context.Context, qtx *db.Queries, workspaceID pgtype.UUID, rawID string) (db.Agent, bool, error) {
	rawID = strings.TrimSpace(rawID)
	if rawID == "" {
		return db.Agent{}, false, nil
	}
	agentID, err := parseUUIDString(rawID)
	if err != nil {
		return db.Agent{}, false, nil
	}
	agent, err := qtx.GetAgentInWorkspace(ctx, db.GetAgentInWorkspaceParams{ID: agentID, WorkspaceID: workspaceID})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.Agent{}, false, nil
	}
	return agent, err == nil, err
}

func syncCreativeFactorySkillTemplate(ctx context.Context, tx pgx.Tx, qtx *db.Queries, workspaceID pgtype.UUID, skill db.Skill, spec creativeFactorySkillSpec, template creativeFactoryTemplate) (db.Skill, error) {
	config, err := json.Marshal(creativeFactorySkillConfig(spec))
	if err != nil {
		return db.Skill{}, err
	}
	if _, err := tx.Exec(ctx, `
UPDATE skill
SET description = $3,
    content = $4,
    config = $5::jsonb,
    updated_at = now()
WHERE id = $1 AND workspace_id = $2
`, skill.ID, workspaceID, spec.Description, template.Content, string(config)); err != nil {
		return db.Skill{}, err
	}
	if err := qtx.DeleteSkillFilesBySkill(ctx, skill.ID); err != nil {
		return db.Skill{}, err
	}
	for _, file := range template.Files {
		if _, err := qtx.UpsertSkillFile(ctx, db.UpsertSkillFileParams{SkillID: skill.ID, Path: file.Path, Content: file.Content}); err != nil {
			return db.Skill{}, err
		}
	}
	return qtx.GetSkillInWorkspace(ctx, db.GetSkillInWorkspaceParams{ID: skill.ID, WorkspaceID: workspaceID})
}

func syncCreativeFactoryAgentTemplate(ctx context.Context, tx pgx.Tx, qtx *db.Queries, workspaceID pgtype.UUID, agent db.Agent, spec creativeFactoryAgentSpec) (db.Agent, error) {
	var model any
	if strings.TrimSpace(spec.Model) != "" {
		model = spec.Model
	}
	var thinkingLevel any
	if strings.TrimSpace(spec.ThinkingLevel) != "" {
		thinkingLevel = spec.ThinkingLevel
	}
	if _, err := tx.Exec(ctx, `
UPDATE agent
SET description = $3,
    instructions = $4,
    max_concurrent_tasks = $5,
    model = $6,
    thinking_level = $7,
    updated_at = now()
WHERE id = $1 AND workspace_id = $2 AND archived_at IS NULL
`, agent.ID, workspaceID, spec.Description, spec.Instructions, spec.MaxConcurrent, model, thinkingLevel); err != nil {
		return db.Agent{}, err
	}
	return qtx.GetAgentInWorkspace(ctx, db.GetAgentInWorkspaceParams{ID: agent.ID, WorkspaceID: workspaceID})
}

func syncCreativeFactoryImageEditPoolAgents(ctx context.Context, tx pgx.Tx, workspaceID, squadID pgtype.UUID, skillIDs map[string]pgtype.UUID, spec creativeFactoryAgentSpec) error {
	imageEditSkillID := skillIDs["image_edit"]
	directEditSkillID := skillIDs["direct_image_edit"]
	primeComposeSkillID := skillIDs["prime_compose"]
	if !workspaceID.Valid || !squadID.Valid || !imageEditSkillID.Valid || !directEditSkillID.Valid || !primeComposeSkillID.Valid {
		return nil
	}

	// Prime compose is an execution dependency for every image-edit lane. Keep
	// custom pool instructions intact while making that dependency explicit.
	if _, err := tx.Exec(ctx, `
INSERT INTO agent_skill (agent_id, skill_id, enabled)
SELECT DISTINCT pool_agent.id, $4::uuid, true
FROM squad_member member
JOIN squad pool_squad
  ON pool_squad.id = member.squad_id
 AND pool_squad.workspace_id = $2
 AND pool_squad.archived_at IS NULL
JOIN agent pool_agent
  ON pool_agent.id = member.member_id
 AND pool_agent.workspace_id = $2
 AND pool_agent.archived_at IS NULL
JOIN agent_skill lane_binding
  ON lane_binding.agent_id = pool_agent.id
 AND lane_binding.enabled
WHERE member.squad_id = $1
  AND member.member_type = 'agent'
  AND lane_binding.skill_id IN ($3::uuid, $5::uuid)
ON CONFLICT (agent_id, skill_id) DO UPDATE SET enabled = true
`, squadID, workspaceID, imageEditSkillID, primeComposeSkillID, directEditSkillID); err != nil {
		return err
	}

	rows, err := tx.Query(ctx, `
UPDATE agent pool_agent
SET description = $4,
    instructions = $5,
    updated_at = now()
FROM squad_member member
JOIN squad pool_squad
  ON pool_squad.id = member.squad_id
 AND pool_squad.workspace_id = $2
 AND pool_squad.archived_at IS NULL
JOIN agent_skill binding
  ON binding.agent_id = member.member_id
 AND binding.enabled
WHERE member.squad_id = $1
  AND member.member_type = 'agent'
  AND binding.skill_id = $3
  AND pool_agent.id = member.member_id
  AND pool_agent.workspace_id = $2
  AND pool_agent.archived_at IS NULL
  AND (
    pool_agent.instructions LIKE '%创意工厂出图能力池合同版本：%'
    OR (
      pool_agent.instructions LIKE '%不创建 QC task%'
      AND (
        pool_agent.instructions LIKE '%delivered_with_qc_risk%'
        OR pool_agent.description LIKE '%精准改图直接交付%'
      )
    )
  )
RETURNING pool_agent.id
`, squadID, workspaceID, imageEditSkillID, spec.Description, spec.Instructions)
	if err != nil {
		return err
	}
	migrated := make([]pgtype.UUID, 0)
	for rows.Next() {
		var agentID pgtype.UUID
		if err := rows.Scan(&agentID); err != nil {
			rows.Close()
			return err
		}
		migrated = append(migrated, agentID)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, agentID := range migrated {
		if _, err := tx.Exec(ctx, `
INSERT INTO agent_skill (agent_id, skill_id, enabled)
VALUES ($1::uuid, $2::uuid, true), ($1::uuid, $3::uuid, true)
ON CONFLICT (agent_id, skill_id) DO UPDATE SET enabled = true
`, agentID, directEditSkillID, primeComposeSkillID); err != nil {
			return err
		}
	}
	return nil
}

func ensureCreativeFactorySquadMember(ctx context.Context, tx pgx.Tx, workspaceID, squadID, agentID pgtype.UUID, role string) error {
	_, err := tx.Exec(ctx, `
INSERT INTO squad_member (squad_id, member_type, member_id, role)
SELECT $1, 'agent', $2, $3
FROM squad
WHERE id = $1 AND workspace_id = $4 AND archived_at IS NULL
ON CONFLICT (squad_id, member_type, member_id) DO UPDATE SET role = EXCLUDED.role
`, squadID, agentID, role, workspaceID)
	return err
}

func creativeFactoryAutopilotNeedsMigration(autopilot db.Autopilot, legacySquadID pgtype.UUID) bool {
	return autopilot.AssigneeType == "squad" &&
		legacySquadID.Valid && autopilot.AssigneeID == legacySquadID &&
		autopilot.Status == "active" &&
		autopilot.ExecutionMode == "run_only" &&
		!autopilot.IssueTitleTemplate.Valid &&
		!autopilot.ProjectID.Valid &&
		!autopilot.LastRunAt.Valid &&
		autopilot.Description.Valid &&
		strings.TrimSpace(autopilot.Description.String) == "由创意工厂事件驱动，从素材分析、文案适配到出图和验收自动推进。"
}

func creativeFactoryAutopilotUsesInstallerDefault(autopilot db.Autopilot) bool {
	return autopilot.Title == creativeFactoryDefaultAutopilotTitle &&
		creativeFactoryAutopilotHasStandardAgentShape(autopilot) &&
		creativeFactoryAutopilotDescriptionLooksManaged(autopilot.Description)
}

func creativeFactoryAutopilotHasStandardAgentShape(autopilot db.Autopilot) bool {
	return autopilot.AssigneeType == "agent" &&
		autopilot.Status == "active" &&
		autopilot.ExecutionMode == "run_only" &&
		!autopilot.IssueTitleTemplate.Valid &&
		!autopilot.ProjectID.Valid
}

func creativeFactoryAutopilotDescriptionLooksManaged(description pgtype.Text) bool {
	if !description.Valid {
		return false
	}
	text := strings.TrimSpace(description.String)
	if text == creativeFactoryDefaultAutopilotDescription {
		return true
	}
	for _, marker := range []string{
		"工作区创意工厂安装记录负责解析市场资源包",
		"执行真实 AppGrowing 多页图片采集",
		"素材类型：仅图片广告（asset_type=image）",
	} {
		if !strings.Contains(text, marker) {
			return false
		}
	}
	return true
}

func loadCreativeFactoryTemplates() (map[string]creativeFactoryTemplate, error) {
	root, err := creativeFactoryTemplateRoot()
	if err != nil {
		return nil, err
	}
	templates := make(map[string]creativeFactoryTemplate, len(creativeFactorySkillSpecs))
	for _, spec := range creativeFactorySkillSpecs {
		directory := filepath.Join(root, spec.Directory)
		content, err := os.ReadFile(filepath.Join(directory, "SKILL.md"))
		if err != nil {
			return nil, fmt.Errorf("read creative factory skill %s: %w", spec.Directory, err)
		}
		if !utf8.Valid(content) {
			return nil, fmt.Errorf("creative factory skill %s contains invalid UTF-8", spec.Directory)
		}
		files := make([]CreateSkillFileRequest, 0)
		referenceRoot := filepath.Join(directory, "references")
		if info, statErr := os.Stat(referenceRoot); statErr == nil && info.IsDir() {
			walkErr := filepath.WalkDir(referenceRoot, func(path string, entry os.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if entry.IsDir() {
					if entry.Name() == "__pycache__" || entry.Name() == ".pytest_cache" {
						return filepath.SkipDir
					}
					return nil
				}
				data, readErr := os.ReadFile(path)
				if readErr != nil {
					return readErr
				}
				if !utf8.Valid(data) {
					return nil
				}
				rel, relErr := filepath.Rel(directory, path)
				if relErr != nil {
					return relErr
				}
				files = append(files, CreateSkillFileRequest{Path: filepath.ToSlash(rel), Content: string(data)})
				return nil
			})
			if walkErr != nil {
				return nil, fmt.Errorf("read creative factory skill files %s: %w", spec.Directory, walkErr)
			}
		}
		sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
		templates[spec.Directory] = creativeFactoryTemplate{Content: string(content), Files: files}
	}
	return templates, nil
}

func creativeFactoryTemplateRoot() (string, error) {
	candidates := []string{}
	if configured := strings.TrimSpace(os.Getenv("MULTICA_CREATIVE_FACTORY_TEMPLATE_ROOT")); configured != "" {
		candidates = append(candidates, configured)
	}
	if working, err := os.Getwd(); err == nil {
		candidates = append(candidates,
			filepath.Join(working, "scripts", "creative-platform-skills"),
			filepath.Join(working, "..", "scripts", "creative-platform-skills"),
		)
	}
	if executable, err := os.Executable(); err == nil {
		directory := filepath.Dir(executable)
		candidates = append(candidates,
			filepath.Join(directory, "creative-platform-skills"),
			filepath.Join(directory, "..", "scripts", "creative-platform-skills"),
		)
	}
	if runtime.GOOS != "windows" {
		candidates = append(candidates, "/app/creative-platform-skills")
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate, nil
		}
	}
	return "", errors.New("creative factory templates are unavailable; configure MULTICA_CREATIVE_FACTORY_TEMPLATE_ROOT")
}

func creativeFactoryRuntime(ctx context.Context, tx pgx.Tx, workspaceID pgtype.UUID) (pgtype.UUID, string, error) {
	var runtimeID pgtype.UUID
	var runtimeMode string
	err := tx.QueryRow(ctx, `
SELECT id, runtime_mode
FROM agent_runtime
WHERE workspace_id = $1
ORDER BY (status = 'online') DESC, updated_at DESC, created_at ASC
LIMIT 1
`, workspaceID).Scan(&runtimeID, &runtimeMode)
	if errors.Is(err, pgx.ErrNoRows) {
		return pgtype.UUID{}, "", errors.New("creative factory requires an available workspace runtime")
	}
	return runtimeID, runtimeMode, err
}

func creativeFactorySkillConfig(spec creativeFactorySkillSpec) map[string]any {
	return map[string]any{
		"kind":             "creative_role",
		"capability":       spec.Capability,
		"version":          spec.Version,
		"template_version": creativeFactoryTemplateVersion,
		"origin":           "creative_factory",
	}
}

func creativeFactorySkill(ctx context.Context, tx pgx.Tx, qtx *db.Queries, workspaceID, userID pgtype.UUID, spec creativeFactorySkillSpec, template creativeFactoryTemplate) (db.Skill, error) {
	names := append([]string{spec.Name}, spec.Aliases...)
	var skillID pgtype.UUID
	err := tx.QueryRow(ctx, `
SELECT id FROM skill
WHERE workspace_id = $1 AND name = ANY($2::text[])
ORDER BY CASE WHEN name = $3 THEN 0 ELSE 1 END, created_at
LIMIT 1
`, workspaceID, names, spec.Name).Scan(&skillID)
	if err == nil {
		return qtx.GetSkillInWorkspace(ctx, db.GetSkillInWorkspaceParams{ID: skillID, WorkspaceID: workspaceID})
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return db.Skill{}, err
	}
	created, err := createSkillWithFilesInTx(ctx, qtx, skillCreateInput{
		WorkspaceID: workspaceID,
		CreatorID:   userID,
		Name:        spec.Name,
		Description: spec.Description,
		Content:     template.Content,
		Config:      creativeFactorySkillConfig(spec),
		Files:       template.Files,
	})
	if err != nil {
		return db.Skill{}, err
	}
	return qtx.GetSkillInWorkspace(ctx, db.GetSkillInWorkspaceParams{ID: parseUUID(created.ID), WorkspaceID: workspaceID})
}

func creativeFactoryAgent(ctx context.Context, tx pgx.Tx, qtx *db.Queries, workspaceID, userID, runtimeID pgtype.UUID, runtimeMode string, spec creativeFactoryAgentSpec) (db.Agent, bool, error) {
	names := append([]string{spec.Name}, spec.Aliases...)
	var agentID pgtype.UUID
	err := tx.QueryRow(ctx, `
SELECT id FROM agent
WHERE workspace_id = $1 AND archived_at IS NULL AND name = ANY($2::text[])
ORDER BY CASE WHEN name = $3 THEN 0 ELSE 1 END, created_at
LIMIT 1
`, workspaceID, names, spec.Name).Scan(&agentID)
	if err == nil {
		agent, getErr := qtx.GetAgentInWorkspace(ctx, db.GetAgentInWorkspaceParams{ID: agentID, WorkspaceID: workspaceID})
		return agent, false, getErr
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return db.Agent{}, false, err
	}
	agent, err := qtx.CreateAgent(ctx, db.CreateAgentParams{
		WorkspaceID:        workspaceID,
		Name:               spec.Name,
		Description:        spec.Description,
		Instructions:       spec.Instructions,
		RuntimeMode:        runtimeMode,
		RuntimeConfig:      []byte(`{}`),
		RuntimeID:          runtimeID,
		Visibility:         "workspace",
		MaxConcurrentTasks: spec.MaxConcurrent,
		OwnerID:            userID,
		CustomEnv:          []byte(`{}`),
		CustomArgs:         []byte(`[]`),
		Model:              pgtype.Text{String: spec.Model, Valid: spec.Model != ""},
		ThinkingLevel:      pgtype.Text{String: spec.ThinkingLevel, Valid: spec.ThinkingLevel != ""},
	})
	return agent, true, err
}

func creativeFactorySquad(ctx context.Context, tx pgx.Tx, qtx *db.Queries, workspaceID, userID, leaderID pgtype.UUID) (db.Squad, bool, error) {
	names := []string{"素材流程小队", "AdaKami 素材小队"}
	var squadID pgtype.UUID
	err := tx.QueryRow(ctx, `
SELECT id FROM squad
WHERE workspace_id = $1 AND archived_at IS NULL AND name = ANY($2::text[])
ORDER BY CASE WHEN name = '素材流程小队' THEN 0 ELSE 1 END, created_at
LIMIT 1
`, workspaceID, names).Scan(&squadID)
	if err == nil {
		squad, getErr := qtx.GetSquadInWorkspace(ctx, db.GetSquadInWorkspaceParams{ID: squadID, WorkspaceID: workspaceID})
		return squad, false, getErr
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return db.Squad{}, false, err
	}
	squad, err := qtx.CreateSquad(ctx, db.CreateSquadParams{
		WorkspaceID: workspaceID,
		Name:        "素材流程小队",
		Description: "从 AppGrowing 候选采集、逐图文案确认到三尺寸修图、完整贴图和验收发布。",
		LeaderID:    leaderID,
		CreatorID:   userID,
	})
	if err != nil {
		return db.Squad{}, false, err
	}
	if _, err := tx.Exec(ctx, `
UPDATE squad SET instructions = $2, updated_at = now()
WHERE id = $1
	`, squad.ID, "Leader 读取 Creative Order 领域状态以及名册中每个智能体的职责和平台 Skill，动态选择成员。一个订单只关联一个用户可见 Issue；分析、方案、变体生成和 QC 通过原生 task fanout 委派，品牌组件由后端确定性合成，不创建子 Issue。领域对象保存过程与证据，Issue 只保留用户目标、决定、真实阻塞和最终验收。"); err != nil {
		return db.Squad{}, false, err
	}
	squad.Instructions = "Leader 读取 Creative Order 领域状态以及名册中每个智能体的职责和平台 Skill，动态选择成员。一个订单只关联一个用户可见 Issue；分析、方案、变体生成和 QC 通过原生 task fanout 委派，品牌组件由后端确定性合成，不创建子 Issue。领域对象保存过程与证据，Issue 只保留用户目标、决定、真实阻塞和最终验收。"
	return squad, true, nil
}

func creativeFactorySquadRole(role string) string {
	roles := map[string]string{
		"leadership": "流程统筹", "collection": "素材采集", "diagnostics": "流程诊断",
		"reference_analysis": "参考分析", "generation_plan": "生成方案", "image_edit": "图像编辑",
		"quality_control": "质量验收",
	}
	return roles[role]
}

func creativeFactoryResourceDefaults() (map[string]creativeFactoryResourceDefault, error) {
	defaults := map[string]creativeFactoryResourceDefault{}
	if err := json.Unmarshal(creativeFactoryDefaultResourcesJSON, &defaults); err != nil {
		return nil, fmt.Errorf("decode embedded creative factory defaults: %w", err)
	}
	if marketPack, ok := defaults["market_pack"]; ok {
		marketPack.Config = creativeFactoryAdaKamiNamingConfig(marketPack.Config)
		defaults["market_pack"] = marketPack
	}
	for _, kind := range []string{"copy_library", "market_pack"} {
		resource, ok := defaults[kind]
		if !ok || len(resource.Config) == 0 {
			return nil, fmt.Errorf("embedded creative factory default %s is missing config", kind)
		}
		if kind == "market_pack" && len(resource.Files) == 0 {
			return nil, errors.New("embedded creative factory market pack has no files")
		}
	}
	return defaults, nil
}

func creativeFactoryResourceDefaultsForProfile(profile creativeFactoryMarketProfile) (map[string]creativeFactoryResourceDefault, error) {
	defaults, err := creativeFactoryResourceDefaults()
	if err != nil {
		return nil, err
	}
	copyDefault := defaults["copy_library"]
	copyConfig := cloneCreativeFactoryConfig(copyDefault.Config)
	copyConfig["market"] = profile.Market
	copyConfig["locale"] = profile.Locale
	copyDefault.Description = fmt.Sprintf("%s %s 已审核投放文案和还款计划库。", profile.Brand, profile.MarketLabel)

	usesEmbeddedIndonesiaCopy := profile.Brand == "AdaKami" && profile.Market == "Indonesia" && profile.Locale == "id-ID" && profile.Currency == "IDR"
	if !usesEmbeddedIndonesiaCopy {
		copyConfig["fragments"] = []any{}
		copyConfig["recipes"] = []any{}
		copyConfig["source"] = map[string]any{
			"name":        "",
			"url":         "",
			"sync_status": "pending",
			"note":        "待补充当前市场已审核文案与还款计划。",
		}
		copyConfig["repayment_plan"] = map[string]any{
			"labels": map[string]any{
				"principal":           "Principal",
				"tenor":               "Tenor",
				"monthly_installment": "Monthly Installment",
				"total_interest":      "Total Interest",
				"total_repayment":     "Total Repayment",
			},
			"entries": []any{},
		}
	}
	copyDefault.Config = copyConfig
	defaults["copy_library"] = copyDefault

	marketDefault := defaults["market_pack"]
	marketConfig := cloneCreativeFactoryConfig(marketDefault.Config)
	marketConfig["brand"] = profile.Brand
	marketConfig["market"] = profile.Market
	marketConfig["locale"] = profile.Locale
	marketConfig["currency"] = profile.Currency
	if !usesEmbeddedIndonesiaCopy {
		marketConfig["calculation_rules"] = []any{}
		marketConfig["compliance_rules"] = "只能使用当前市场已审核的金融事实、文案和品牌组件；不得复制竞品品牌、金额、法律文字或专属页面元素。"
	}
	if !usesEmbeddedIndonesiaCopy {
		namingDefaults, ok := marketConfig["naming_defaults"].(map[string]any)
		if !ok {
			namingDefaults = map[string]any{}
		}
		namingDefaults["brand_abbreviation"] = creativeFactoryBrandAbbreviation(profile.Brand)
		namingDefaults["market_abbreviation"] = profile.MarketAbbreviation
		marketConfig["naming_defaults"] = namingDefaults
	}
	marketDefault.Description = fmt.Sprintf("%s %s 市场规则、品牌组件和交付配置。", profile.Brand, profile.MarketLabel)
	marketDefault.Config = marketConfig
	marketDefault.Files = creativeFactoryResourceFileSeedsForProfile(marketDefault.Files, profile)
	defaults["market_pack"] = marketDefault
	return defaults, nil
}

func creativeFactoryResourceFileSeedsForProfile(files []creativeFactoryResourceFileSeed, profile creativeFactoryMarketProfile) []creativeFactoryResourceFileSeed {
	if len(files) == 0 {
		return nil
	}
	out := make([]creativeFactoryResourceFileSeed, 0, len(files))
	for _, file := range files {
		seed := file
		var metadata map[string]any
		if len(seed.Metadata) > 0 && json.Unmarshal(seed.Metadata, &metadata) == nil {
			if seed.Role == "app_ui_reference" {
				metadata["market"] = profile.Market
				metadata["locale"] = profile.Locale
			}
			if encoded, err := json.Marshal(metadata); err == nil {
				seed.Metadata = encoded
			}
		}
		out = append(out, seed)
	}
	return out
}

func creativeFactoryAdaKamiNamingConfig(config map[string]any) map[string]any {
	normalized := cloneCreativeFactoryConfig(config)
	normalized["naming_rule"] = "{month}_P_AK_MY_{date}_{type}_{theme}_{device}_{designer}_{size}"
	normalized["video_naming_rule"] = "{month}_V_AK_MY_{date}_{type}_{theme}_{device}_{designer}_{size}_{duration}"
	normalized["naming_size_abbreviations"] = map[string]any{
		"1080x1080": "11",
		"1920x1080": "169",
		"1200x628":  "191",
		"1080x1920": "916",
		"800x1000":  "45",
	}
	normalized["naming_defaults"] = map[string]any{
		"device":              "SX",
		"designer":            "AI",
		"brand_abbreviation":  "AK",
		"market_abbreviation": "MY",
	}
	return normalized
}

func cloneCreativeFactoryConfig(config map[string]any) map[string]any {
	if config == nil {
		return map[string]any{}
	}
	encoded, err := json.Marshal(config)
	if err != nil {
		return map[string]any{}
	}
	clone := map[string]any{}
	if err := json.Unmarshal(encoded, &clone); err != nil {
		return map[string]any{}
	}
	return clone
}

func (h *Handler) cloneCreativeFactoryResourceFiles(ctx context.Context, tx pgx.Tx, workspaceID, userID, targetResourceID pgtype.UUID, fileSeeds []creativeFactoryResourceFileSeed) ([]string, error) {
	if h.Storage == nil {
		return nil, errors.New("creative factory default market pack requires configured storage")
	}
	uploadedKeys := []string{}
	cleanup := func() {
		if len(uploadedKeys) > 0 {
			h.Storage.DeleteKeys(context.Background(), uploadedKeys)
		}
	}
	for _, seed := range fileSeeds {
		data, err := creativeFactoryDefaultAssets.ReadFile(seed.AssetPath)
		if err != nil {
			cleanup()
			return nil, fmt.Errorf("read embedded creative factory market file %s: %w", seed.AssetPath, err)
		}
		if len(data) > maxCreativePrimeInputBytes {
			cleanup()
			return nil, fmt.Errorf("embedded creative factory market file %s exceeds %d bytes", seed.AssetPath, maxCreativePrimeInputBytes)
		}
		attachmentID, err := uuid.NewV7()
		if err != nil {
			cleanup()
			return nil, fmt.Errorf("create creative factory default attachment: %w", err)
		}
		objectFilename := filepath.Base(strings.ReplaceAll(seed.AssetPath, "\\", "/"))
		if objectFilename == "." || objectFilename == "" {
			objectFilename = "market-resource"
		}
		filename := objectFilename
		var metadata struct {
			SourceFilename string `json:"source_filename"`
		}
		if err := json.Unmarshal(seed.Metadata, &metadata); err == nil && strings.TrimSpace(metadata.SourceFilename) != "" {
			filename = metadata.SourceFilename
		}
		contentType := "application/octet-stream"
		switch strings.ToLower(filepath.Ext(objectFilename)) {
		case ".jpg", ".jpeg":
			contentType = "image/jpeg"
		case ".png":
			contentType = "image/png"
		}
		objectKey := filepath.ToSlash(filepath.Join(
			"workspaces", uuidToString(workspaceID), "creative-factory-defaults",
			uuidToString(targetResourceID), attachmentID.String()+"-"+objectFilename,
		))
		objectURL, err := h.Storage.Upload(ctx, objectKey, data, contentType, filename)
		if err != nil {
			cleanup()
			return nil, fmt.Errorf("copy embedded creative factory market file %s: %w", seed.AssetPath, err)
		}
		uploadedKeys = append(uploadedKeys, objectKey)
		if _, err := tx.Exec(ctx, `
INSERT INTO attachment (id, workspace_id, uploader_type, uploader_id, filename, url, content_type, size_bytes)
VALUES ($1, $2, 'member', $3, $4, $5, $6, $7)
		`, pgtype.UUID{Bytes: attachmentID, Valid: true}, workspaceID, userID, filename, objectURL, contentType, int64(len(data))); err != nil {
			cleanup()
			return nil, fmt.Errorf("register creative factory default attachment %s: %w", seed.AssetPath, err)
		}
		if _, err := tx.Exec(ctx, `
INSERT INTO creative_resource_file (
  resource_id, workspace_id, attachment_id, role, label, metadata, created_version, created_by
) VALUES ($1, $2, $3, $4, $5, $6::jsonb, 1, $7)
`, targetResourceID, workspaceID, pgtype.UUID{Bytes: attachmentID, Valid: true}, seed.Role, seed.Label, seed.Metadata, userID); err != nil {
			cleanup()
			return nil, fmt.Errorf("register creative factory default market file %s: %w", seed.AssetPath, err)
		}
	}
	return uploadedKeys, nil
}

func (h *Handler) ensureCreativeFactoryProfileResources(ctx context.Context, tx pgx.Tx, workspaceID, userID pgtype.UUID, resourceDefaults map[string]creativeFactoryResourceDefault, profile creativeFactoryMarketProfile) (creativeResourceResponse, creativeResourceResponse, []string, error) {
	copyDefault := resourceDefaults["copy_library"]
	marketDefault := resourceDefaults["market_pack"]
	copyLibraryName, marketPackName := creativeFactoryResourceNames(profile)
	copyLibrary, _, err := creativeFactoryResource(ctx, tx, workspaceID, userID, "copy_library", copyLibraryName, copyDefault.Description, copyDefault.Config)
	if err != nil {
		return creativeResourceResponse{}, creativeResourceResponse{}, nil, err
	}
	marketConfig := cloneCreativeFactoryConfig(marketDefault.Config)
	marketConfig["copy_library_id"] = copyLibrary.ID
	marketPack, marketCreated, err := creativeFactoryResource(ctx, tx, workspaceID, userID, "market_pack", marketPackName, marketDefault.Description, marketConfig)
	if err != nil {
		return creativeResourceResponse{}, creativeResourceResponse{}, nil, err
	}
	clonedKeys := []string{}
	if marketCreated && len(marketDefault.Files) > 0 {
		clonedKeys, err = h.cloneCreativeFactoryResourceFiles(ctx, tx, workspaceID, userID, parseUUID(marketPack.ID), marketDefault.Files)
		if err != nil {
			return creativeResourceResponse{}, creativeResourceResponse{}, nil, err
		}
	}
	if profile.Explicit {
		if err := demoteOtherCreativeFactoryMarketDefaults(ctx, tx, workspaceID, parseUUID(marketPack.ID)); err != nil {
			return creativeResourceResponse{}, creativeResourceResponse{}, nil, err
		}
	}
	return copyLibrary, marketPack, clonedKeys, nil
}

func demoteOtherCreativeFactoryMarketDefaults(ctx context.Context, tx pgx.Tx, workspaceID, defaultMarketPackID pgtype.UUID) error {
	if _, err := tx.Exec(ctx, `
UPDATE creative_resource
SET config = jsonb_set(config, '{pre_adaptation_default}', 'false'::jsonb, true),
    updated_at = now()
WHERE workspace_id = $1
  AND kind = 'market_pack'
  AND id <> $2
  AND status <> 'archived'
  AND COALESCE(config->>'pre_adaptation_default', 'false') = 'true'
`, workspaceID, defaultMarketPackID); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `
UPDATE creative_resource_revision revision
SET config = jsonb_set(revision.config, '{pre_adaptation_default}', 'false'::jsonb, true)
FROM creative_resource resource
WHERE revision.resource_id = resource.id
  AND resource.workspace_id = $1
  AND resource.kind = 'market_pack'
  AND resource.id <> $2
  AND resource.status <> 'archived'
  AND resource.published_version IS NOT NULL
  AND revision.version = resource.published_version
  AND COALESCE(revision.config->>'pre_adaptation_default', 'false') = 'true'
`, workspaceID, defaultMarketPackID)
	return err
}

func creativeFactoryResource(ctx context.Context, tx pgx.Tx, workspaceID, userID pgtype.UUID, kind, name, description string, config map[string]any) (creativeResourceResponse, bool, error) {
	var resource creativeResourceResponse
	var configRaw string
	seedPublish := creativeFactorySeedPublishAllowed(kind, config)
	err := tx.QueryRow(ctx, `
SELECT id::text, workspace_id::text, kind, name, description, status, version,
       COALESCE(published_version, 0), config::text, created_by::text,
       created_at::text, updated_at::text
FROM creative_resource
WHERE workspace_id = $1 AND kind = $2 AND name = $3 AND status <> 'archived'
ORDER BY updated_at DESC
LIMIT 1
`, workspaceID, kind, name).Scan(&resource.ID, &resource.WorkspaceID, &resource.Kind, &resource.Name, &resource.Description, &resource.Status, &resource.Version, &resource.PublishedVersion, &configRaw, &resource.CreatedBy, &resource.CreatedAt, &resource.UpdatedAt)
	if err == nil {
		resource.Config = json.RawMessage(configRaw)
		// A v1 factory seed has no user-authored revision yet. Promote that
		// exact seed in place so enabling the feature can immediately enqueue
		// pre-adaptation without overwriting later workspace edits.
		if seedPublish && resource.Status == "draft" && resource.Version == 1 && resource.PublishedVersion == 0 {
			encoded, marshalErr := json.Marshal(config)
			if marshalErr != nil {
				return creativeResourceResponse{}, false, marshalErr
			}
			if _, updateErr := tx.Exec(ctx, `
UPDATE creative_resource
SET status = 'published', published_version = version, config = $2::jsonb, updated_at = now()
WHERE id = $1
`, parseUUID(resource.ID), string(encoded)); updateErr != nil {
				return creativeResourceResponse{}, false, updateErr
			}
			if _, updateErr := tx.Exec(ctx, `
UPDATE creative_resource_revision
SET config = $2::jsonb
WHERE resource_id = $1 AND version = 1
`, parseUUID(resource.ID), string(encoded)); updateErr != nil {
				return creativeResourceResponse{}, false, updateErr
			}
			resource.Status = "published"
			resource.PublishedVersion = resource.Version
			resource.Config = json.RawMessage(encoded)
		}
		return resource, false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return creativeResourceResponse{}, false, err
	}
	encoded, err := json.Marshal(config)
	if err != nil {
		return creativeResourceResponse{}, false, err
	}
	if strings.TrimSpace(description) == "" {
		description = "工作区创意工厂资源"
		if kind == "market_pack" {
			description = "工作区市场资源包；首次开启后由工作区管理员补充或复制 Prime 附件。"
		} else if kind == "copy_library" {
			description = "工作区已审核投放文案和还款计划库。"
		}
	}
	err = tx.QueryRow(ctx, `
INSERT INTO creative_resource (workspace_id, kind, name, description, status, published_version, config, created_by)
VALUES ($1, $2, $3, $4,
        CASE WHEN $7 THEN 'published' ELSE 'draft' END,
        CASE WHEN $7 THEN 1 ELSE NULL END,
        $5::jsonb, $6)
RETURNING id::text, workspace_id::text, kind, name, description, status, version,
          COALESCE(published_version, 0), config::text, created_by::text,
          created_at::text, updated_at::text
`, workspaceID, kind, name, description, string(encoded), userID, seedPublish).Scan(&resource.ID, &resource.WorkspaceID, &resource.Kind, &resource.Name, &resource.Description, &resource.Status, &resource.Version, &resource.PublishedVersion, &configRaw, &resource.CreatedBy, &resource.CreatedAt, &resource.UpdatedAt)
	if err != nil {
		return creativeResourceResponse{}, false, err
	}
	resource.Config = json.RawMessage(configRaw)
	if _, err := tx.Exec(ctx, `
INSERT INTO creative_resource_revision (resource_id, version, name, description, config, created_by)
VALUES ($1, $2, $3, $4, $5::jsonb, $6)
`, parseUUID(resource.ID), resource.Version, resource.Name, resource.Description, string(encoded), userID); err != nil {
		return creativeResourceResponse{}, false, err
	}
	return resource, true, nil
}

func creativeFactorySeedPublishAllowed(kind string, config map[string]any) bool {
	if kind == "copy_library" {
		return true
	}
	if kind != "market_pack" {
		return false
	}
	preAdaptationDefault, ok := config["pre_adaptation_default"].(bool)
	return ok && preAdaptationDefault
}

func creativeFactoryMarketPackNeedsSetup(resource creativeResourceResponse) bool {
	var config map[string]any
	if err := json.Unmarshal(resource.Config, &config); err != nil {
		return true
	}
	seed, _ := config["pre_adaptation_seed"].(bool)
	return seed
}

func scanCreativeFactoryInstallation(row pgx.Row) (creativeFactoryInstallationRecord, error) {
	var installation creativeFactoryInstallationRecord
	var roleAgentsRaw, roleSkillsRaw, configRaw []byte
	err := row.Scan(&installation.WorkspaceID, &installation.Status, &installation.SchemaVersion, &installation.TemplateVersion, &installation.RuntimeID, &installation.MarketPackID, &installation.CopyLibraryID, &installation.SquadID, &installation.OrchestrationSkillID, &roleAgentsRaw, &roleSkillsRaw, &configRaw)
	if err != nil {
		return creativeFactoryInstallationRecord{}, err
	}
	installation.RoleAgents = map[string]string{}
	installation.RoleSkills = map[string]string{}
	installation.Config = map[string]any{}
	_ = json.Unmarshal(roleAgentsRaw, &installation.RoleAgents)
	_ = json.Unmarshal(roleSkillsRaw, &installation.RoleSkills)
	_ = json.Unmarshal(configRaw, &installation.Config)
	return installation, nil
}

func (h *Handler) creativeFactoryInstallation(ctx context.Context, workspaceID pgtype.UUID) (creativeFactoryInstallationRecord, error) {
	return scanCreativeFactoryInstallation(h.DB.QueryRow(ctx, `
SELECT workspace_id, status, schema_version, template_version,
       runtime_id, market_pack_id, copy_library_id, squad_id,
       orchestration_skill_id, role_agents, role_skills, config
FROM creative_factory_installation
WHERE workspace_id = $1
`, workspaceID))
}

func (h *Handler) creativeFactoryAgentByRole(ctx context.Context, workspaceID pgtype.UUID, role string) (db.Agent, error) {
	installation, err := h.creativeFactoryInstallation(ctx, workspaceID)
	if err != nil {
		return db.Agent{}, err
	}
	id, ok := installation.RoleAgents[role]
	if !ok || strings.TrimSpace(id) == "" {
		return db.Agent{}, pgx.ErrNoRows
	}
	agentID, err := parseUUIDString(id)
	if err != nil {
		return db.Agent{}, err
	}
	return h.Queries.GetAgentInWorkspace(ctx, db.GetAgentInWorkspaceParams{ID: agentID, WorkspaceID: workspaceID})
}
