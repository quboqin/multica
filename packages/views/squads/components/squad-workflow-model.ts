import type { Agent, SquadMember } from "@multica/core/types";

export type WorkflowStageId =
  | "requirements"
  | "knowledge"
  | "research"
  | "design"
  | "implementation"
  | "review"
  | "test"
  | "delivery"
  | "support";

export type WorkflowMatchSource = "name" | "role" | "description" | "instructions";

export type WorkflowPlacement = {
  member: SquadMember;
  agent: Agent | undefined;
  match: { source: WorkflowMatchSource; keyword: string } | null;
};

export type InferredWorkflowStage = {
  id: WorkflowStageId;
  placements: WorkflowPlacement[];
};

type WorkflowStageDefinition = {
  id: WorkflowStageId;
  keywords: string[];
};

export const WORKFLOW_STAGE_DEFINITIONS: WorkflowStageDefinition[] = [
  {
    id: "requirements",
    keywords: ["需求分析", "需求理解", "需求", "prd", "requirement", "产品分析", "业务分析", "验收标准"],
  },
  {
    id: "knowledge",
    keywords: ["规范知识", "知识库", "知识", "规范", "context", "上下文", "文档检索", "knowledge"],
  },
  {
    id: "research",
    keywords: ["系统调研", "代码调研", "技术调研", "调研", "排查", "诊断", "research", "investigation", "analysis"],
  },
  {
    id: "design",
    keywords: ["方案设计", "技术设计", "架构设计", "设计", "方案", "architecture", "design", "任务拆分"],
  },
  {
    id: "implementation",
    keywords: ["代码实现", "编码实现", "开发实现", "写代码", "开发", "编码", "implementation", "coding", "developer"],
  },
  {
    id: "review",
    keywords: ["代码评审", "代码审核", "质量评审", "review", "审查", "评审", "code quality"],
  },
  {
    id: "test",
    keywords: ["测试修复", "缺陷修复", "测试", "修复", "回归", "qa", "test", "bugfix", "bug fix"],
  },
  {
    id: "delivery",
    keywords: ["测试提测", "交付动作", "发布上线", "提测", "交付", "发布", "上线", "submit", "delivery", "release", "deploy"],
  },
  { id: "support", keywords: [] },
];

export function inferSquadWorkflow(members: SquadMember[], agents: Agent[]): InferredWorkflowStage[] {
  const placementsByStage = new Map<WorkflowStageId, WorkflowPlacement[]>();
  const fallbackStage = WORKFLOW_STAGE_DEFINITIONS[WORKFLOW_STAGE_DEFINITIONS.length - 1]!;

  for (const member of members) {
    const agent = agents.find((candidate) => candidate.id === member.member_id);
    const sources: Array<{ source: WorkflowMatchSource; value: string; weight: number }> = [
      { source: "name", value: agent?.name ?? "", weight: 8 },
      { source: "role", value: member.role ?? "", weight: 6 },
      { source: "description", value: agent?.description ?? "", weight: 3 },
      { source: "instructions", value: agent?.instructions?.slice(0, 4000) ?? "", weight: 1 },
    ];
    let bestStage = fallbackStage;
    let bestScore = 0;
    let bestMatch: WorkflowPlacement["match"] = null;

    for (const stage of WORKFLOW_STAGE_DEFINITIONS.slice(0, -1)) {
      for (const source of sources) {
        const normalizedValue = source.value.toLocaleLowerCase();
        for (const keyword of stage.keywords) {
          if (!normalizedValue.includes(keyword.toLocaleLowerCase())) continue;
          const score = source.weight * 100 + keyword.length;
          if (score > bestScore) {
            bestScore = score;
            bestStage = stage;
            bestMatch = { source: source.source, keyword };
          }
        }
      }
    }

    const placements = placementsByStage.get(bestStage.id) ?? [];
    placements.push({ member, agent, match: bestMatch });
    placementsByStage.set(bestStage.id, placements);
  }

  return WORKFLOW_STAGE_DEFINITIONS.flatMap((stage) => {
    const placements = placementsByStage.get(stage.id);
    return placements?.length ? [{ id: stage.id, placements }] : [];
  });
}
