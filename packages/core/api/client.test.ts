import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiClient, ApiError } from "./client";

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("ApiClient", () => {
  describe("preview session endpoints", () => {
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
      expires_at: null,
      last_active_at: "2026-07-13T12:00:00Z",
      lease_expires_at: "2026-07-13T12:05:00Z",
      started_at: "2026-07-13T12:00:00Z",
      stopped_at: null,
      created_at: "2026-07-13T11:59:00Z",
      updated_at: "2026-07-13T12:00:00Z",
    };

    it("uses the list/create/get/touch/device/stop contracts and returns camelCase data", async () => {
      const fetchMock = vi
        .fn()
        .mockResolvedValueOnce(
          new Response(JSON.stringify({ preview_sessions: [session], total: 1 }), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          }),
        )
        .mockResolvedValueOnce(
          new Response(JSON.stringify(session), {
            status: 201,
            headers: { "Content-Type": "application/json" },
          }),
        )
        .mockResolvedValueOnce(
          new Response(JSON.stringify(session), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          }),
        )
        .mockResolvedValueOnce(
          new Response(JSON.stringify(session), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          }),
        )
        .mockResolvedValueOnce(
          new Response(
            JSON.stringify({
              ...session,
              preview_url: "http://127.0.0.1:18081?serial=USB-123",
            }),
            { status: 200, headers: { "Content-Type": "application/json" } },
          ),
        )
        .mockResolvedValueOnce(
          new Response(
            JSON.stringify({
              ...session,
              status: "stopped",
              stopped_at: "2026-07-13T12:10:00Z",
            }),
            { status: 200, headers: { "Content-Type": "application/json" } },
          ),
        );
      vi.stubGlobal("fetch", fetchMock);

      const client = new ApiClient("https://api.example.test");
      const list = await client.listPreviewSessions("issue-1");
      const created = await client.createPreviewSession("issue-1", {
        previewUrl: "https://preview.example.test/checkout",
      });
      const detail = await client.getPreviewSession("preview-1");
      const touched = await client.touchPreviewSession("preview-1");
      const switched = await client.switchPreviewSessionDevice("preview-1", "USB-123", true);
      const stopped = await client.stopPreviewSession("preview-1");

      expect(list.previewSessions[0]?.previewUrl).toBe(
        "https://preview.example.test/checkout",
      );
      expect(created.workspaceId).toBe("ws-1");
      expect(detail.createdAt).toBe("2026-07-13T11:59:00Z");
      expect(touched.leaseExpiresAt).toBe("2026-07-13T12:05:00Z");
      expect(switched.previewUrl).toContain("serial=USB-123");
      expect(stopped).toMatchObject({
        status: "stopped",
        stoppedAt: "2026-07-13T12:10:00Z",
      });
      expect(fetchMock.mock.calls.map(([url, init]) => ({
        url,
        method: init?.method ?? "GET",
        body: init?.body,
      }))).toMatchObject([
        {
          url: "https://api.example.test/api/issues/issue-1/preview-sessions",
          method: "GET",
        },
        {
          url: "https://api.example.test/api/issues/issue-1/preview-sessions",
          method: "POST",
          body: JSON.stringify({
            preview_url: "https://preview.example.test/checkout",
            platform: "web",
            provider: "external_web",
          }),
        },
        {
          url: "https://api.example.test/api/preview-sessions/preview-1",
          method: "GET",
        },
        {
          url: "https://api.example.test/api/preview-sessions/preview-1/touch",
          method: "POST",
        },
        {
          url: "https://api.example.test/api/preview-sessions/preview-1/device",
          method: "POST",
          body: JSON.stringify({ serial: "USB-123", confirmed: true }),
        },
        {
          url: "https://api.example.test/api/preview-sessions/preview-1/stop",
          method: "POST",
        },
      ]);
    });

    it("falls back to an empty list when the response is malformed", async () => {
      vi.stubGlobal(
        "fetch",
        vi.fn().mockResolvedValue(
          new Response(
            JSON.stringify({ preview_sessions: null, total: "one" }),
            { status: 200, headers: { "Content-Type": "application/json" } },
          ),
        ),
      );

      const client = new ApiClient("https://api.example.test");

      await expect(client.listPreviewSessions("issue-1")).resolves.toEqual({
        previewSessions: [],
        total: 0,
      });
    });
  });

  it("preserves HTTP status on failed requests", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(JSON.stringify({ error: "workspace slug already exists" }), {
          status: 409,
          statusText: "Conflict",
          headers: { "Content-Type": "application/json" },
        }),
      ),
    );

    const client = new ApiClient("https://api.example.test");

    try {
      await client.createWorkspace({ name: "Test", slug: "test" });
      throw new Error("expected createWorkspace to fail");
    } catch (error) {
      expect(error).toBeInstanceOf(ApiError);
      expect(error).toMatchObject({
        message: "workspace slug already exists",
        status: 409,
        statusText: "Conflict",
      });
    }
  });

  it("uses the expected HTTP contract for autopilot endpoints", async () => {
    const fetchMock = vi.fn().mockImplementation(() => Promise.resolve(
      new Response(JSON.stringify({ autopilots: [], runs: [], total: 0 }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    ));
    vi.stubGlobal("fetch", fetchMock);

    const client = new ApiClient("https://api.example.test");

    await client.listAutopilots({ status: "active" });
    await client.getAutopilot("ap-1");
    await client.createAutopilot({
      title: "Daily triage",
      project_id: "project-1",
      assignee_id: "agent-1",
      execution_mode: "create_issue",
    });
    await client.updateAutopilot("ap-1", { status: "paused", project_id: null });
    await client.deleteAutopilot("ap-1");
    await client.triggerAutopilot("ap-1");
    await client.listAutopilotRuns("ap-1", { limit: 10, offset: 20 });
    await client.createAutopilotTrigger("ap-1", {
      kind: "schedule",
      cron_expression: "0 9 * * *",
      timezone: "UTC",
    });
    await client.updateAutopilotTrigger("ap-1", "tr-1", { enabled: false });
    await client.deleteAutopilotTrigger("ap-1", "tr-1");
    await client.rotateAutopilotTriggerWebhookToken("ap-1", "tr-1");

    const calls = fetchMock.mock.calls.map(([url, init]) => ({
      url,
      method: init?.method ?? "GET",
      body: init?.body,
    }));

    expect(calls).toMatchObject([
      { url: "https://api.example.test/api/autopilots?status=active", method: "GET" },
      { url: "https://api.example.test/api/autopilots/ap-1", method: "GET" },
      {
        url: "https://api.example.test/api/autopilots",
        method: "POST",
        body: JSON.stringify({
          title: "Daily triage",
          project_id: "project-1",
          assignee_id: "agent-1",
          execution_mode: "create_issue",
        }),
      },
      {
        url: "https://api.example.test/api/autopilots/ap-1",
        method: "PATCH",
        body: JSON.stringify({ status: "paused", project_id: null }),
      },
      { url: "https://api.example.test/api/autopilots/ap-1", method: "DELETE" },
      { url: "https://api.example.test/api/autopilots/ap-1/trigger", method: "POST" },
      { url: "https://api.example.test/api/autopilots/ap-1/runs?limit=10&offset=20", method: "GET" },
      {
        url: "https://api.example.test/api/autopilots/ap-1/triggers",
        method: "POST",
        body: JSON.stringify({
          kind: "schedule",
          cron_expression: "0 9 * * *",
          timezone: "UTC",
        }),
      },
      {
        url: "https://api.example.test/api/autopilots/ap-1/triggers/tr-1",
        method: "PATCH",
        body: JSON.stringify({ enabled: false }),
      },
      { url: "https://api.example.test/api/autopilots/ap-1/triggers/tr-1", method: "DELETE" },
      {
        url: "https://api.example.test/api/autopilots/ap-1/triggers/tr-1/rotate-webhook-token",
        method: "POST",
      },
    ]);
  });

  it("emits X-Client-* headers when identity is configured", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify([]), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    const client = new ApiClient("https://api.example.test", {
      identity: { platform: "desktop", version: "1.2.3", os: "macos" },
    });
    await client.listWorkspaces();

    const headers = fetchMock.mock.calls[0]![1]!.headers as Record<string, string>;
    expect(headers["X-Client-Platform"]).toBe("desktop");
    expect(headers["X-Client-Version"]).toBe("1.2.3");
    expect(headers["X-Client-OS"]).toBe("macos");
  });

  it("omits X-Client-* headers when identity is not configured", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify([]), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    const client = new ApiClient("https://api.example.test");
    await client.listWorkspaces();

    const headers = fetchMock.mock.calls[0]![1]!.headers as Record<string, string>;
    expect(headers["X-Client-Platform"]).toBeUndefined();
    expect(headers["X-Client-Version"]).toBeUndefined();
    expect(headers["X-Client-OS"]).toBeUndefined();
  });

  it("uses the expected HTTP contract for comment trigger preview and suppress", async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ agents: [] }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
      )
      .mockResolvedValueOnce(
        new Response(JSON.stringify({
          id: "comment-1",
          issue_id: "issue-1",
          author_type: "member",
          author_id: "user-1",
          content: "hello",
          type: "comment",
          parent_id: null,
          reactions: [],
          attachments: [],
          created_at: "2026-06-05T00:00:00Z",
          updated_at: "2026-06-05T00:00:00Z",
        }), {
          status: 201,
          headers: { "Content-Type": "application/json" },
        }),
      )
      .mockResolvedValueOnce(
        new Response(JSON.stringify({
          id: "comment-1",
          issue_id: "issue-1",
          author_type: "member",
          author_id: "user-1",
          content: "updated",
          type: "comment",
          parent_id: null,
          reactions: [],
          attachments: [],
          created_at: "2026-06-05T00:00:00Z",
          updated_at: "2026-06-05T00:01:00Z",
        }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
      );
    vi.stubGlobal("fetch", fetchMock);

    const client = new ApiClient("https://api.example.test");
    await client.previewCommentTriggers("issue-1", "hello", "parent-1", "comment-1");
    await client.createComment(
      "issue-1",
      "hello",
      "comment",
      "parent-1",
      ["attachment-1"],
      ["agent-1"],
    );
    await client.updateComment("comment-1", "updated", ["attachment-1"], ["agent-1"]);

    expect(fetchMock.mock.calls.map(([url, init]) => ({
      url,
      method: init?.method,
      body: init?.body,
    }))).toMatchObject([
      {
        url: "https://api.example.test/api/issues/issue-1/comments/trigger-preview",
        method: "POST",
        body: JSON.stringify({ content: "hello", parent_id: "parent-1", editing_comment_id: "comment-1" }),
      },
      {
        url: "https://api.example.test/api/issues/issue-1/comments",
        method: "POST",
        body: JSON.stringify({
          content: "hello",
          type: "comment",
          parent_id: "parent-1",
          attachment_ids: ["attachment-1"],
          suppress_agent_ids: ["agent-1"],
        }),
      },
      {
        url: "https://api.example.test/api/comments/comment-1",
        method: "PUT",
        body: JSON.stringify({
          content: "updated",
          attachment_ids: ["attachment-1"],
          suppress_agent_ids: ["agent-1"],
        }),
      },
    ]);
  });

  it("uses the Cloud Runtime node API contract", async () => {
    const node = {
      id: "node-1",
      owner_id: "user-1",
      instance_id: "i-0123456789abcdef0",
      region: "us-west-2",
      instance_type: "g5.xlarge",
      image_id: "ami-1",
      subnet_id: "subnet-1",
      name: "gpu-dev-01",
      status: "launching",
      tags: {},
      metadata: {},
      created_at: "2026-05-21T08:30:00Z",
      updated_at: "2026-05-21T08:30:00Z",
    };
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(
        new Response(JSON.stringify([]), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
      )
      .mockResolvedValueOnce(
        new Response(JSON.stringify(node), {
          status: 201,
          headers: { "Content-Type": "application/json" },
        }),
      );
    vi.stubGlobal("fetch", fetchMock);

    const client = new ApiClient("https://api.example.test");
    await client.listCloudRuntimeNodes({ limit: 20, offset: 5 });
    await client.createCloudRuntimeNode(
      { instance_type: "g5.xlarge", name: "gpu-dev-01" },
    );

    const listCall = fetchMock.mock.calls[0]!;
    const createCall = fetchMock.mock.calls[1]!;
    expect(listCall[0]).toBe(
      "https://api.example.test/api/cloud-runtime/nodes?limit=20&offset=5",
    );
    expect(createCall[0]).toBe(
      "https://api.example.test/api/cloud-runtime/nodes",
    );
    expect(createCall[1]).toMatchObject({
      method: "POST",
      body: JSON.stringify({
        instance_type: "g5.xlarge",
        name: "gpu-dev-01",
      }),
    });
  });

  it("falls back when Cloud Runtime node responses drift", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(
        new Response(JSON.stringify([{ id: 123 }]), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
      )
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ id: 123 }), {
          status: 201,
          headers: { "Content-Type": "application/json" },
        }),
      );
    vi.stubGlobal("fetch", fetchMock);

    const client = new ApiClient("https://api.example.test");

    await expect(client.listCloudRuntimeNodes()).resolves.toEqual([]);
    await expect(
      client.createCloudRuntimeNode({ instance_type: "g5.xlarge" }),
    ).resolves.toMatchObject({ id: "", status: "" });
  });

  it("deleteCloudRuntimeNode sends DELETE with JSON body containing instance id", async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(
      new Response(null, { status: 204 }),
    );
    vi.stubGlobal("fetch", fetchMock);

    const client = new ApiClient("https://api.example.test");
    await client.deleteCloudRuntimeNode("i-0123456789abcdef0");

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, opts] = fetchMock.mock.calls[0]!;
    expect(url).toBe("https://api.example.test/api/cloud-runtime/nodes");
    expect(opts).toMatchObject({
      method: "DELETE",
      body: JSON.stringify({ instance_id: "i-0123456789abcdef0" }),
    });
    expect((opts.headers as Record<string, string>)["Content-Type"]).toBe(
      "application/json",
    );
  });

  describe("getAttachment", () => {
    it("returns the parsed attachment for a well-formed response", async () => {
      vi.stubGlobal(
        "fetch",
        vi.fn().mockResolvedValue(
          new Response(
            JSON.stringify({
              id: "att-1",
              workspace_id: "ws-1",
              issue_id: null,
              comment_id: null,
              uploader_type: "member",
              uploader_id: "u-1",
              filename: "report.md",
              url: "https://static.example.test/ws/att-1.md",
              download_url:
                "https://static.example.test/ws/att-1.md?Policy=p&Signature=s&Key-Pair-Id=k",
              content_type: "text/markdown",
              size_bytes: 123,
              created_at: "2026-05-11T00:00:00Z",
            }),
            { status: 200, headers: { "Content-Type": "application/json" } },
          ),
        ),
      );

      const client = new ApiClient("https://api.example.test");
      const att = await client.getAttachment("att-1");

      expect(att.id).toBe("att-1");
      expect(att.download_url).toContain("Policy=");
    });

    it("falls back to an empty attachment when the response is missing download_url", async () => {
      vi.stubGlobal(
        "fetch",
        vi.fn().mockResolvedValue(
          new Response(JSON.stringify({ id: "att-1" }), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          }),
        ),
      );

      const client = new ApiClient("https://api.example.test");
      const att = await client.getAttachment("att-1");

      // parseWithFallback returns the EMPTY_ATTACHMENT record so callers can
      // safely read `download_url` without crashing — they'll see "" and
      // surface a user-facing error instead of opening `undefined`.
      expect(att.id).toBe("");
      expect(att.download_url).toBe("");
    });
  });

  describe("favorites", () => {
    it("uses the expected category, favorite, move, and unfavorite contracts", async () => {
      const attachment = {
        id: "att-1",
        workspace_id: "ws-1",
        issue_id: "issue-1",
        comment_id: "comment-1",
        chat_session_id: null,
        chat_message_id: null,
        uploader_type: "agent",
        uploader_id: "agent-1",
        filename: "report.md",
        url: "/uploads/report.md",
        download_url: "/api/attachments/att-1/download",
        markdown_url: "/api/attachments/att-1/download",
        content_type: "text/markdown",
        size_bytes: 123,
        created_at: "2026-07-20T00:00:00Z",
      };
      const category = {
        id: "cat-1",
        workspace_id: "ws-1",
        user_id: "user-1",
        name: "Default",
        is_default: true,
        favorite_count: 1,
        created_at: "2026-07-21T00:00:00Z",
        updated_at: "2026-07-21T00:00:00Z",
      };
      const favorite = {
        id: "favorite-1",
        workspace_id: "ws-1",
        user_id: "user-1",
        item_type: "attachment",
        item_id: "att-1",
        attachment,
        category,
        created_at: "2026-07-21T00:00:00Z",
      };
      const fetchMock = vi
        .fn()
        .mockResolvedValueOnce(
          new Response(JSON.stringify([category]), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          }),
        )
        .mockResolvedValueOnce(
          new Response(JSON.stringify(category), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          }),
        )
        .mockResolvedValueOnce(
          new Response(JSON.stringify({ ...category, name: "Sources" }), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          }),
        )
        .mockResolvedValueOnce(new Response(null, { status: 204 }))
        .mockResolvedValueOnce(
          new Response(JSON.stringify([favorite]), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          }),
        )
        .mockResolvedValueOnce(
          new Response(JSON.stringify(favorite), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          }),
        )
        .mockResolvedValueOnce(
          new Response(JSON.stringify({
            ...favorite,
            category: { ...category, id: "cat-2", name: "Research", is_default: false },
          }), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          }),
        )
        .mockResolvedValueOnce(new Response(null, { status: 204 }));
      vi.stubGlobal("fetch", fetchMock);

      const client = new ApiClient("https://api.example.test");
      expect((await client.listFavoriteCategories())[0]?.isDefault).toBe(true);
      expect((await client.createFavoriteCategory("Default"))?.workspaceId).toBe("ws-1");
      expect((await client.updateFavoriteCategory("cat-1", "Sources")).name).toBe("Sources");
      await client.deleteFavoriteCategory("cat-1");
      expect(await client.listFavorites()).toHaveLength(1);
      const created = await client.putFavorite("attachment", "att-1");
      expect(created.itemType === "attachment" && created.attachment.id).toBe("att-1");
      expect((await client.moveFavorite("attachment", "att-1", "cat-2")).category.name).toBe("Research");
      await client.deleteFavorite("attachment", "att-1");

      expect(fetchMock.mock.calls.map(([url, init]) => ({
        url,
        method: init?.method ?? "GET",
        body: init?.body,
      }))).toEqual([
        { url: "https://api.example.test/api/favorite-categories", method: "GET", body: undefined },
        { url: "https://api.example.test/api/favorite-categories", method: "POST", body: JSON.stringify({ name: "Default" }) },
        { url: "https://api.example.test/api/favorite-categories/cat-1", method: "PATCH", body: JSON.stringify({ name: "Sources" }) },
        { url: "https://api.example.test/api/favorite-categories/cat-1", method: "DELETE", body: undefined },
        { url: "https://api.example.test/api/favorites", method: "GET", body: undefined },
        { url: "https://api.example.test/api/favorites/attachment/att-1", method: "PUT", body: undefined },
        { url: "https://api.example.test/api/favorites/attachment/att-1", method: "PATCH", body: JSON.stringify({ category_id: "cat-2" }) },
        { url: "https://api.example.test/api/favorites/attachment/att-1", method: "DELETE", body: undefined },
      ]);
    });

    it("falls back safely for malformed category and favorite lists", async () => {
      const fetchMock = vi
        .fn()
        .mockResolvedValueOnce(
          new Response(JSON.stringify({ categories: null }), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          }),
        )
        .mockResolvedValueOnce(
          new Response(JSON.stringify({ attachments: null }), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          }),
        );
      vi.stubGlobal("fetch", fetchMock);

      const client = new ApiClient("https://api.example.test");
      expect(await client.listFavoriteCategories()).toEqual([]);
      expect(await client.listFavorites()).toEqual([]);
    });

    it("falls back safely for a malformed updated category", async () => {
      vi.stubGlobal(
        "fetch",
        vi.fn().mockResolvedValue(
          new Response(JSON.stringify({ id: null, name: "Broken" }), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          }),
        ),
      );

      const client = new ApiClient("https://api.example.test");
      expect((await client.updateFavoriteCategory("cat-1", "Broken")).id).toBe("");
    });
  });

  describe("issue and project favorites", () => {
    const category = {
      id: "cat-1",
      workspace_id: "ws-1",
      user_id: "user-1",
      name: "Default",
      is_default: true,
      favorite_count: 1,
      created_at: "2026-07-21T00:00:00Z",
      updated_at: "2026-07-21T00:00:00Z",
    };
    const favorite = {
      id: "favorite-1",
      workspace_id: "ws-1",
      user_id: "user-1",
      item_type: "issue",
      item_id: "issue-1",
      category,
      created_at: "2026-07-21T00:00:00Z",
    };

    it("uses the expected list, create, move, and delete contracts", async () => {
      const fetchMock = vi
        .fn()
        .mockResolvedValueOnce(
          new Response(JSON.stringify([favorite]), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          }),
        )
        .mockResolvedValueOnce(
          new Response(JSON.stringify(favorite), {
            status: 201,
            headers: { "Content-Type": "application/json" },
          }),
        )
        .mockResolvedValueOnce(
          new Response(
            JSON.stringify({
              ...favorite,
              category: { ...category, id: "cat-2", name: "Research" },
            }),
            {
              status: 200,
              headers: { "Content-Type": "application/json" },
            },
          ),
        )
        .mockResolvedValueOnce(new Response(null, { status: 204 }));
      vi.stubGlobal("fetch", fetchMock);

      const client = new ApiClient("https://api.example.test");
      expect((await client.listFavorites())[0]?.itemType).toBe("issue");
      expect(
        (await client.putFavorite("issue", "issue-1")).category.isDefault,
      ).toBe(true);
      expect(
        (await client.moveFavorite("issue", "issue-1", "cat-2")).category.name,
      ).toBe("Research");
      await client.deleteFavorite("issue", "issue-1");

      expect(fetchMock.mock.calls.map(([url, init]) => ({
        url,
        method: init?.method ?? "GET",
        body: init?.body,
      }))).toEqual([
        { url: "https://api.example.test/api/favorites", method: "GET", body: undefined },
        {
          url: "https://api.example.test/api/favorites/issue/issue-1",
          method: "PUT",
          body: undefined,
        },
        {
          url: "https://api.example.test/api/favorites/issue/issue-1",
          method: "PATCH",
          body: JSON.stringify({ category_id: "cat-2" }),
        },
        {
          url: "https://api.example.test/api/favorites/issue/issue-1",
          method: "DELETE",
          body: undefined,
        },
      ]);
    });

    it("falls back safely for a malformed favorite list", async () => {
      vi.stubGlobal(
        "fetch",
        vi.fn().mockResolvedValue(
          new Response(JSON.stringify({ favorites: null }), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          }),
        ),
      );

      const client = new ApiClient("https://api.example.test");
      expect(await client.listFavorites()).toEqual([]);
    });
  });

  describe("getAttachmentTextContent", () => {
    it("returns body text and the original content type from the X-* header", async () => {
      vi.stubGlobal(
        "fetch",
        vi.fn().mockResolvedValue(
          new Response("# heading\n\nbody\n", {
            status: 200,
            headers: {
              "Content-Type": "text/plain; charset=utf-8",
              "X-Original-Content-Type": "text/markdown",
            },
          }),
        ),
      );

      const client = new ApiClient("https://api.example.test");
      const { text, originalContentType } =
        await client.getAttachmentTextContent("att-1");

      expect(text).toBe("# heading\n\nbody\n");
      expect(originalContentType).toBe("text/markdown");
    });

    it("throws PreviewTooLargeError on 413", async () => {
      const { PreviewTooLargeError } = await import("./client");
      vi.stubGlobal(
        "fetch",
        vi.fn().mockResolvedValue(
          new Response("", { status: 413, statusText: "Payload Too Large" }),
        ),
      );

      const client = new ApiClient("https://api.example.test");
      await expect(client.getAttachmentTextContent("att-1")).rejects.toBeInstanceOf(
        PreviewTooLargeError,
      );
    });

    it("throws PreviewUnsupportedError on 415", async () => {
      const { PreviewUnsupportedError } = await import("./client");
      vi.stubGlobal(
        "fetch",
        vi.fn().mockResolvedValue(
          new Response("", { status: 415, statusText: "Unsupported Media Type" }),
        ),
      );

      const client = new ApiClient("https://api.example.test");
      await expect(client.getAttachmentTextContent("att-1")).rejects.toBeInstanceOf(
        PreviewUnsupportedError,
      );
    });
  });

  describe("listChatMessagesPage deployment-order fallback", () => {
    const jsonResponse = (body: unknown, status: number, statusText = "") =>
      new Response(JSON.stringify(body), {
        status,
        statusText,
        headers: { "Content-Type": "application/json" },
      });

    it("falls back to the legacy full-list endpoint when the paged route 404s", async () => {
      const legacy = [
        {
          id: "m1",
          chat_session_id: "session-1",
          role: "user",
          content: "hi",
          task_id: null,
          created_at: "2026-06-01T00:00:00Z",
        },
        {
          id: "m2",
          chat_session_id: "session-1",
          role: "assistant",
          content: "yo",
          task_id: null,
          created_at: "2026-06-01T00:00:01Z",
        },
      ];
      const fetchMock = vi
        .fn()
        .mockResolvedValueOnce(jsonResponse({ error: "not found" }, 404, "Not Found"))
        .mockResolvedValueOnce(jsonResponse(legacy, 200));
      vi.stubGlobal("fetch", fetchMock);

      const client = new ApiClient("https://api.example.test");
      const page = await client.listChatMessagesPage("session-1", { limit: 50 });

      expect(fetchMock).toHaveBeenCalledTimes(2);
      expect(fetchMock.mock.calls[0]![0]).toBe(
        "https://api.example.test/api/chat/sessions/session-1/messages/page?limit=50",
      );
      expect(fetchMock.mock.calls[1]![0]).toBe(
        "https://api.example.test/api/chat/sessions/session-1/messages",
      );
      expect(page).toEqual({ messages: legacy, limit: 50, has_more: false, next_cursor: null });
    });

    it("does NOT fall back on a cursor request — a 404 there propagates", async () => {
      const fetchMock = vi
        .fn()
        .mockResolvedValue(jsonResponse({ error: "not found" }, 404, "Not Found"));
      vi.stubGlobal("fetch", fetchMock);

      const client = new ApiClient("https://api.example.test");
      await expect(
        client.listChatMessagesPage("session-1", {
          before: { created_at: "2026-06-01T00:00:00Z", id: "m1" },
        }),
      ).rejects.toBeInstanceOf(ApiError);
      // Only the paged request fires; no legacy full-list call that would duplicate messages.
      expect(fetchMock).toHaveBeenCalledTimes(1);
    });

    it("propagates non-404 errors instead of masking them with the legacy list", async () => {
      const fetchMock = vi
        .fn()
        .mockResolvedValue(jsonResponse({ error: "boom" }, 500, "Internal Server Error"));
      vi.stubGlobal("fetch", fetchMock);

      const client = new ApiClient("https://api.example.test");
      await expect(client.listChatMessagesPage("session-1")).rejects.toMatchObject({
        status: 500,
      });
      expect(fetchMock).toHaveBeenCalledTimes(1);
    });

    it("falls back safely when the paged response is malformed", async () => {
      const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ messages: null }, 200));
      vi.stubGlobal("fetch", fetchMock);

      const client = new ApiClient("https://api.example.test");
      await expect(client.listChatMessagesPage("session-1", { limit: 25 })).resolves.toEqual({
        messages: [],
        limit: 50,
        has_more: false,
      });
    });
  });

  describe("chat message feedback", () => {
    it("parses current feedback and ignores unknown sentiment values", async () => {
      const fetchMock = vi.fn().mockResolvedValue(
        new Response(JSON.stringify([
          {
            id: "message-1",
            chat_session_id: "session-1",
            role: "assistant",
            content: "Answer",
            task_id: null,
            created_at: "2026-07-23T00:00:00Z",
            feedback: { sentiment: "future-value", comment: null },
          },
        ]), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
      );
      vi.stubGlobal("fetch", fetchMock);

      const client = new ApiClient("https://api.example.test");
      const messages = await client.listChatMessages("session-1");

      expect(messages).toHaveLength(1);
      expect(messages[0]?.feedback).toEqual({ sentiment: null, comment: "" });
    });

    it("submits sentiment and comment for one assistant message", async () => {
      const fetchMock = vi.fn().mockResolvedValue(
        new Response(null, { status: 204 }),
      );
      vi.stubGlobal("fetch", fetchMock);

      const client = new ApiClient("https://api.example.test");
      await client.upsertChatMessageFeedback("session-1", "message-1", {
        sentiment: "negative",
        comment: "The answer missed the requested format.",
      });

      expect(fetchMock).toHaveBeenCalledWith(
        "https://api.example.test/api/chat/sessions/session-1/messages/message-1/feedback",
        expect.objectContaining({
          method: "PUT",
          body: JSON.stringify({
            sentiment: "negative",
            comment: "The answer missed the requested format.",
          }),
        }),
      );
    });

  });

  describe("cancelTaskById response parsing", () => {
    const taskResponse = {
      id: "task-1",
      agent_id: "agent-1",
      runtime_id: "runtime-1",
      issue_id: "",
      status: "cancelled",
      priority: 0,
      dispatched_at: null,
      started_at: null,
      completed_at: "2026-06-12T06:40:00Z",
      result: null,
      error: null,
      created_at: "2026-06-12T06:39:00Z",
    };

    it("parses the cancelled chat message payload", async () => {
      const fetchMock = vi.fn().mockResolvedValue(
        new Response(JSON.stringify({
          ...taskResponse,
          cancelled_chat_message: {
            chat_session_id: "session-1",
            message_id: "message-1",
            content: "restore me",
            restore_to_input: true,
          },
        }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
      );
      vi.stubGlobal("fetch", fetchMock);

      const client = new ApiClient("https://api.example.test");
      const result = await client.cancelTaskById("task-1");

      expect(fetchMock.mock.calls[0]).toMatchObject([
        "https://api.example.test/api/tasks/task-1/cancel",
        { method: "POST" },
      ]);
      expect(result.cancelled_chat_message).toEqual({
        chat_session_id: "session-1",
        message_id: "message-1",
        content: "restore me",
        restore_to_input: true,
      });
    });

    it("treats a null cancelled chat message as absent", async () => {
      vi.stubGlobal(
        "fetch",
        vi.fn().mockResolvedValue(
          new Response(JSON.stringify({
            ...taskResponse,
            cancelled_chat_message: null,
          }), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          }),
        ),
      );

      const client = new ApiClient("https://api.example.test");
      const result = await client.cancelTaskById("task-1");

      expect(result.id).toBe("task-1");
      expect(result.cancelled_chat_message).toBeUndefined();
    });

    it.each([
      ["a missing task id", { ...taskResponse, id: undefined }],
      [
        "a malformed cancelled chat message",
        {
          ...taskResponse,
          cancelled_chat_message: {
            chat_session_id: "session-1",
            message_id: "message-1",
            content: "restore me",
            restore_to_input: "true",
          },
        },
      ],
      ["a null body", null],
    ])("falls back for %s", async (_label, body) => {
      vi.stubGlobal(
        "fetch",
        vi.fn().mockResolvedValue(
          new Response(JSON.stringify(body), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          }),
        ),
      );

      const client = new ApiClient("https://api.example.test");
      const result = await client.cancelTaskById("task-1");

      expect(result.id).toBe("");
      expect(result.cancelled_chat_message).toBeUndefined();
    });
  });

  describe("chat attachment wiring", () => {
    it("uploadFile includes chat_session_id in the FormData body", async () => {
      const fetchMock = vi.fn().mockResolvedValue(
        new Response(JSON.stringify({ id: "att-1", url: "https://cdn/x" }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
      );
      vi.stubGlobal("fetch", fetchMock);

      const client = new ApiClient("https://api.example.test");
      const file = new File(["hi"], "hi.png", { type: "image/png" });
      await client.uploadFile(file, { chatSessionId: "session-123" });

      expect(fetchMock).toHaveBeenCalledTimes(1);
      const [url, init] = fetchMock.mock.calls[0]!;
      expect(url).toBe("https://api.example.test/api/upload-file");
      expect(init?.method).toBe("POST");
      const body = init?.body as FormData;
      expect(body).toBeInstanceOf(FormData);
      expect(body.get("chat_session_id")).toBe("session-123");
      expect(body.get("issue_id")).toBeNull();
      expect(body.get("comment_id")).toBeNull();
    });

    it("sendChatMessage serialises attachment_ids onto the JSON body when present", async () => {
      const fetchMock = vi.fn().mockResolvedValue(
        new Response(JSON.stringify({ message_id: "m1", task_id: "t1", created_at: "" }), {
          status: 201,
          headers: { "Content-Type": "application/json" },
        }),
      );
      vi.stubGlobal("fetch", fetchMock);

      const client = new ApiClient("https://api.example.test");
      await client.sendChatMessage("session-1", "hello", ["att-1", "att-2"]);

      const [, init] = fetchMock.mock.calls[0]!;
      expect(JSON.parse(init?.body as string)).toEqual({
        content: "hello",
        attachment_ids: ["att-1", "att-2"],
      });
    });

    it("sendChatMessage omits attachment_ids when the list is empty or undefined", async () => {
      const fetchMock = vi.fn().mockImplementation(() =>
        Promise.resolve(
          new Response(JSON.stringify({ message_id: "m1", task_id: "t1", created_at: "" }), {
            status: 201,
            headers: { "Content-Type": "application/json" },
          }),
        ),
      );
      vi.stubGlobal("fetch", fetchMock);

      const client = new ApiClient("https://api.example.test");
      await client.sendChatMessage("session-1", "hello");
      await client.sendChatMessage("session-1", "again", []);

      expect(JSON.parse(fetchMock.mock.calls[0]![1]?.body as string)).toEqual({ content: "hello" });
      expect(JSON.parse(fetchMock.mock.calls[1]![1]?.body as string)).toEqual({ content: "again" });
    });
  });
});
