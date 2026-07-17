// @vitest-environment jsdom

import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { CreativeMaterialsResponse, Issue } from "@multica/core/types";

const apiMocks = vi.hoisted(() => ({
  getCreativeMaterials: vi.fn(),
  updateCreativeMaterialCandidate: vi.fn(),
  listAgents: vi.fn(),
  createComment: vi.fn(),
  syncCreativeEditJob: vi.fn(),
  downloadCreativeEditJob: vi.fn(),
  createCreativeEditFeedback: vi.fn(),
}));

vi.mock("@multica/core/api", () => ({ api: apiMocks }));
vi.mock("@multica/core/hooks", () => ({ useWorkspaceId: () => "workspace-1" }));
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

import { CreativeMaterialPool } from "./creative-material-pool";

const issue: Issue = {
  id: "issue-1",
  workspace_id: "workspace-1",
  number: 1,
  identifier: "CRE-1",
  title: "素材生成",
  description: null,
  status: "in_review",
  priority: "none",
  assignee_type: null,
  assignee_id: null,
  creator_type: "member",
  creator_id: "user-1",
  parent_issue_id: null,
  project_id: null,
  position: 0,
  start_date: null,
  due_date: null,
  metadata: { workflow: "creative_material" },
  created_at: "2026-07-16T00:00:00Z",
  updated_at: "2026-07-16T00:00:00Z",
};

const response: CreativeMaterialsResponse = {
  enabled: true,
  summary: {
    total: 1,
    new: 0,
    selected: 1,
    rejected: 0,
    sent_to_edit: 0,
    edited: 0,
    approved: 0,
    archived: 0,
  },
  candidates: [{
    id: "candidate-1",
    workspace_id: "workspace-1",
    connector_id: "connector-1",
    external_id: "external-1",
    dedupe_key: "dedupe-1",
    competitor: "Kredit Pintar",
    title: "额度素材",
    asset_type: "image",
    preview_url: "",
    resource_url: "",
    poster_url: "",
    original_url: "",
    archived_url: "",
    archive_status: "completed",
    archive_error: "",
    duration_days: 6,
    impression_estimate: 24100,
    media_names: [],
    area_names: ["Indonesia"],
    language_names: ["Indonesian"],
    platform_names: ["Android"],
    status: "selected",
    tags: [],
    note: "",
    selected_at: "2026-07-16T00:00:00Z",
    first_seen_at: "2026-07-16T00:00:00Z",
    last_seen_at: "2026-07-16T00:00:00Z",
    created_at: "2026-07-16T00:00:00Z",
    updated_at: "2026-07-16T00:00:00Z",
  }],
  crawl_runs: [],
  edit_jobs: [],
};

function renderPool() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <CreativeMaterialPool issue={issue} />
    </QueryClientProvider>,
  );
}

describe("CreativeMaterialPool edit prompt", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    window.sessionStorage.clear();
    apiMocks.getCreativeMaterials.mockResolvedValue(response);
    apiMocks.listAgents.mockResolvedValue([{ id: "creative-agent-1", name: "修图智能体" }]);
    apiMocks.createComment.mockResolvedValue({ id: "comment-1" });
  });

  it("keeps the draft across a remount and hands selected materials to the creative agent", async () => {
    const draft = "保留原图构图，只替换核心利益点。";
    const firstRender = renderPool();
    const firstInput = await screen.findByPlaceholderText("给修图智能体的补充要求，可不填");

    fireEvent.change(firstInput, { target: { value: draft } });
    expect(firstInput).toHaveValue(draft);
    expect(window.sessionStorage.getItem(
      "multica:creative-edit-prompt:workspace-1:issue-1",
    )).toBe(draft);

    firstRender.unmount();
    renderPool();
    const restoredInput = await screen.findByPlaceholderText("给修图智能体的补充要求，可不填");
    await waitFor(() => expect(restoredInput).toHaveValue(draft));

    fireEvent.click(screen.getByRole("button", { name: "交给修图智能体" }));
    await waitFor(() => expect(apiMocks.createComment).toHaveBeenCalledWith(
      "issue-1",
      expect.stringContaining("[@修图智能体](mention://agent/creative-agent-1)"),
    ));
    const handoffComment = apiMocks.createComment.mock.calls[0]?.[1];
    expect(handoffComment).toContain(draft);
    await waitFor(() => expect(restoredInput).toHaveValue(""));
    expect(window.sessionStorage.getItem(
      "multica:creative-edit-prompt:workspace-1:issue-1",
    )).toBeNull();
  });

  it("shows QC-rejected attempts when a failed job has no final variants", async () => {
    apiMocks.getCreativeMaterials.mockResolvedValue({
      ...response,
      edit_jobs: [{
        id: "job-failed",
        workspace_id: "workspace-1",
        issue_id: "issue-1",
        status: "failed",
        prompt: "",
        rules: {},
        process_data: {
          candidates: [{
            candidate_id: "candidate-1",
            submissions: [{
              submission_index: 1,
              status: "failed",
              process: {
                rounds: [{
                  round: 1,
                  verdicts: [{
                    concept_index: 1,
                    size: "1080x1080",
                    verdict: "fail",
                    issues: [{ type: "financial", message: "金额文案与授权文案不一致。" }],
                    artifacts: {
                      image: "attempt_1_1_1080x1080.png",
                      image_url: "http://127.0.0.1:8010/files/lean-failed/attempt_1_1_1080x1080.png",
                    },
                  }],
                }],
              },
            }],
          }],
        },
        external_provider: "workspace_mcp",
        mcp_connection_id: "connection-1",
        external_job_id: "external-failed",
        external_status: "failed",
        stage: "quality_rejected",
        progress: 100,
        last_poll_at: "2026-07-16T00:10:00Z",
        next_poll_at: "",
        completed_at: "2026-07-16T00:10:00Z",
        error_message: "No conceptual variant completed all QCs.",
        poll_attempts: 2,
        created_at: "2026-07-16T00:00:00Z",
        updated_at: "2026-07-16T00:10:00Z",
        candidate_ids: ["candidate-1"],
        variants: [],
      }],
    } satisfies CreativeMaterialsResponse);

    renderPool();

    expect(await screen.findByText("任务失败")).toBeInTheDocument();
    expect(screen.getByText("QC 每轮记录")).toBeInTheDocument();
    expect(screen.getByText("金额文案与授权文案不一致。")).toBeInTheDocument();
    expect(screen.getByAltText("第 1 轮概念 1 1080x1080 尝试图")).toHaveAttribute(
      "src",
      "http://127.0.0.1:8010/files/lean-failed/attempt_1_1_1080x1080.png",
    );
  });
});
