package handler

import (
	"testing"

	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

func TestChooseSquadWorkflowTemplate(t *testing.T) {
	tests := []struct {
		name    string
		squad   db.Squad
		agents  []squadWorkflowAgent
		profile string
	}{
		{name: "software", squad: db.Squad{Name: "研发小队V2"}, profile: "software"},
		{name: "product", squad: db.Squad{Name: "产品", Description: "产品线和用户研究协作"}, profile: "product"},
		{name: "modeling", squad: db.Squad{Name: "模型", Description: "变量衍生和模型分监控"}, profile: "data_model"},
		{
			name:  "risk beats mixed member specialties",
			squad: db.Squad{Name: "风险决策", Description: "风险策略协作"},
			agents: []squadWorkflowAgent{
				{name: "数据", description: "数据分析"},
				{name: "产品", description: "产品方案"},
				{name: "风险代码", description: "策略规则实现"},
			},
			profile: "risk",
		},
		{name: "marketing", squad: db.Squad{Name: "市场", Description: "广告投放和增长运营"}, profile: "marketing"},
		{name: "general", squad: db.Squad{Name: "专项协作组"}, profile: "general"},
		{
			name:  "mixed specialties stay general",
			squad: db.Squad{Name: "测试小队"},
			agents: []squadWorkflowAgent{
				{name: "研发-rubik可行性分析", description: "分析哪些用例可以写入 rubik 平台"},
				{name: "产品_知识问答", description: "提供产品知识支持"},
				{name: "催收", description: "马来西亚业务催收专家"},
			},
			profile: "general",
		},
		{
			name:  "member majority selects domain",
			squad: db.Squad{Name: "专项协作组"},
			agents: []squadWorkflowAgent{
				{name: "后端开发", description: "服务端代码实现"},
				{name: "测试工程师", description: "代码测试和质量验证"},
				{name: "业务顾问"},
			},
			profile: "software",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := chooseSquadWorkflowTemplate(tt.squad, tt.agents).profile; got != tt.profile {
				t.Fatalf("profile = %q, want %q", got, tt.profile)
			}
		})
	}
}

func TestChooseAgentWorkflowStage(t *testing.T) {
	template := squadWorkflowTemplates[0]
	tests := []struct {
		name  string
		agent squadWorkflowAgent
		stage string
	}{
		{name: "requirements", agent: squadWorkflowAgent{name: "需求分析", role: "requirement analysis"}, stage: "requirements"},
		{name: "implementation", agent: squadWorkflowAgent{name: "代码实现", description: "编写代码和测试"}, stage: "implementation"},
		{name: "review", agent: squadWorkflowAgent{name: "代码评审", role: "code review"}, stage: "review"},
		{name: "delivery", agent: squadWorkflowAgent{name: "交付动作", role: "delivery actions"}, stage: "delivery"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			index := chooseAgentWorkflowStage(tt.agent, template.stages)
			if got := template.stages[index].id; got != tt.stage {
				t.Fatalf("stage = %q, want %q", got, tt.stage)
			}
		})
	}
}

func TestBuildSquadWorkflowLayoutOnlyKeepsOccupiedStages(t *testing.T) {
	workers := []squadWorkflowAgent{
		{name: "研发-rubik可行性分析", description: "分析哪些用例可以写入 rubik 平台"},
		{name: "催收", description: "马来西亚业务催收专家"},
		{name: "产品_知识问答", description: "提供产品知识支持"},
	}
	stageIndexes, assignments := buildSquadWorkflowLayout(generalSquadWorkflowTemplate, workers)

	wantStages := []string{"research", "execution", "knowledge"}
	if len(stageIndexes) != len(wantStages) {
		t.Fatalf("stage count = %d, want %d", len(stageIndexes), len(wantStages))
	}
	for index, stageIndex := range stageIndexes {
		if got := generalSquadWorkflowTemplate.stages[stageIndex].id; got != wantStages[index] {
			t.Fatalf("stage %d = %q, want %q", index, got, wantStages[index])
		}
	}

	wantAssignments := []string{"research", "execution", "knowledge"}
	for index, stageIndex := range assignments {
		if got := generalSquadWorkflowTemplate.stages[stageIndex].id; got != wantAssignments[index] {
			t.Fatalf("assignment %d = %q, want %q", index, got, wantAssignments[index])
		}
	}
}

func TestChooseAgentWorkflowStageIgnoresSingleInstructionKeyword(t *testing.T) {
	agent := squadWorkflowAgent{
		name:         "业务专家",
		description:  "负责马来西亚业务",
		instructions: "根据现有资料给出处理方案",
	}
	index := chooseAgentWorkflowStage(agent, generalSquadWorkflowTemplate.stages)
	if got := generalSquadWorkflowTemplate.stages[index].id; got != "execution" {
		t.Fatalf("stage = %q, want execution", got)
	}
}
