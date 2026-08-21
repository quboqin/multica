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

const creativeFactoryTemplateVersion = 6

//go:embed creative_factory_defaults/resources.json
var creativeFactoryDefaultResourcesJSON []byte

//go:embed creative_factory_defaults/assets/*
var creativeFactoryDefaultAssets embed.FS

const (
	creativeFactoryAutopilotTitle       = "印尼竞品素材周度采集"
	creativeFactoryLegacyAutopilotTitle = "创意工厂自动化"
	creativeFactoryAutopilotDescription = `每周抓取 AppGrowing 印度尼西亚的现金贷/金融竞品素材，并创建一个可追踪的 Crawl Run。

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

const creativeFactoryDirectEditAgentInstructions = `全程使用中文。只执行 creative_direct_edit；source asset 不可覆盖，task context 的 source_revision 是上一版，revision 是平台已锁定的输出 revision。精准调整不得再次调用 variant-put 或把 revision 再加一。只下载并使用 source_asset_id/source_attachment_id 指向的同尺寸无品牌底图；若存在 annotation_guide_attachment_id，再把用户最终成图标注 brief 作为第二输入。Input 1 是唯一可编辑无品牌底图，Input 2 只用于读取红框编号、评论位置和固定贴片/标题/Logo 遮挡关系；不得复制红框、编号、Prime 组件、Logo、二维码、商店徽章或官方条款，也不得把 Prime 成图当作可编辑来源。reference_asset_id/reference_attachment_id 仅用于协作对照。

任务号只使用运行时注入的 MULTICA_TASK_ID；不得把 issue_id、adjustment_issue_id、variant_id 或 item_key 当 task_id。使用 Image Edit 返回的完整 JSON 作为 image-edit-result.json，prompt 和 prompt_sha256 只取该 JSON，prompt.txt 仅供展示且末尾换行不能参与 hash。提示词保留用户原话并追加约束：只编辑 Input 1，Input 2 仅为标注/遮挡参考，平台会重新贴回固定组件，不能把标注或 Prime 组件画进底图。人物替换必须是肉眼可见的 replacement，现有人物是移除目标，不是身份、五官、发型、服装、姿势、手势、身形轮廓或构图参考；给出具体不同的新人物属性。只处理 target_size，未修改尺寸沿用平台复制的上一 revision 底图和过程证据，不重新生成。

每一次 Image Edit 实际回图，无论采用还是拒绝，都必须上传并用 diagnostic-asset-put 登记当前尺寸过程图；拒绝回图使用直接改图尝试编号和未采用原因，不能丢失方图或失败图片。若诊断写回遇到 task ownership 错误，修正 task_id 为 MULTICA_TASK_ID；仅在直接改图诊断允许的情况下省略 task_id 重试，不得重新生成图片。只有采用的无品牌底图写入 generated/completed。asset-put 必须同时传 model-result-file、prompt-contract-file、copy-validation-file、normalization-evidence-file；edited-asset.json 不得带 metadata/evidence，CLI 会从四份证据生成它们。出现协议、归属、prompt/hash、证据或上传错误时，复用同一回图和同一模型 JSON 修复后重试；只有没有有效回图或真实视觉失败才执行每尺寸最多一次的模型重试。

最后一个 expected size 写回后调用绑定的素材_技能_贴片，由后端重新贴回官方透明组件并登记 primed/delivered；随后回读订单确认所有尺寸和过程图完整。只有回读成功才允许调用 task complete。不创建贴片 task，不创建 QC task，不调用 QC。不得触发采集、分析、方案或标准生产。`

func creativeFactoryImageEditAgentInstructions() string {
	return `全程使用中文。根据 task context.workflow 选择唯一执行分支。

creative_production 分支只执行 creative_production；使用冻结 brief/copy_snapshot、实际 Prime context 和无品牌来源，严格按生产 Skill 的 GPT Image 2 模板写每个尺寸的 prompt：先写 COMPOSITION GATE，再写明确的 Input 1/Input 2 角色；横版拥挤时沿 Y 轴压缩留白、模块间距和行距，不删 approved copy 或表格结构。写 generated assets、lineage 与 CLI 原始模型证据，只补当前 revision 的缺失尺寸。保持 prompt 1800-2800 字符、无坐标、无审计重复，并记录 prompt/hash；prompt-contract 必须带当前 brief 的 parent_direction_sha256。调用 multica image edit/edit-batch 时必须给 Bash 工具设置 timeout_ms 至少 1500000（25 分钟），等待 CLI 返回完整 JSON 后，立即按生产 Skill 调用 register_process_assets.py 登记当前尺寸的 Prime context、模型原图和规范化底图，并回读订单确认 registered 数量和归属。遇到 qc_visual_rework 时只对失败尺寸执行一次 Image2 重排，仍失败就写 action_required。比例重试仍失败时保留最后回图并按 Skill 的 aspect fallback 规则归一，不得丢弃尺寸。creative_production 分支不得处理 direct_edit、品牌组件、QC、采集或分析；只有当前 revision 的全部 expected_sizes 写回 generated/completed、过程证据登记回读成功且贴片 Skill 返回后端合成完成后才调用 task complete，缺尺寸不能提前完成，平台会自动创建有上限的 fresh continuation。调用绑定的素材_技能_贴片执行后端唯一 Prime 合成入口，不得直接创建 primed 资产或伪造贴片结果。

creative_direct_edit 分支严格执行现有素材_技能_改图契约：
` + creativeFactoryDirectEditAgentInstructions + `

两个分支都只处理 task context 指定的对象、revision 和 scope；按绑定 Skill 写结构化领域结果和机器证据。不得创建或修改 Issue，不得用评论代替领域数据。输入、凭证、工具或写回失败时保留已成功对象，写真实 error_code/error_message 并让当前 task 失败；兄弟对象继续。`
}

var creativeFactorySkillSpecs = []creativeFactorySkillSpec{
	{Role: "collection", Name: "素材_技能_采集", Aliases: []string{"AppGrowing 素材采集"}, Directory: "appgrowing-material-collector", Description: "创建 Crawl Run，只采集真实图片广告，平台最多采集 2 张，并用原生 task fanout 自动预分析新增图片。", Capability: "material_collection", Version: 16},
	{Role: "diagnostics", Name: "素材_技能_诊断", Aliases: []string{"创意流程诊断", "出图诊断", "AppGrowing 采集诊断"}, Directory: "creative-flow-diagnostician", Description: "读取创意采集、出图、品牌组件、QC、订单和 daemon/runtime 证据，在允许范围内恢复或给出明确动作。", Capability: "crawl_diagnosis", Version: 5},
	{Role: "reference_analysis", Name: "素材_技能_分析", Aliases: []string{"广告参考分析"}, Directory: "ad-creative-analysis", Description: "市场中立地读取真实图片，识别可变视觉区域、原图文字及坐标、主题、利益点、语义锚点、App UI 类型和布局约束；numeric 区域只包含可重排还款字段，混合区域必须拆分。", Capability: "reference_analysis", Version: 17},
	{Role: "pre_adaptation", Name: "素材_技能_文案适配", Aliases: []string{"广告预适配"}, Directory: "ad-creative-pre-adaptation", Description: "按冻结资源完成可生产文案与数值适配；我方已审核还款方案优先，按我方可用数量落表，超出的原图数值默认移除，不借用其他期限金额；数值布局说明必须列出每个冻结展示值。", Capability: "pre_adaptation", Version: 25},
	{Role: "generation_plan", Name: "素材_技能_方案", Aliases: []string{"广告生成方案"}, Directory: "ad-creative-plan", Description: "消费冻结分析、逐块文案与市场快照，严格继承顶层非空文案字段，规划 3 个同题变体及品牌组件视觉关系。", Capability: "generation_plan", Version: 34},
	{Role: "image_edit", Name: "素材_技能_出图", Aliases: []string{"广告图像编辑"}, Directory: "ad-creative-production", Description: "使用 GPT Image 2 提示词模板和冻结业务结构生成无品牌三尺寸底图，严格继承所有非空 approved copy，明确 Input 1/Input 2 角色，候选图通过 candidate_id 受控下载，Prime context 按当前 revision 绑定，横版用 Y 轴压缩避开上下 Prime 组件带，provider 画布使用 16px 合法尺寸再归一化为交付尺寸，使用 canonical generated asset 写回并保存完整 trace 与 parent_direction_sha256；每个尺寸同步登记 Prime context、模型原图和规范化底图，比例异常保留一次压缩归一化，视觉遮挡最多执行一次定向 Image2 重排；缺尺寸不提前 complete，由平台自动续跑，完成底图后调用贴片 Skill。", Capability: "image_edit", Version: 87},
	{Role: "prime_compose", Name: "素材_技能_贴片", Aliases: []string{"广告品牌组件合成"}, Directory: "ad-creative-prime-compose", Description: "调用后端唯一的确定性 Prime 合成入口，校验合成 JSON，并由后端登记贴片完成过程图、primed 资产和标准 QC/交付交接；不创建 Prime Agent 或 Prime task。", Capability: "prime_compose", Version: 2},
	{Role: "direct_image_edit", Name: "素材_技能_改图", Aliases: []string{"广告图片直接修改"}, Directory: "ad-creative-direct-edit", Description: "按用户原话和最终图标注 brief 修改固定无品牌底图，再由贴片 Skill 调用平台确定性合成并直接交付，不执行 QC；协议错误复用同一回图修复写回。", Capability: "direct_image_edit", Version: 13},
	{Role: "quality_control", Name: "素材_技能_质检", Aliases: []string{"广告成图验收"}, Directory: "ad-creative-qc", Description: "独立执行 technical 或 visual QC，只记录成图检测与调整建议；证据契约失败由平台自动复用 Prime 资产重跑双 QC。", Capability: "quality_control", Version: 31},
	{Role: "creative_leadership", Name: "素材_技能_流程", Aliases: []string{"素材_技能_统筹", "创意素材协作", "素材小队 Leader 编排"}, Directory: "ad-creative-leadership", Description: "使用原生 task fanout 启动并恢复标准生产或直接改图，汇总结构化结果。", Capability: "creative_leadership", Version: 49},
}

var creativeFactoryAgentSpecs = []creativeFactoryAgentSpec{
	{Role: "leadership", Name: "素材_流程", Aliases: []string{"素材_统筹", "素材小队 Leader"}, Description: "按冻结能力映射启动和恢复 Creative Order，并负责用户汇总。", Instructions: "全程使用中文。只执行判断、原生 fanout、异常恢复和用户汇总，不代替专业角色。标准订单只创建方案 task；direct_edit 只创建直接修改 task；正常下游由各阶段唯一 owner 续链。每次唤醒回读订单、task 与冻结 squad snapshot，按 target/source/item_key 只补真正缺失项，一次提交后立即结束。不得按名称猜 Agent，不轮询，不创建阶段子 Issue；Issue 只记录人工决定、真实阻塞和最终验收。", SkillRoles: []string{"creative_leadership"}, Model: "gpt-5.6-terra", ThinkingLevel: "medium", MaxConcurrent: 6},
	{Role: "reference_analysis", Name: "素材_分析", Aliases: []string{"广告参考分析智能体"}, Description: "按 task workflow 读取真实像素、识别可变视觉区域、写市场中立分析，并在后台完成可确认的文案与数值预适配。", Instructions: "全程使用中文。creative_reference_analysis 只写指定 candidate/version 的市场中立 Source Analysis；每个可变原图文字区块必须归入唯一 copy 或 numeric 视觉区域，不能把同一画面组件拆入两条处理路径。creative_pre_adaptation 只消费指定的冻结市场包和文案库，逐区域优先绑定已审核内容；没有兼容已审核片段的普通 headline、subheadline、benefit、supporting 或 cta 区块，必须按市场语言、区块职责、原图语义和可读长度生成一个新的 recommended 文案，source_keys 必须为空数组，recommendation_basis 必须说明依据，页面会标记待用户确认；不得引用不存在的 fragment key。本金、期限、月供、总利息、总还款、利率、法律文字和品牌事实没有审核来源或冻结计算时不得凭空生成，逐项写 missing replacement。多行数值表不要求原图行数与我方方案数量相等：按表格语义和期限从冻结 approved repayment plan 取我方兼容方案，实际渲染行数取原图可渲染行数与我方可用方案数的较小值；每个实际渲染行写 numeric_layouts，且每个 layout 的 render_instruction 必须逐字列出其 scenario_ids 对应 selection.values 中每个 target_columns 的完整冻结展示值，不能只写按行展示；源图多出的数值块逐项写空 missing 作为默认移除项。没有我方某一期限方案时不得借用其他期限金额；整体重构为我方支持的期限列，或让该列源块留空移除。不能把整表硬塞进一个 layout，也不能伪装成已绑定或改写原图事实。输入和产物不得混用，不生成图片，不修改市场包或文案库。只处理 task context 指定的对象、revision 和 scope；按绑定 Skill 写结构化领域结果和机器证据。不得创建或修改 Issue，不得用评论代替领域数据。输入、凭证、工具或写回失败时保留已成功对象，写真实 error_code/error_message 并让当前 task 失败；兄弟对象继续。", SkillRoles: []string{"reference_analysis", "pre_adaptation"}, Model: "gpt-5.6-terra", ThinkingLevel: "medium", MaxConcurrent: 6},
	{Role: "collection", Name: "素材_采集", Aliases: []string{"AppGrowing 素材采集智能体"}, Description: "按 task 配置创建 Crawl Run，只导入真实图片广告并委派新增图片分析。", Instructions: "全程使用中文。只执行 task context 和 AutoPilot 明确的 AppGrowing 查询；只导入 asset_type=image，视频、非图片和未知类型不占用采集配额。使用注入的 analysis_agent_id=%s，不得按名称猜测。筛选、分页、预算和 fallback 由 task/平台配置决定。结果、证据和失败写 Crawl Run；导入后用原生 fanout 委派本次新增图片，不创建 Issue，不使用测试数据。", SkillRoles: []string{"collection"}, Model: "gpt-5.6-luna", ThinkingLevel: "low", MaxConcurrent: 1},
	{Role: "generation_plan", Name: "素材_方案", Aliases: []string{"生成方案智能体"}, Description: "消费冻结分析、文案与市场快照，写 3 个同题 Variant 并委派生产。", Instructions: "全程使用中文。只执行 creative_plan。copy_snapshot 与 market snapshot 是唯一文案、事实和资源真值；不得重选或改写。写 V01-V03 结构化 brief，保留语义与主体，只改变表达；将缺失 production items 一次 fanout。需要输入时写 needs_input/action_required，不生成图片。只处理 task context 指定的对象、revision 和 scope；按绑定 Skill 写结构化领域结果和机器证据。不得创建或修改 Issue，不得用评论代替领域数据。输入、凭证、工具或写回失败时保留已成功对象，写真实 error_code/error_message 并让当前 task 失败；兄弟对象继续。", SkillRoles: []string{"generation_plan"}, Model: "gpt-5.6-terra", ThinkingLevel: "medium", MaxConcurrent: 6},
	{Role: "image_edit", Name: "素材_出图", Aliases: []string{"图像编辑智能体"}, Description: "按 GPT Image 2 提示词模板执行标准出图或用户标注精准改图；分别生成或修改无品牌底图，调用贴片 Skill 完成后端合成，标准出图进入 QC，精准改图直接交付。", Instructions: creativeFactoryImageEditAgentInstructions(), SkillRoles: []string{"image_edit", "direct_image_edit", "prime_compose"}, Model: "gpt-5.6-terra", ThinkingLevel: "low", MaxConcurrent: 10},
	{Role: "quality_control", Name: "素材_质检", Aliases: []string{"广告验收智能体"}, Description: "独立执行一个 technical 或 visual lane，按三道闸门验收成图并输出尺寸级阻断或建议；证据契约错误由平台自动恢复双 QC。", Instructions: "全程使用中文。只执行 context 指定 QC lane，读取同 Variant/revision/expected_sizes 的完整品牌组件包。technical 检查文件、尺寸、品牌组件、模板布局和机器可见性证据；visual 必须逐张检查冻结文案和关键组件是否完整、顶部和底部 Prime 是否遮挡正文、Logo/条款是否可读，以及中部是否出现空框或内容缺失，并核对 generated evidence 的 parent_direction_sha256 与当前 brief 一致。写独立 QC Report 后调用 qc-finalize 完成归档。对 actual_prime_obstruction、official_prime_text_unreadable、generated_content_missing 写 failed 和每个失败尺寸一个 blocking_failure；证据缺失、manifest/compose 不匹配等 delegation/contract 错误也要写结构化 blocking_failure，平台会自动复用已完成 Prime 资产重跑 technical 和 visual 一次。其他发现写 warning。服务端仅对真实 Prime 遮挡或官方 Prime 文字不可读最多自动返工当前 Variant 一轮，不能改 Prime，不能影响兄弟 Variant；预测遮挡和关键内容缺失保留为人工阻断。只处理 task context 指定的对象、revision 和 scope；按绑定 Skill 写结构化领域结果和机器证据。不得创建或修改 Issue，不得用评论代替领域数据。输入、凭证、工具或写回失败时保留已成功对象，写真实 error_code/error_message 并让当前 task 失败；兄弟对象继续。", SkillRoles: []string{"quality_control"}, Model: "gpt-5.6-terra", ThinkingLevel: "medium", MaxConcurrent: 6},
	{Role: "diagnostics", Name: "素材_诊断", Aliases: []string{"创意流程诊断智能体", "出图诊断智能体", "AppGrowing 采集诊断智能体"}, Description: "诊断创意采集、出图、品牌组件、QC、订单状态和 daemon/runtime 异常，并通过平台入口执行受控恢复。", Instructions: "全程使用中文。处理 creative_crawl_diagnosis、订单短 ID、Variant 标签、页面卡片文案、报错文本和用户明确指向的创意流程诊断。先定位当前订单、order item、Variant、revision、task、daemon/runtime 与 Skill 快照证据，再给结论；需要恢复时只通过 multica CLI 或平台 API 重试、取消、fanout、推进明确授权的 Variant revision 或调用现有修复入口。不得直接写 DB、修改凭证、业务筛选、市场包或生产代码；只有用户明确要求维护文案库时，才可先回读文案库并使用 multica creative copy-library 的 add-fragment、update-fragment、upsert-repayment-plan 保存草稿；只有用户明确要求发布时才加 --publish。不得把诊断图当成交付资产；修改前说明对象和原因，修改后回读验证。", SkillRoles: []string{"diagnostics"}, Model: "gpt-5.6-luna", ThinkingLevel: "medium", MaxConcurrent: 2},
}

func (h *Handler) initializeCreativeFactory(ctx context.Context, workspaceID, userID pgtype.UUID) (creativeFactoryInstallationRecord, error) {
	if !workspaceID.Valid || !userID.Valid {
		return creativeFactoryInstallationRecord{}, errors.New("workspace and user are required")
	}
	templates, err := loadCreativeFactoryTemplates()
	if err != nil {
		return creativeFactoryInstallationRecord{}, err
	}
	resourceDefaults, err := creativeFactoryResourceDefaults()
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
	if existing, err := scanCreativeFactoryInstallation(tx.QueryRow(ctx, `
SELECT workspace_id, status, schema_version, template_version,
       runtime_id, market_pack_id, copy_library_id, squad_id,
       orchestration_skill_id, role_agents, role_skills, config
FROM creative_factory_installation
WHERE workspace_id = $1
FOR UPDATE
	`, workspaceID)); err == nil && existing.Status == "ready" {
		qtx := h.Queries.WithTx(tx)
		if err := h.syncCreativeFactoryManagedAssets(ctx, tx, qtx, workspaceID, userID, &existing, templates); err != nil {
			return creativeFactoryInstallationRecord{}, err
		}
		collectionAgentID, agentIDErr := creativeFactoryRoleAgentID(existing, "collection")
		if agentIDErr != nil {
			return creativeFactoryInstallationRecord{}, agentIDErr
		}
		autopilot, autopilotErr := h.creativeFactoryAutopilot(ctx, tx, qtx, workspaceID, userID, collectionAgentID, existing.SquadID)
		if autopilotErr != nil {
			return creativeFactoryInstallationRecord{}, autopilotErr
		}
		autopilotID := uuidToString(autopilot.ID)
		if existing.Config == nil {
			existing.Config = map[string]any{}
		}
		if existing.Config["autopilot_id"] != autopilotID {
			existing.Config["autopilot_id"] = autopilotID
		}
		existing.Config["template_version"] = creativeFactoryTemplateVersion
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
    updated_at = now()
WHERE workspace_id = $1
`, workspaceID, creativeFactoryTemplateVersion, existing.OrchestrationSkillID, string(roleAgentsJSON), string(roleSkillsJSON), string(configJSON)); updateErr != nil {
			return creativeFactoryInstallationRecord{}, updateErr
		}
		if commitErr := tx.Commit(ctx); commitErr != nil {
			return creativeFactoryInstallationRecord{}, commitErr
		}
		return existing, nil
	} else if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return creativeFactoryInstallationRecord{}, err
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
	autopilot, err := h.creativeFactoryAutopilot(ctx, tx, qtx, workspaceID, userID, agentIDs["collection"], squad.ID)
	if err != nil {
		return creativeFactoryInstallationRecord{}, err
	}

	copyDefault := resourceDefaults["copy_library"]
	copyLibrary, _, err := creativeFactoryResource(ctx, tx, workspaceID, userID, "copy_library", "AdaKami Indonesia 文案库", copyDefault.Description, copyDefault.Config)
	if err != nil {
		return creativeFactoryInstallationRecord{}, err
	}
	marketDefault := resourceDefaults["market_pack"]
	marketConfig := cloneCreativeFactoryConfig(marketDefault.Config)
	marketConfig["copy_library_id"] = copyLibrary.ID
	marketPack, marketCreated, err := creativeFactoryResource(ctx, tx, workspaceID, userID, "market_pack", "AdaKami Indonesia 市场资源包", marketDefault.Description, marketConfig)
	if err != nil {
		return creativeFactoryInstallationRecord{}, err
	}
	if marketCreated && len(marketDefault.Files) > 0 {
		var clonedKeys []string
		clonedKeys, err = h.cloneCreativeFactoryResourceFiles(ctx, tx, workspaceID, userID, parseUUID(marketPack.ID), marketDefault.Files)
		if err != nil {
			return creativeFactoryInstallationRecord{}, err
		}
		uploadedDefaultObjectKeys = append(uploadedDefaultObjectKeys, clonedKeys...)
	}

	roleAgentsJSON, _ := json.Marshal(roleAgents)
	roleSkillsJSON, _ := json.Marshal(roleSkills)
	configJSON, _ := json.Marshal(map[string]any{
		"origin":            "creative_factory",
		"template_version":  creativeFactoryTemplateVersion,
		"resource_policy":   "clone_ad_creative_published_defaults_adopt_existing",
		"user_edits_policy": "never_overwrite",
		"autopilot_id":      uuidToString(autopilot.ID),
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

func (h *Handler) creativeFactoryAutopilot(ctx context.Context, tx pgx.Tx, qtx *db.Queries, workspaceID, userID, collectionAgentID, legacySquadID pgtype.UUID) (db.Autopilot, error) {
	var autopilotID pgtype.UUID
	err := tx.QueryRow(ctx, `
SELECT id
FROM autopilot
WHERE workspace_id = $1 AND title = ANY($2::text[])
ORDER BY CASE WHEN title = $3 THEN 0 ELSE 1 END, created_at ASC
LIMIT 1
`, workspaceID, []string{creativeFactoryAutopilotTitle, creativeFactoryLegacyAutopilotTitle}, creativeFactoryAutopilotTitle).Scan(&autopilotID)
	if err == nil {
		autopilot, getErr := qtx.GetAutopilotInWorkspace(ctx, db.GetAutopilotInWorkspaceParams{ID: autopilotID, WorkspaceID: workspaceID})
		if getErr != nil {
			return db.Autopilot{}, getErr
		}
		if autopilot.Title == creativeFactoryAutopilotTitle {
			return autopilot, nil
		}
		if autopilot.Title == creativeFactoryLegacyAutopilotTitle && creativeFactoryAutopilotNeedsMigration(autopilot, legacySquadID) {
			updated, updateErr := qtx.UpdateAutopilot(ctx, db.UpdateAutopilotParams{
				ID:                 autopilot.ID,
				Title:              pgtype.Text{String: creativeFactoryAutopilotTitle, Valid: true},
				Description:        pgtype.Text{String: creativeFactoryAutopilotDescription, Valid: true},
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
		Title:         creativeFactoryAutopilotTitle,
		Description:   pgtype.Text{String: creativeFactoryAutopilotDescription, Valid: true},
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
					if entry.Name() == "__pycache__" {
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
