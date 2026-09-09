package handler

import (
	"context"
	"fmt"
	"strings"
	"unicode"

	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

type squadWorkflowTemplate struct {
	profile  string
	keywords []string
	stages   []squadWorkflowTemplateStage
}

type squadWorkflowTemplateStage struct {
	id            string
	nameZH        string
	nameEN        string
	descriptionZH string
	descriptionEN string
	keywords      []string
}

type squadWorkflowAgent struct {
	id           pgtype.UUID
	name         string
	role         string
	description  string
	instructions string
}

type workflowWeightedText struct {
	value  string
	weight int
}

var squadWorkflowTemplates = []squadWorkflowTemplate{
	{
		profile:  "software",
		keywords: []string{"研发", "开发", "代码", "技术", "software", "engineering", "developer", "tech"},
		stages: []squadWorkflowTemplateStage{
			{id: "requirements", nameZH: "需求与范围", nameEN: "Requirements and scope", descriptionZH: "澄清目标、边界、约束和验收标准", descriptionEN: "Clarify goals, boundaries, constraints, and acceptance criteria", keywords: []string{"需求", "prd", "requirement", "业务分析", "验收"}},
			{id: "research", nameZH: "调研与方案", nameEN: "Research and design", descriptionZH: "调研现状并形成可执行的技术方案", descriptionEN: "Inspect the current system and define an executable solution", keywords: []string{"调研", "排查", "分析", "设计", "方案", "架构", "research", "design", "architecture"}},
			{id: "implementation", nameZH: "实现与集成", nameEN: "Implementation and integration", descriptionZH: "完成代码、配置、依赖和集成改动", descriptionEN: "Implement code, configuration, dependencies, and integrations", keywords: []string{"代码", "实现", "开发", "编程", "coding", "implementation", "developer"}},
			{id: "review", nameZH: "评审与验证", nameEN: "Review and verification", descriptionZH: "检查质量、风险并完成测试与修复", descriptionEN: "Review quality and risk, then test and fix the result", keywords: []string{"评审", "审核", "测试", "修复", "回归", "review", "qa", "test", "bug"}},
			{id: "delivery", nameZH: "交付与发布", nameEN: "Delivery and release", descriptionZH: "完成提测、外部工单、发布和结果同步", descriptionEN: "Handle test handoff, external actions, release, and reporting", keywords: []string{"提测", "交付", "工单", "发布", "上线", "deploy", "delivery", "release", "submit", "ticket"}},
			{id: "knowledge", nameZH: "知识与持续改进", nameEN: "Knowledge and improvement", descriptionZH: "查询规范、沉淀经验并支持后续迭代", descriptionEN: "Query standards, retain lessons, and support future iterations", keywords: []string{"知识", "规范", "文档", "复盘", "knowledge", "standard", "documentation"}},
		},
	},
	{
		profile:  "product",
		keywords: []string{"产品", "用户研究", "需求方案", "product", "prd", "discovery"},
		stages: []squadWorkflowTemplateStage{
			{id: "discovery", nameZH: "目标与机会", nameEN: "Goals and opportunities", descriptionZH: "识别业务目标、用户问题和机会优先级", descriptionEN: "Identify business goals, user problems, and opportunity priorities", keywords: []string{"目标", "机会", "用户研究", "discovery", "research"}},
			{id: "requirements", nameZH: "需求与方案", nameEN: "Requirements and solution", descriptionZH: "形成需求范围、产品方案和验收标准", descriptionEN: "Define scope, product solution, and acceptance criteria", keywords: []string{"需求", "方案", "prd", "用户故事", "验收", "requirement"}},
			{id: "domain", nameZH: "业务模块协同", nameEN: "Domain collaboration", descriptionZH: "由各产品模块补充业务规则并协同落地", descriptionEN: "Coordinate domain rules and implementation across product areas", keywords: []string{"贷前", "贷中", "贷后", "客服", "模块", "业务", "domain"}},
			{id: "experiment", nameZH: "指标与实验", nameEN: "Metrics and experiments", descriptionZH: "定义指标口径、埋点、实验和效果判断", descriptionEN: "Define metrics, instrumentation, experiments, and outcome checks", keywords: []string{"指标", "埋点", "实验", "看板", "漏斗", "abtest", "metric", "experiment"}},
			{id: "compliance", nameZH: "合规与验收", nameEN: "Compliance and acceptance", descriptionZH: "完成合规预审、风险检查和结果验收", descriptionEN: "Complete compliance review, risk checks, and acceptance", keywords: []string{"合规", "审核", "风险", "验收", "compliance", "acceptance"}},
			{id: "knowledge", nameZH: "知识更新", nameEN: "Knowledge update", descriptionZH: "沉淀稳定产品事实、决策和复盘结论", descriptionEN: "Retain stable product facts, decisions, and review outcomes", keywords: []string{"知识", "知识库", "更新", "复盘", "knowledge"}},
		},
	},
	{
		profile:  "data_model",
		keywords: []string{"数据", "模型", "算法", "数仓", "sql", "data", "model", "analytics", "bi"},
		stages: []squadWorkflowTemplateStage{
			{id: "goals", nameZH: "目标与口径", nameEN: "Goals and definitions", descriptionZH: "明确分析或建模目标、样本范围和指标口径", descriptionEN: "Define analysis or modeling goals, sample scope, and metrics", keywords: []string{"目标", "口径", "指标", "顾问", "advisor", "metric"}},
			{id: "data", nameZH: "数据准备", nameEN: "Data preparation", descriptionZH: "完成数据获取、清洗、质量检查和样本构建", descriptionEN: "Acquire, clean, validate, and prepare datasets", keywords: []string{"数据", "数仓", "取数", "清洗", "样本", "sql", "data"}},
			{id: "features", nameZH: "变量与特征", nameEN: "Variables and features", descriptionZH: "完成变量衍生、筛选和特征验证", descriptionEN: "Derive, select, and validate variables and features", keywords: []string{"变量", "特征", "衍生", "feature", "variable"}},
			{id: "modeling", nameZH: "分析与建模", nameEN: "Analysis and modeling", descriptionZH: "执行分析、训练模型并解释关键结果", descriptionEN: "Run analysis, train models, and interpret key results", keywords: []string{"模型", "建模", "算法", "分析", "model", "analysis"}},
			{id: "validation", nameZH: "验证与评审", nameEN: "Validation and review", descriptionZH: "验证准确性、稳定性、偏差和业务适用性", descriptionEN: "Validate accuracy, stability, bias, and business fitness", keywords: []string{"验证", "评审", "校验", "质量", "validation", "review", "qa"}},
			{id: "monitoring", nameZH: "监控与报告", nameEN: "Monitoring and reporting", descriptionZH: "建设监控预警、报告并推动业务行动", descriptionEN: "Build monitoring, alerts, reports, and business actions", keywords: []string{"监控", "预警", "看板", "报告", "monitor", "dashboard", "report"}},
		},
	},
	{
		profile:  "risk",
		keywords: []string{"风险", "风控", "策略", "决策", "drools", "risk", "policy", "decision"},
		stages: []squadWorkflowTemplateStage{
			{id: "intake", nameZH: "风险问题接入", nameEN: "Risk intake", descriptionZH: "明确风险场景、目标、约束和决策边界", descriptionEN: "Define the risk scenario, goal, constraints, and decision boundary", keywords: []string{"负责人", "接入", "路由", "intake", "leader"}},
			{id: "evidence", nameZH: "数据与证据", nameEN: "Data and evidence", descriptionZH: "收集数据、业务事实和历史策略证据", descriptionEN: "Collect data, business facts, and historical policy evidence", keywords: []string{"数据", "取数", "sql", "证据", "data"}},
			{id: "analysis", nameZH: "风险分析", nameEN: "Risk analysis", descriptionZH: "分析风险模式、影响范围和候选策略", descriptionEN: "Analyze risk patterns, impact, and candidate policies", keywords: []string{"顾问", "分析", "模型", "市场", "产品", "advisor", "analysis", "model"}},
			{id: "policy", nameZH: "策略与规则实现", nameEN: "Policy implementation", descriptionZH: "编写策略、规则、SQL 或执行代码", descriptionEN: "Implement policies, rules, SQL, or execution code", keywords: []string{"策略", "规则", "drl", "drools", "代码", "编写", "policy", "code"}},
			{id: "validation", nameZH: "审查与验证", nameEN: "Review and validation", descriptionZH: "审查策略正确性、合规性和潜在副作用", descriptionEN: "Review policy correctness, compliance, and side effects", keywords: []string{"审查", "验证", "合规", "review", "validation", "compliance"}},
			{id: "monitoring", nameZH: "监控与复盘", nameEN: "Monitoring and review", descriptionZH: "跟踪变量与策略效果，形成预警和迭代结论", descriptionEN: "Track variables and policy outcomes, then produce alerts and iteration findings", keywords: []string{"监控", "预警", "复盘", "monitor", "alert"}},
		},
	},
	{
		profile:  "marketing",
		keywords: []string{"市场", "营销", "投放", "广告", "增长", "运营", "marketing", "ads", "campaign", "growth"},
		stages: []squadWorkflowTemplateStage{
			{id: "strategy", nameZH: "目标与策略", nameEN: "Goals and strategy", descriptionZH: "明确受众、增长目标、预算和渠道策略", descriptionEN: "Define audience, growth goals, budget, and channel strategy", keywords: []string{"负责人", "策略", "目标", "预算", "strategy", "leader"}},
			{id: "research", nameZH: "市场与渠道洞察", nameEN: "Market and channel insight", descriptionZH: "分析用户、竞品、关键词和渠道机会", descriptionEN: "Analyze users, competitors, keywords, and channel opportunities", keywords: []string{"竞品", "调研", "aso", "关键词", "adspy", "research", "insight"}},
			{id: "creative", nameZH: "内容与素材", nameEN: "Content and creative", descriptionZH: "策划并制作广告、社媒和商店素材", descriptionEN: "Plan and produce ads, social content, and store assets", keywords: []string{"素材", "内容", "文案", "社媒", "creative", "content"}},
			{id: "acquisition", nameZH: "渠道投放", nameEN: "Channel acquisition", descriptionZH: "执行并优化各广告平台和渠道投放", descriptionEN: "Execute and optimize advertising platforms and acquisition channels", keywords: []string{"投放", "google", "meta", "tiktok", "asa", "网盟", "ads", "affiliate"}},
			{id: "growth", nameZH: "产品增长运营", nameEN: "Product growth", descriptionZH: "优化激活、转化、留存和用户旅程", descriptionEN: "Improve activation, conversion, retention, and user journeys", keywords: []string{"增长", "新客", "app", "激活", "留存", "运营", "growth", "retention"}},
			{id: "review", nameZH: "数据复盘与迭代", nameEN: "Performance review", descriptionZH: "评估投放与内容效果并形成下一轮动作", descriptionEN: "Evaluate campaign and content outcomes and define next actions", keywords: []string{"分析", "复盘", "归因", "效果", "analysis", "review", "attribution"}},
		},
	},
	{
		profile:  "support",
		keywords: []string{"客服", "支持", "工单", "投诉", "服务", "support", "service", "ticket"},
		stages: []squadWorkflowTemplateStage{
			{id: "intake", nameZH: "请求接入", nameEN: "Request intake", descriptionZH: "识别用户诉求、优先级和服务边界", descriptionEN: "Identify the request, priority, and service boundary", keywords: []string{"接入", "分派", "客服", "intake", "triage"}},
			{id: "diagnosis", nameZH: "问题诊断", nameEN: "Diagnosis", descriptionZH: "补齐上下文并定位问题和影响范围", descriptionEN: "Gather context and diagnose the issue and impact", keywords: []string{"诊断", "排查", "分析", "diagnosis", "analysis"}},
			{id: "resolution", nameZH: "方案处理", nameEN: "Resolution", descriptionZH: "执行解决方案、协调资源并同步进展", descriptionEN: "Execute the resolution, coordinate resources, and report progress", keywords: []string{"处理", "解决", "执行", "resolution", "action"}},
			{id: "quality", nameZH: "质量确认", nameEN: "Quality confirmation", descriptionZH: "确认结果、SLA、用户满意度和遗留风险", descriptionEN: "Confirm outcomes, SLA, user satisfaction, and residual risk", keywords: []string{"质检", "sla", "验收", "质量", "quality"}},
			{id: "knowledge", nameZH: "反馈与沉淀", nameEN: "Feedback and knowledge", descriptionZH: "沉淀解决方案、反馈和可复用服务知识", descriptionEN: "Retain resolutions, feedback, and reusable service knowledge", keywords: []string{"反馈", "知识", "复盘", "knowledge", "feedback"}},
		},
	},
	{
		profile:  "sales",
		keywords: []string{"商务", "销售", "客户成功", "商机", "bd", "sales", "business development"},
		stages: []squadWorkflowTemplateStage{
			{id: "leads", nameZH: "线索识别", nameEN: "Lead qualification", descriptionZH: "识别目标客户、线索质量和跟进优先级", descriptionEN: "Identify target accounts, lead quality, and follow-up priority", keywords: []string{"线索", "客户", "lead", "account"}},
			{id: "discovery", nameZH: "需求诊断", nameEN: "Needs discovery", descriptionZH: "澄清客户目标、痛点、预算和决策链", descriptionEN: "Clarify goals, pain points, budget, and decision process", keywords: []string{"需求", "诊断", "调研", "discovery"}},
			{id: "proposal", nameZH: "方案与报价", nameEN: "Proposal and pricing", descriptionZH: "形成解决方案、商务条款和价值说明", descriptionEN: "Prepare the solution, commercial terms, and value case", keywords: []string{"方案", "报价", "合同", "proposal", "pricing"}},
			{id: "closing", nameZH: "成交推进", nameEN: "Closing", descriptionZH: "处理异议、审批和签约动作", descriptionEN: "Handle objections, approvals, and contracting", keywords: []string{"成交", "签约", "审批", "closing", "contract"}},
			{id: "success", nameZH: "交付与客户成功", nameEN: "Delivery and customer success", descriptionZH: "完成交付、回访、续约和增长机会识别", descriptionEN: "Deliver, follow up, renew, and identify growth opportunities", keywords: []string{"交付", "回访", "续约", "客户成功", "delivery", "success"}},
		},
	},
}

var generalSquadWorkflowTemplate = squadWorkflowTemplate{
	profile: "general",
	stages: []squadWorkflowTemplateStage{
		{id: "requirements", nameZH: "需求与范围", nameEN: "Requirements and scope", descriptionZH: "澄清目标、边界、约束和完成标准", descriptionEN: "Clarify goals, scope, constraints, and completion criteria", keywords: []string{"目标", "需求", "范围", "验收", "业务分析", "goal", "requirement", "scope", "acceptance"}},
		{id: "research", nameZH: "调研与方案", nameEN: "Research and planning", descriptionZH: "调研现状、分析可行性并形成执行方案", descriptionEN: "Research the current state, analyze feasibility, and define a plan", keywords: []string{"调研", "排查", "分析", "可行性", "方案", "规划", "设计", "架构", "research", "analysis", "feasibility", "plan", "design", "architecture"}},
		{id: "execution", nameZH: "业务执行", nameEN: "Business execution", descriptionZH: "完成专业处理、业务操作或实现工作", descriptionEN: "Perform specialist handling, business operations, or implementation work", keywords: []string{"执行", "处理", "实现", "操作", "运营", "催收", "客服", "服务", "operation", "execution", "implementation", "support", "service"}},
		{id: "review", nameZH: "评审与验证", nameEN: "Review and verification", descriptionZH: "检查结果质量、准确性和潜在风险", descriptionEN: "Review result quality, accuracy, and potential risk", keywords: []string{"检查", "审核", "评审", "验证", "质量", "测试", "修复", "review", "quality", "test", "validation", "qa"}},
		{id: "delivery", nameZH: "交付与同步", nameEN: "Delivery and reporting", descriptionZH: "交付结果、发布内容并同步处理结论", descriptionEN: "Deliver results, publish outputs, and report outcomes", keywords: []string{"交付", "发布", "上线", "提测", "同步", "工单", "delivery", "release", "deploy", "submit", "ticket"}},
		{id: "knowledge", nameZH: "知识与支持", nameEN: "Knowledge and support", descriptionZH: "提供知识问答、规范查询和经验沉淀", descriptionEN: "Provide knowledge support, standards, and reusable guidance", keywords: []string{"知识", "知识库", "问答", "规范", "文档", "复盘", "顾问", "knowledge", "documentation", "standard", "advisor"}},
	},
}

func containsHan(value string) bool {
	for _, r := range value {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

func workflowTextScore(value string, keywords []string) int {
	value = strings.ToLower(value)
	score := 0
	for _, keyword := range keywords {
		if strings.Contains(value, strings.ToLower(keyword)) {
			score++
		}
	}
	return score
}

func bestSquadWorkflowTemplate(sources []workflowWeightedText) (squadWorkflowTemplate, int, bool) {
	best := generalSquadWorkflowTemplate
	bestScore := 0
	tied := false
	for _, candidate := range squadWorkflowTemplates {
		score := 0
		for _, source := range sources {
			score += workflowTextScore(source.value, candidate.keywords) * source.weight
		}
		if score > bestScore {
			best = candidate
			bestScore = score
			tied = false
		} else if score > 0 && score == bestScore {
			tied = true
		}
	}
	return best, bestScore, tied
}

func chooseSquadWorkflowTemplate(squad db.Squad, agents []squadWorkflowAgent) squadWorkflowTemplate {
	squadSources := []workflowWeightedText{
		{value: squad.Name, weight: 12},
		{value: squad.Description, weight: 7},
	}
	best, score, tied := bestSquadWorkflowTemplate(squadSources)
	if score > 0 && !tied {
		return best
	}

	// Squad instructions are often long and cross-functional. Only use them when
	// they contain more than one explicit signal for a single domain.
	best, score, tied = bestSquadWorkflowTemplate([]workflowWeightedText{{value: squad.Instructions, weight: 1}})
	if score >= 2 && !tied {
		return best
	}

	workers := make([]squadWorkflowAgent, 0, len(agents))
	for _, agent := range agents {
		if !squad.LeaderID.Valid || agent.id != squad.LeaderID {
			workers = append(workers, agent)
		}
	}
	if len(workers) == 0 {
		return generalSquadWorkflowTemplate
	}

	votes := make(map[string]int)
	profiles := make(map[string]squadWorkflowTemplate)
	for _, agent := range workers {
		candidate, agentScore, agentTied := bestSquadWorkflowTemplate([]workflowWeightedText{
			{value: agent.name, weight: 8},
			{value: agent.role, weight: 7},
			{value: agent.description, weight: 4},
		})
		if agentScore == 0 || agentTied {
			continue
		}
		votes[candidate.profile]++
		profiles[candidate.profile] = candidate
	}

	majority := len(workers)/2 + 1
	for profile, voteCount := range votes {
		if voteCount >= majority {
			return profiles[profile]
		}
	}
	return generalSquadWorkflowTemplate
}

func defaultAgentWorkflowStage(stages []squadWorkflowTemplateStage) int {
	preferred := []string{"execution", "implementation", "resolution", "domain", "analysis", "growth", "acquisition"}
	for _, stageID := range preferred {
		for index, stage := range stages {
			if stage.id == stageID {
				return index
			}
		}
	}
	return len(stages) / 2
}

func chooseAgentWorkflowStage(agent squadWorkflowAgent, stages []squadWorkflowTemplateStage) int {
	sources := []workflowWeightedText{
		{value: agent.name, weight: 8},
		{value: agent.role, weight: 7},
		{value: agent.description, weight: 4},
	}
	bestIndex, bestScore := 0, 0
	for stageIndex, stage := range stages {
		score := 0
		for _, source := range sources {
			score += workflowTextScore(source.value, stage.keywords) * source.weight
		}
		if score > bestScore {
			bestIndex, bestScore = stageIndex, score
		}
	}
	if bestScore > 0 {
		return bestIndex
	}

	// Instructions may contain examples spanning several responsibilities. A
	// single generic word must not decide an agent's workflow position.
	bestIndex, bestScore = 0, 0
	for stageIndex, stage := range stages {
		score := workflowTextScore(agent.instructions, stage.keywords)
		if score > bestScore {
			bestIndex, bestScore = stageIndex, score
		}
	}
	if bestScore >= 2 {
		return bestIndex
	}
	return defaultAgentWorkflowStage(stages)
}

func buildSquadWorkflowLayout(template squadWorkflowTemplate, workers []squadWorkflowAgent) ([]int, []int) {
	assignments := make([]int, len(workers))
	usedStages := make(map[int]bool)
	for index, agent := range workers {
		stageIndex := chooseAgentWorkflowStage(agent, template.stages)
		assignments[index] = stageIndex
		usedStages[stageIndex] = true
	}

	stageIndexes := make([]int, 0, len(usedStages))
	for stageIndex := range template.stages {
		if usedStages[stageIndex] {
			stageIndexes = append(stageIndexes, stageIndex)
		}
	}
	if len(stageIndexes) == 0 && len(template.stages) > 0 {
		stageIndexes = append(stageIndexes, defaultAgentWorkflowStage(template.stages))
	}
	return stageIndexes, assignments
}

func (h *Handler) loadSquadWorkflowAgents(ctx context.Context, squadID pgtype.UUID) ([]squadWorkflowAgent, error) {
	rows, err := h.DB.Query(ctx, `
		SELECT sm.member_id, sm.role, a.name, a.description, a.instructions
		FROM squad_member sm
		JOIN agent a ON a.id = sm.member_id
		WHERE sm.squad_id = $1 AND sm.member_type = 'agent'
		ORDER BY sm.created_at, sm.id
	`, squadID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	agents := make([]squadWorkflowAgent, 0)
	for rows.Next() {
		var agent squadWorkflowAgent
		if err := rows.Scan(&agent.id, &agent.role, &agent.name, &agent.description, &agent.instructions); err != nil {
			return nil, err
		}
		agents = append(agents, agent)
	}
	return agents, rows.Err()
}

func (h *Handler) generateSquadWorkflow(ctx context.Context, squad db.Squad, updatedBy pgtype.UUID) (string, error) {
	agents, err := h.loadSquadWorkflowAgents(ctx, squad.ID)
	if err != nil {
		return "", err
	}
	template := chooseSquadWorkflowTemplate(squad, agents)
	useChinese := containsHan(squad.Name + squad.Description + squad.Instructions)

	tx, err := h.TxStarter.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if _, err := tx.Exec(ctx, `DELETE FROM squad_workflow_assignment WHERE squad_id = $1`, squad.ID); err != nil {
		return "", err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM squad_workflow_stage WHERE squad_id = $1`, squad.ID); err != nil {
		return "", err
	}
	workers := make([]squadWorkflowAgent, 0, len(agents))
	for _, agent := range agents {
		if !squad.LeaderID.Valid || agent.id != squad.LeaderID {
			workers = append(workers, agent)
		}
	}
	stageIndexes, assignments := buildSquadWorkflowLayout(template, workers)
	for position, stageIndex := range stageIndexes {
		stage := template.stages[stageIndex]
		name, description := stage.nameEN, stage.descriptionEN
		if useChinese {
			name, description = stage.nameZH, stage.descriptionZH
		}
		stageID := fmt.Sprintf("generated_%s_%s", template.profile, stage.id)
		if _, err := tx.Exec(ctx, `
			INSERT INTO squad_workflow_stage
				(squad_id, id, name, description, position, baseline_position, keywords, updated_by)
			VALUES ($1, $2, $3, $4, $5, $5, $6, $7)
		`, squad.ID, stageID, name, description, position, stage.keywords, updatedBy); err != nil {
			return "", err
		}
	}

	for index, agent := range workers {
		stageIndex := assignments[index]
		stageID := fmt.Sprintf("generated_%s_%s", template.profile, template.stages[stageIndex].id)
		if _, err := tx.Exec(ctx, `
			INSERT INTO squad_workflow_assignment (squad_id, agent_id, stage_id, updated_by, source)
			VALUES ($1, $2, $3, $4, 'generated')
		`, squad.ID, agent.id, stageID, updatedBy); err != nil {
			return "", err
		}
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO squad_workflow_config (squad_id, source, profile, generated_at, updated_at, canvas_layout)
		VALUES ($1, 'domain_generated', $2, now(), now(), '{"stages":{},"agents":{}}'::jsonb)
		ON CONFLICT (squad_id) DO UPDATE SET
			source = EXCLUDED.source,
			profile = EXCLUDED.profile,
			generated_at = EXCLUDED.generated_at,
			canvas_layout = EXCLUDED.canvas_layout,
			updated_at = EXCLUDED.updated_at
	`, squad.ID, template.profile); err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return template.profile, nil
}
