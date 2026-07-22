import { describe, expect, it } from "vitest";
import {
  AppConfigSchema,
  CredentialCrawlResultSchema,
  CreativeMaterialsResponseSchema,
  DashboardAgentRunTimeListSchema,
  DashboardUsageByAgentListSchema,
  DashboardUsageDailyListSchema,
  DuplicateIssueErrorBodySchema,
  EMPTY_USER,
  ListCredentialProfilesResponseSchema,
  ListWorkspaceMCPConnectionsResponseSchema,
  LoginResponseSchema,
  ListIssuesResponseSchema,
  PreviewSessionListResponseSchema,
  RuntimeHourlyActivityListSchema,
  RuntimeUsageByAgentListSchema,
  RuntimeUsageByHourListSchema,
  RuntimeUsageListSchema,
  SquadListSchema,
  SquadSchema,
  UserSchema,
} from "./schemas";
import { parseWithFallback } from "./schema";

const baseIssue = {
  id: "11111111-1111-1111-1111-111111111111",
  workspace_id: "ws-1",
  number: 1,
  identifier: "MUL-1",
  title: "Test",
  description: null,
  status: "todo",
  priority: "medium",
  assignee_type: null,
  assignee_id: null,
  creator_type: "member",
  creator_id: "user-1",
  parent_issue_id: null,
  project_id: null,
  position: 0,
  start_date: null,
  due_date: null,
  metadata: {},
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
};

describe("IssueSchema (via ListIssuesResponseSchema)", () => {
  it("accepts a primitive metadata KV map", () => {
    const payload = {
      issues: [
        {
          ...baseIssue,
          metadata: { pipeline_status: "waiting", pr_number: 3, is_blocked: true },
        },
      ],
      total: 1,
    };
    const parsed = ListIssuesResponseSchema.parse(payload);
    expect(parsed.issues[0]?.metadata).toEqual({
      pipeline_status: "waiting",
      pr_number: 3,
      is_blocked: true,
    });
  });

  it("defaults metadata to {} when the server omits it (older backend)", () => {
    const { metadata: _omit, ...issueWithoutMetadata } = baseIssue;
    const payload = { issues: [issueWithoutMetadata], total: 1 };
    const parsed = ListIssuesResponseSchema.parse(payload);
    expect(parsed.issues[0]?.metadata).toEqual({});
  });

  it("rejects metadata with non-primitive values (nested object)", () => {
    const payload = {
      issues: [{ ...baseIssue, metadata: { nested: { x: 1 } } }],
      total: 1,
    };
    expect(ListIssuesResponseSchema.safeParse(payload).success).toBe(false);
  });
});

describe("credential broker schemas", () => {
  it("keeps profile lists renderable when optional fields are absent", () => {
    const parsed = ListCredentialProfilesResponseSchema.parse({
      profiles: [
        {
          id: "profile-1",
          connector_id: "appgrowing",
          label: "Manual Acceptance",
          status: "active",
          created_at: "2026-07-06T10:00:00Z",
          updated_at: "2026-07-06T10:01:00Z",
        },
      ],
    });

    expect(parsed.profiles[0]?.last_used_at).toBeUndefined();
    expect(parsed.profiles[0]?.status).toBe("active");
  });

  it("defaults crawl probe counters and preserves raw auth diagnostics", () => {
    const parsed = CredentialCrawlResultSchema.parse({
      status: "completed",
      message: "ok",
      raw: { auth_probe: { auth_check: { authenticated: true } } },
    });

    expect(parsed.downloaded).toBe(0);
    expect(parsed.raw).toEqual({
      auth_probe: { auth_check: { authenticated: true } },
    });
  });
});

describe("creative material schemas", () => {
  it("defaults recovery and archive fields from an older backend", () => {
    const parsed = CreativeMaterialsResponseSchema.parse({
      enabled: true,
      candidates: [{ id: "candidate-1" }],
      edit_jobs: [{ id: "job-1" }],
    });

    expect(parsed.candidates[0]?.archived_url).toBe("");
    expect(parsed.candidates[0]?.archive_status).toBe("pending");
    expect(parsed.edit_jobs[0]?.poll_attempts).toBe(0);
    expect(parsed.edit_jobs[0]?.mcp_connection_id).toBe("");
    expect(parsed.edit_jobs[0]?.process_data).toEqual({});
    expect(parsed.edit_jobs[0]?.variants).toEqual([]);
  });

  it("defaults feedback history and contains malformed telemetry fields", () => {
    const parsed = CreativeMaterialsResponseSchema.parse({
      enabled: true,
      edit_jobs: [{
        id: "job-1",
        variants: [{
          id: "variant-1",
          feedback: [{
            id: "feedback-1",
            decision: "accepted",
            reason_codes: null,
            process_snapshot: null,
          }],
        }, {
          id: "variant-from-older-server",
        }],
      }],
    });

    expect(parsed.edit_jobs[0]?.variants[0]?.feedback[0]?.reason_codes).toEqual([]);
    expect(parsed.edit_jobs[0]?.variants[0]?.feedback[0]?.process_snapshot).toEqual({});
    expect(parsed.edit_jobs[0]?.variants[1]?.feedback).toEqual([]);
  });

  it("preserves valid process data and contains malformed process data", () => {
    const parsed = CreativeMaterialsResponseSchema.parse({
      enabled: true,
      edit_jobs: [
        {
          id: "valid",
          process_data: {
            schema_version: "1",
            usage: { cost: "not_available" },
          },
        },
        { id: "malformed", process_data: "unexpected" },
      ],
    });

    expect(parsed.edit_jobs[0]?.process_data).toEqual({
      schema_version: "1",
      usage: { cost: "not_available" },
    });
    expect(parsed.edit_jobs[1]?.process_data).toEqual({});
  });
});

describe("workspace MCP schemas", () => {
  it("never requires secret values in a list response", () => {
    const parsed = ListWorkspaceMCPConnectionsResponseSchema.parse({
      connections: [{
        id: "connection-1",
        name: "Creative service",
        server_url: "https://creative.example.com/mcp",
        has_secret_headers: true,
        secret_header_names: ["Authorization"],
      }],
    });

    expect(parsed.connections[0]?.secret_header_names).toEqual(["Authorization"]);
    expect(parsed.connections[0]?.status).toBe("disabled");
    expect(parsed.connections[0]).not.toHaveProperty("secret_headers");
  });
});

describe("PreviewSessionListResponseSchema", () => {
  const session = {
    id: "preview-1",
    workspace_id: "ws-1",
    issue_id: "issue-1",
    task_id: null,
    platform: "web",
    provider: "external_web",
    title: "Checkout preview",
    preview_url: "https://preview.example.test/checkout",
    status: "running",
    creator_type: "member",
    creator_id: "user-1",
    error_message: null,
    expires_at: "2026-07-14T12:00:00Z",
    last_active_at: "2026-07-13T12:00:00Z",
    lease_expires_at: "2026-07-13T12:05:00Z",
    started_at: "2026-07-13T12:00:00Z",
    stopped_at: null,
    created_at: "2026-07-13T11:59:00Z",
    updated_at: "2026-07-13T12:00:00Z",
  };

  it("converts the wire response to camelCase domain data", () => {
    const parsed = PreviewSessionListResponseSchema.parse({
      preview_sessions: [session],
      total: 1,
    });

    expect(parsed).toEqual({
      previewSessions: [
        expect.objectContaining({
          id: "preview-1",
          workspaceId: "ws-1",
          issueId: "issue-1",
          taskId: null,
          previewUrl: "https://preview.example.test/checkout",
          errorMessage: null,
          expiresAt: "2026-07-14T12:00:00Z",
          lastActiveAt: "2026-07-13T12:00:00Z",
          leaseExpiresAt: "2026-07-13T12:05:00Z",
          createdAt: "2026-07-13T11:59:00Z",
        }),
      ],
      total: 1,
    });
  });

  it("defaults optional display fields and preserves unknown enum values", () => {
    const parsed = PreviewSessionListResponseSchema.parse({
      preview_sessions: [
        {
          id: "preview-2",
          workspace_id: "ws-1",
          issue_id: "issue-1",
          platform: "vision_os",
          status: "paused_by_provider",
        },
      ],
    });

    expect(parsed.previewSessions[0]).toMatchObject({
      taskId: null,
      platform: "vision_os",
      provider: "unknown",
      title: "",
      previewUrl: "",
      status: "paused_by_provider",
      errorMessage: null,
    });
    expect(parsed.total).toBe(0);
  });

  it("lets parseWithFallback reject a malformed list body", () => {
    const fallback = { previewSessions: [], total: 0 };
    const parsed = parseWithFallback(
      { preview_sessions: null, total: "one" },
      PreviewSessionListResponseSchema,
      fallback,
      { endpoint: "GET /api/issues/:id/preview-sessions" },
    );

    expect(parsed).toBe(fallback);
  });
});

describe("LoginResponseSchema", () => {
  it("keeps auth responses renderable when user is malformed", () => {
    const parsed = LoginResponseSchema.parse({ token: "tok", user: null });
    expect(parsed.token).toBe("tok");
    expect(parsed.user).toEqual(EMPTY_USER);
  });
});

// The duplicate-issue branch in create-issue.tsx feeds ApiError.body
// (typed as `unknown`) through this schema. Any future server drift that
// loses the contract MUST fail the parse so the UI falls back to a normal
// error toast instead of rendering an empty / partial duplicate card.
describe("DuplicateIssueErrorBodySchema", () => {
  const valid = {
    code: "active_duplicate_issue",
    error: "An active issue with this title already exists: MUL-12 – Login bug",
    issue: {
      id: "11111111-1111-1111-1111-111111111111",
      identifier: "MUL-12",
      title: "Login bug",
    },
  };

  it("accepts a well-formed body", () => {
    expect(DuplicateIssueErrorBodySchema.safeParse(valid).success).toBe(true);
  });

  it("accepts unknown extra fields via .loose()", () => {
    const forwardCompat = {
      ...valid,
      hint: "Try a different title",
      issue: { ...valid.issue, workspace_id: "ws-1", status: "todo" },
    };
    expect(DuplicateIssueErrorBodySchema.safeParse(forwardCompat).success).toBe(true);
  });

  it("rejects a renamed code (so renames degrade to the generic toast)", () => {
    const renamed = { ...valid, code: "duplicate_issue" };
    expect(DuplicateIssueErrorBodySchema.safeParse(renamed).success).toBe(false);
  });

  it("rejects a missing issue object", () => {
    const { issue: _omit, ...without } = valid;
    expect(DuplicateIssueErrorBodySchema.safeParse(without).success).toBe(false);
  });

  it("rejects a non-string issue.id", () => {
    const broken = { ...valid, issue: { ...valid.issue, id: 42 } };
    expect(DuplicateIssueErrorBodySchema.safeParse(broken).success).toBe(false);
  });

  it("accepts a missing error field (it is optional)", () => {
    const { error: _omit, ...without } = valid;
    expect(DuplicateIssueErrorBodySchema.safeParse(without).success).toBe(true);
  });
});

// `user.timezone` (Viewing tz) was added in the timezone-architecture RFC.
// A desktop build older than the server — or a server predating the
// `user.timezone` migration — will return a `/api/me` body with no
// `timezone` key. The schema must not fail closed on that: the field
// defaults to `null`, which the frontend resolves to the browser-detected
// tz at render time.
describe("UserSchema timezone drift", () => {
  const base = {
    id: "11111111-1111-1111-1111-111111111111",
    name: "Ada",
    email: "ada@example.com",
  };

  it("defaults timezone to null when the field is absent", () => {
    const parsed = UserSchema.parse(base);
    expect(parsed.timezone).toBe(null);
  });

  it("preserves an explicit IANA timezone", () => {
    const parsed = UserSchema.parse({ ...base, timezone: "Asia/Tokyo" });
    expect(parsed.timezone).toBe("Asia/Tokyo");
  });

  it("accepts an explicit null timezone", () => {
    const parsed = UserSchema.parse({ ...base, timezone: null });
    expect(parsed.timezone).toBe(null);
  });

  // Wrong-type drift: a future server bug sending `timezone` as a number
  // must not throw into the UI. parseWithFallback degrades the whole user
  // object to the explicit fallback (EMPTY_USER) so /api/me callers keep a
  // valid shape instead of white-screening.
  it("falls back to EMPTY_USER when timezone is the wrong type", () => {
    const parsed = parseWithFallback(
      { ...base, timezone: 42 },
      UserSchema,
      EMPTY_USER,
      { endpoint: "GET /api/me" },
    );
    expect(parsed).toBe(EMPTY_USER);
  });
});

describe("SquadListSchema member preview drift", () => {
  const baseSquad = {
    id: "squad-1",
    workspace_id: "ws-1",
    name: "Frontend Squad",
    description: "",
    instructions: "",
    avatar_url: null,
    leader_id: "agent-1",
    creator_id: "user-1",
    created_at: "2026-05-01T00:00:00Z",
    updated_at: "2026-05-01T00:00:00Z",
    archived_at: null,
    archived_by: null,
  };

  it("defaults preview fields when an older backend omits them", () => {
    const parsed = SquadListSchema.parse([baseSquad]);
    expect(parsed[0]?.member_count).toBe(0);
    expect(parsed[0]?.member_preview).toEqual([]);
  });

  it("defaults preview fields on a single squad response", () => {
    const parsed = SquadSchema.parse(baseSquad);
    expect(parsed.member_count).toBe(0);
    expect(parsed.member_preview).toEqual([]);
  });

  it("preserves lightweight member preview rows", () => {
    const parsed = SquadListSchema.parse([
      {
        ...baseSquad,
        member_count: 2,
        member_preview: [
          { member_type: "agent", member_id: "agent-1", role: "leader" },
          { member_type: "member", member_id: "user-2", role: "member" },
        ],
      },
    ]);
    expect(parsed[0]?.member_count).toBe(2);
    expect(parsed[0]?.member_preview).toHaveLength(2);
    expect(parsed[0]?.member_preview?.[0]?.role).toBe("leader");
  });
});

// The workspace dashboard and runtime-detail pages were re-pointed at the
// unified `task_usage_hourly` rollup. Every numeric field drives chart /
// KPI math, and string keys (date / agent_id / model) bucket the series.
// The contract these schemas must hold: a row missing a field degrades
// that field to a sane default rather than dropping the WHOLE array to
// the `[]` fallback — one drifted row must not blank the entire chart.
describe("dashboard + runtime usage schema drift", () => {
  it("coerces a missing numeric field to 0 instead of dropping the array", () => {
    const parsed = DashboardUsageDailyListSchema.parse([
      { date: "2026-05-19", model: "claude-opus-4-7", input_tokens: 100 },
    ]);
    expect(parsed).toHaveLength(1);
    expect(parsed[0]?.output_tokens).toBe(0);
    expect(parsed[0]?.cache_read_tokens).toBe(0);
    expect(parsed[0]?.cache_write_tokens).toBe(0);
  });

  it("coerces a missing date key to \"\" so the rest of the series survives", () => {
    const parsed = DashboardUsageDailyListSchema.parse([
      { model: "claude-opus-4-7", input_tokens: 5 },
    ]);
    expect(parsed).toHaveLength(1);
    expect(parsed[0]?.date).toBe("");
  });

  it("coerces a missing agent_id key to \"\" for the agent-runtime panel", () => {
    const parsed = DashboardAgentRunTimeListSchema.parse([
      { total_seconds: 42, task_count: 3, failed_count: 0 },
    ]);
    expect(parsed).toHaveLength(1);
    expect(parsed[0]?.agent_id).toBe("");
  });

  it("coerces a missing agent_id key to \"\" for the usage-by-agent panel", () => {
    const parsed = DashboardUsageByAgentListSchema.parse([
      { model: "claude-opus-4-7", input_tokens: 7 },
    ]);
    expect(parsed[0]?.agent_id).toBe("");
  });

  it("coerces missing fields on every runtime usage schema", () => {
    expect(RuntimeUsageListSchema.parse([{ date: "2026-05-19" }])[0]?.input_tokens).toBe(0);
    expect(RuntimeHourlyActivityListSchema.parse([{ hour: 9 }])[0]?.count).toBe(0);
    expect(RuntimeUsageByAgentListSchema.parse([{ model: "x" }])[0]?.agent_id).toBe("");
    expect(RuntimeUsageByHourListSchema.parse([{ hour: 9 }])[0]?.model).toBe("");
  });

  it("rejects a non-array body so parseWithFallback can return its fallback", () => {
    expect(DashboardUsageDailyListSchema.safeParse(null).success).toBe(false);
    expect(RuntimeUsageListSchema.safeParse({ rows: [] }).success).toBe(false);
  });

  it("keeps unknown server-side fields via .loose()", () => {
    const parsed = RuntimeUsageListSchema.parse([
      { date: "2026-05-19", region: "us-east" },
    ]);
    expect((parsed[0] as Record<string, unknown>).region).toBe("us-east");
  });
});

describe("AppConfigSchema cdn_signed drift", () => {
  it("defaults cdn_signed to false when the server omits it (pre-MUL-3254 servers)", () => {
    const parsed = AppConfigSchema.parse({ cdn_domain: "cdn.example.com" });
    expect(parsed.cdn_signed).toBe(false);
  });

  it("coerces a malformed cdn_signed to false instead of failing the whole config", () => {
    const parsed = AppConfigSchema.parse({
      cdn_domain: "cdn.example.com",
      cdn_signed: "yes",
    });
    expect(parsed.cdn_signed).toBe(false);
    expect(parsed.cdn_domain).toBe("cdn.example.com");
  });

  it("keeps cdn_signed=true from a signing-enabled server", () => {
    const parsed = AppConfigSchema.parse({ cdn_signed: true });
    expect(parsed.cdn_signed).toBe(true);
  });
});
