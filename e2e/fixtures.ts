/**
 * TestApiClient — lightweight API helper for E2E test data setup/teardown.
 *
 * Uses raw fetch so E2E tests have zero build-time coupling to the web app.
 */

import "./env";
import pg from "pg";

// `||` (not `??`) so an empty `NEXT_PUBLIC_API_URL=` in .env still falls
// back to localhost. dotenv sets unset-vs-empty both as "" — treating them
// the same matches user intent.
const API_BASE = process.env.NEXT_PUBLIC_API_URL || `http://localhost:${process.env.PORT || "8080"}`;
const DATABASE_URL = process.env.DATABASE_URL ?? "postgres://multica:multica@localhost:5432/multica?sslmode=disable";

export interface TestWorkspace {
  id: string;
  name: string;
  slug: string;
}

export type TestCollectionFieldType = "text" | "number" | "checkbox";

export interface TestCollectionField {
  id: string;
  name: string;
  type: TestCollectionFieldType;
}

export interface TestCollection {
  id: string;
  workspace_id: string;
  name: string;
  revision: number;
}

export interface TestCollectionRecord {
  id: string;
  workspace_id: string;
  collection_id: string;
  title: string;
  fields: Record<string, unknown>;
  position: number;
  revision: number;
}

export interface TestCollectionMember {
  userId: string;
  memberId: string;
  email: string;
}

export interface CollectionSideEffectCounts {
  issueCount: number;
  issueCounter: number;
  agentTaskCount: number;
  inboxCount: number;
}

export type TestIssueStatus =
  | "backlog"
  | "todo"
  | "in_progress"
  | "in_review"
  | "done"
  | "blocked"
  | "cancelled";

export type TestIssuePriority = "urgent" | "high" | "medium" | "low" | "none";

export interface TestTableIssueSeed {
  title: string;
  status?: TestIssueStatus;
  priority?: TestIssuePriority;
  parentIssueId?: string | null;
  position?: number;
}

export interface TestTableIssue {
  id: string;
  title: string;
  status: TestIssueStatus;
  number: number;
}

export class TestApiClient {
  private token: string | null = null;
  private workspaceSlug: string | null = null;
  private workspaceId: string | null = null;
  private workspace: TestWorkspace | null = null;
  private email: string | null = null;
  private createdIssueIds: string[] = [];
  private seededIssueIds: string[] = [];
  private createdCollectionIds: string[] = [];
  private createdMemberUserIds: string[] = [];

  async login(email: string, name: string) {
    const client = new pg.Client(DATABASE_URL);
    await client.connect();
    try {
      // Keep each E2E login isolated so previous test runs do not trip the
      // per-email send-code rate limit.
      await client.query("DELETE FROM verification_code WHERE email = $1", [email]);

      // Step 1: Send verification code
      const sendRes = await fetch(`${API_BASE}/auth/send-code`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ email }),
      });
      if (!sendRes.ok) {
        throw new Error(`send-code failed: ${sendRes.status}`);
      }

      // Step 2: Read code from database
      const result = await client.query(
        "SELECT code FROM verification_code WHERE email = $1 AND used = FALSE AND expires_at > now() ORDER BY created_at DESC LIMIT 1",
        [email],
      );
      if (result.rows.length === 0) {
        throw new Error(`No verification code found for ${email}`);
      }

      const configuredDevCode = process.env.MULTICA_DEV_VERIFICATION_CODE?.trim();
      const code = configuredDevCode || result.rows[0].code;

      // Step 3: Verify code to get JWT
      const verifyRes = await fetch(`${API_BASE}/auth/verify-code`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ email, code }),
      });
      if (!verifyRes.ok) {
        throw new Error(`verify-code failed: ${verifyRes.status}`);
      }
      const data = await verifyRes.json();

      this.token = data.token;
      this.email = email;

      // Update user name if needed
      if (name && data.user?.name !== name) {
        await this.authedFetch("/api/me", {
          method: "PATCH",
          body: JSON.stringify({ name }),
        });
      }

      await client.query("DELETE FROM verification_code WHERE email = $1", [email]);

      return data;
    } finally {
      await client.end();
    }
  }

  async getWorkspaces(): Promise<TestWorkspace[]> {
    const res = await this.authedFetch("/api/workspaces");
    return res.json();
  }

  setWorkspaceId(id: string) {
    this.workspaceId = id;
  }

  setWorkspaceSlug(slug: string) {
    this.workspaceSlug = slug;
  }

  async ensureWorkspace(name = "E2E Workspace", slug = "e2e-workspace") {
    const workspaces = await this.getWorkspaces();
    const workspace = workspaces.find((item) => item.slug === slug) ?? workspaces[0];
    if (workspace) {
      this.workspaceId = workspace.id;
      this.workspaceSlug = workspace.slug;
      this.workspace = workspace;
      return workspace;
    }

    const res = await this.authedFetch("/api/workspaces", {
      method: "POST",
      body: JSON.stringify({ name, slug }),
    });
    if (res.ok) {
      const created = (await res.json()) as TestWorkspace;
      this.workspaceId = created.id;
      this.workspaceSlug = created.slug;
      this.workspace = created;
      return created;
    }

    const refreshed = await this.getWorkspaces();
    const created = refreshed.find((item) => item.slug === slug) ?? refreshed[0];
    if (created) {
      this.workspaceId = created.id;
      this.workspaceSlug = created.slug;
      this.workspace = created;
      return created;
    }

    throw new Error(`Failed to ensure workspace ${slug}: ${res.status} ${res.statusText}`);
  }

  setWorkspace(workspace: TestWorkspace) {
    this.workspaceId = workspace.id;
    this.workspaceSlug = workspace.slug;
    this.workspace = workspace;
  }

  getWorkspace(): TestWorkspace {
    if (!this.workspace) {
      throw new Error("Cannot read workspace before workspace setup");
    }
    return this.workspace;
  }

  trackCollection(id: string) {
    if (!this.createdCollectionIds.includes(id)) this.createdCollectionIds.push(id);
  }

  async markUserOnboarded() {
    if (!this.email) {
      throw new Error("Cannot mark E2E user onboarded before login");
    }

    const client = new pg.Client(DATABASE_URL);
    await client.connect();
    try {
      const result = await client.query(
        `
          UPDATE "user"
          SET
            onboarded_at = COALESCE(onboarded_at, now()),
            onboarding_questionnaire = COALESCE(onboarding_questionnaire, '{}'::jsonb)
              || '{"source":["friends_colleagues"],"source_other":null,"source_skipped":false}'::jsonb
          WHERE email = $1
        `,
        [this.email],
      );
      if (result.rowCount !== 1) {
        throw new Error(`Failed to mark E2E user onboarded: ${this.email}`);
      }
    } finally {
      await client.end();
    }
  }

  async createIssue(title: string, opts?: Record<string, unknown>) {
    const res = await this.authedFetch("/api/issues", {
      method: "POST",
      body: JSON.stringify({ title, ...opts }),
    });
    const issue = await res.json();
    this.createdIssueIds.push(issue.id);
    return issue;
  }

  /**
   * Insert a large, deterministic issue fixture in one transaction.
   *
   * Browser E2E coverage for cursor-backed Table views needs 1,000+ rows,
   * which would make setup itself dominate the test if every row went through
   * the HTTP create endpoint. These rows intentionally contain no dependent
   * records; cleanup deletes exactly the returned IDs from the isolated E2E
   * workspace.
   */
  async seedTableIssues(rows: TestTableIssueSeed[]): Promise<TestTableIssue[]> {
    if (rows.length === 0) return [];
    if (!this.workspaceId || !this.email) {
      throw new Error("Cannot seed table issues before login and workspace setup");
    }

    const client = new pg.Client(DATABASE_URL);
    await client.connect();
    try {
      await client.query("BEGIN");
      const userResult = await client.query<{ id: string }>(
        `SELECT id FROM "user" WHERE email = $1`,
        [this.email],
      );
      const creatorId = userResult.rows[0]?.id;
      if (!creatorId) {
        throw new Error(`Cannot resolve E2E creator for ${this.email}`);
      }

      const counterResult = await client.query<{ issue_counter: number }>(
        `
          UPDATE workspace
          SET issue_counter = issue_counter + $2
          WHERE id = $1
          RETURNING issue_counter
        `,
        [this.workspaceId, rows.length],
      );
      const finalCounter = Number(counterResult.rows[0]?.issue_counter);
      if (!Number.isFinite(finalCounter)) {
        throw new Error(`Cannot reserve issue numbers for workspace ${this.workspaceId}`);
      }
      const firstNumber = finalCounter - rows.length + 1;

      const inserted = await client.query<TestTableIssue>(
        `
          INSERT INTO issue (
            workspace_id,
            title,
            status,
            priority,
            creator_type,
            creator_id,
            parent_issue_id,
            position,
            number
          )
          SELECT
            $1::uuid,
            fixture.title,
            fixture.status,
            fixture.priority,
            'member',
            $2::uuid,
            fixture.parent_issue_id,
            fixture.position,
            fixture.number
          FROM unnest(
            $3::text[],
            $4::text[],
            $5::text[],
            $6::uuid[],
            $7::double precision[],
            $8::integer[]
          ) WITH ORDINALITY AS fixture(
            title,
            status,
            priority,
            parent_issue_id,
            position,
            number,
            ordinal
          )
          ORDER BY fixture.ordinal
          RETURNING id, title, status, number
        `,
        [
          this.workspaceId,
          creatorId,
          rows.map((row) => row.title),
          rows.map((row) => row.status ?? "backlog"),
          rows.map((row) => row.priority ?? "none"),
          rows.map((row) => row.parentIssueId ?? null),
          rows.map((row, index) => row.position ?? index + 1),
          rows.map((_row, index) => firstNumber + index),
        ],
      );
      await client.query("COMMIT");
      this.seededIssueIds.push(...inserted.rows.map((row) => row.id));
      return inserted.rows;
    } catch (error) {
      await client.query("ROLLBACK");
      throw error;
    } finally {
      await client.end();
    }
  }

  async deleteIssue(id: string) {
    await this.authedFetch(`/api/issues/${id}`, { method: "DELETE" });
  }

  async updateIssue(id: string, updates: Record<string, unknown>) {
    const res = await this.authedFetch(`/api/issues/${id}`, {
      method: "PUT",
      body: JSON.stringify(updates),
    });
    if (!res.ok) {
      throw new Error(`update issue failed: ${res.status} ${await res.text()}`);
    }
    return res.json();
  }

  async createCollection(
    name: string,
    fields: Array<{ name: string; type: TestCollectionFieldType }>,
  ): Promise<{ collection: TestCollection; fields: TestCollectionField[] }> {
    const res = await this.authedFetch("/api/collections", {
      method: "POST",
      body: JSON.stringify({
        client_request_id: globalThis.crypto.randomUUID(),
        name,
        fields,
      }),
    });
    if (!res.ok) {
      throw new Error(`create collection failed: ${res.status} ${await res.text()}`);
    }
    const body = (await res.json()) as {
      collection: TestCollection;
      fields: TestCollectionField[];
    };
    if (!body.collection?.id || body.fields.length !== fields.length) {
      throw new Error("create collection returned an invalid response");
    }
    this.createdCollectionIds.push(body.collection.id);
    return body;
  }

  async createCollectionRecord(
    collectionId: string,
    title: string,
    fields: Record<string, unknown> = {},
  ): Promise<TestCollectionRecord> {
    const res = await this.authedFetch(`/api/collections/${collectionId}/records`, {
      method: "POST",
      body: JSON.stringify({
        client_request_id: globalThis.crypto.randomUUID(),
        title,
        fields,
      }),
    });
    if (!res.ok) {
      throw new Error(`create collection record failed: ${res.status} ${await res.text()}`);
    }
    const body = (await res.json()) as { record: TestCollectionRecord };
    if (!body.record?.id || body.record.collection_id !== collectionId) {
      throw new Error("create collection record returned an invalid response");
    }
    return body.record;
  }

  /**
   * Create a large record set through the real collection API. Keep the
   * requests bounded rather than issuing an unbounded Promise.all so a bulk
   * side-effect assertion does not become a connection-flood test.
   */
  async createCollectionRecords(
    collectionId: string,
    titlePrefix: string,
    count: number,
    concurrency = 20,
  ): Promise<TestCollectionRecord[]> {
    if (count < 0 || !Number.isInteger(count)) {
      throw new Error(`Invalid collection record count: ${count}`);
    }
    if (concurrency < 1 || !Number.isInteger(concurrency)) {
      throw new Error(`Invalid collection record concurrency: ${concurrency}`);
    }
    if (count === 0) return [];
    const results = new Array<TestCollectionRecord>(count);
    let nextIndex = 0;
    let failed = false;
    let firstError: unknown;
    const worker = async () => {
      while (true) {
        if (failed) return;
        const index = nextIndex++;
        if (index >= count) return;
        try {
          results[index] = await this.createCollectionRecord(
            collectionId,
            `${titlePrefix} ${index.toString().padStart(4, "0")}`,
          );
        } catch (error) {
          // Stop assigning new records after the first failure, but let every
          // already-started request settle before the helper rejects. The
          // caller's afterEach cleanup must run only after no worker can write
          // another record.
          if (!failed) {
            failed = true;
            firstError = error;
          }
          return;
        }
      }
    };
    await Promise.all(
      Array.from({ length: Math.min(concurrency, count) }, () => worker()),
    );
    if (failed) throw firstError;
    return results;
  }

  async updateCollectionRecord(
    collectionId: string,
    recordId: string,
    expectedRevision: number,
    change:
      | { field_id: string; op: "set"; value: unknown }
      | { field_id: string; op: "clear" },
  ): Promise<{ response: Response; record?: TestCollectionRecord }> {
    const response = await this.authedFetch(
      `/api/collections/${collectionId}/records/${recordId}`,
      {
        method: "PATCH",
        body: JSON.stringify({ expected_revision: expectedRevision, change }),
      },
    );
    if (!response.ok) return { response };
    const body = (await response.json()) as { record: TestCollectionRecord };
    return { response, record: body.record };
  }

  async getCollectionRecord(
    collectionId: string,
    recordId: string,
  ): Promise<TestCollectionRecord> {
    const response = await this.authedFetch(
      `/api/collections/${collectionId}/records/${recordId}`,
    );
    if (!response.ok) {
      throw new Error(`get collection record failed: ${response.status} ${await response.text()}`);
    }
    const body = (await response.json()) as { record: TestCollectionRecord };
    if (!body.record?.id || body.record.collection_id !== collectionId) {
      throw new Error("get collection record returned an invalid response");
    }
    return body.record;
  }

  async seedCollectionRecords(
    collectionId: string,
    records: Array<{ title: string; fields?: Record<string, unknown> }>,
  ): Promise<TestCollectionRecord[]> {
    if (records.length === 0) return [];
    if (!this.workspaceId || !this.email) {
      throw new Error("Cannot seed collection records before login and workspace setup");
    }

    const client = new pg.Client(DATABASE_URL);
    await client.connect();
    try {
      await client.query("BEGIN");
      await client.query("SELECT set_config('app.workspace_id', $1, true)", [
        this.workspaceId,
      ]);
      const userResult = await client.query<{ id: string }>(
        `SELECT id FROM "user" WHERE email = $1`,
        [this.email],
      );
      const creatorId = userResult.rows[0]?.id;
      if (!creatorId) throw new Error(`Cannot resolve E2E creator for ${this.email}`);

      const result = await client.query<TestCollectionRecord>(
        `
          INSERT INTO record (
            id,
            workspace_id,
            collection_id,
            title,
            fields,
            position,
            revision,
            created_by,
            updated_by,
            create_request_id,
            create_fingerprint,
            created_at,
            updated_at
          )
          SELECT
            gen_random_uuid(),
            $1::uuid,
            $2::uuid,
            fixture.value->>'title',
            COALESCE(fixture.value->'fields', '{}'::jsonb),
            fixture.ordinal - 1,
            1,
            $3::uuid,
            $3::uuid,
            gen_random_uuid(),
            repeat('0', 64),
            now() + ((fixture.ordinal - 1) * interval '1 microsecond'),
            now() + ((fixture.ordinal - 1) * interval '1 microsecond')
          FROM jsonb_array_elements($4::jsonb) WITH ORDINALITY AS fixture(value, ordinal)
          RETURNING id, workspace_id, collection_id, title, fields, position, revision
        `,
        [this.workspaceId, collectionId, creatorId, JSON.stringify(records)],
      );
      await client.query("COMMIT");
      return result.rows;
    } catch (error) {
      await client.query("ROLLBACK");
      throw error;
    } finally {
      await client.end();
    }
  }

  async createWorkspaceMember(
    email: string,
    name: string,
    role: "member" | "admin" = "member",
  ): Promise<TestCollectionMember> {
    if (!this.workspaceId) {
      throw new Error("Cannot create a workspace member before workspace setup");
    }
    const client = new pg.Client(DATABASE_URL);
    await client.connect();
    try {
      await client.query("BEGIN");
      await client.query("SELECT set_config('app.workspace_id', $1, true)", [
        this.workspaceId,
      ]);
      const user = await client.query<{ id: string }>(
        `
          INSERT INTO "user" (name, email)
          VALUES ($1, $2)
          ON CONFLICT (email) DO UPDATE SET name = EXCLUDED.name
          RETURNING id
        `,
        [name, email],
      );
      const userId = user.rows[0]?.id;
      if (!userId) throw new Error(`Cannot create E2E member user ${email}`);
      const member = await client.query<{ id: string }>(
        `
          INSERT INTO member (workspace_id, user_id, role)
          VALUES ($1, $2, $3)
          ON CONFLICT (workspace_id, user_id) DO UPDATE SET role = EXCLUDED.role
          RETURNING id
        `,
        [this.workspaceId, userId, role],
      );
      const memberId = member.rows[0]?.id;
      if (!memberId) throw new Error(`Cannot create E2E workspace membership for ${email}`);
      await client.query("COMMIT");
      this.createdMemberUserIds.push(userId);
      return { userId, memberId, email };
    } catch (error) {
      await client.query("ROLLBACK");
      throw error;
    } finally {
      await client.end();
    }
  }

  async readCollectionSideEffectCounts(): Promise<CollectionSideEffectCounts> {
    if (!this.workspaceId) {
      throw new Error("Cannot read side-effect counts before workspace setup");
    }
    const client = new pg.Client(DATABASE_URL);
    await client.connect();
    try {
      await client.query("SELECT set_config('app.workspace_id', $1, true)", [
        this.workspaceId,
      ]);
      const result = await client.query<{
        issue_count: string;
        issue_counter: number;
        agent_task_count: string;
        inbox_count: string;
      }>(
        `
          SELECT
            (SELECT COUNT(*) FROM issue WHERE workspace_id = $1) AS issue_count,
            (SELECT issue_counter FROM workspace WHERE id = $1) AS issue_counter,
            (
              SELECT COUNT(*)
              FROM agent_task_queue AS task
              JOIN agent ON agent.id = task.agent_id
              WHERE agent.workspace_id = $1
            ) AS agent_task_count,
            (SELECT COUNT(*) FROM inbox_item WHERE workspace_id = $1) AS inbox_count
        `,
        [this.workspaceId],
      );
      const row = result.rows[0];
      if (!row) throw new Error("No side-effect count row returned");
      return {
        issueCount: Number(row.issue_count),
        issueCounter: Number(row.issue_counter),
        agentTaskCount: Number(row.agent_task_count),
        inboxCount: Number(row.inbox_count),
      };
    } finally {
      await client.end();
    }
  }

  /** Clean up all issues created during this test. */
  async cleanup() {
    if (this.createdCollectionIds.length > 0) {
      const client = new pg.Client(DATABASE_URL);
      await client.connect();
      try {
        await client.query("BEGIN");
        await client.query("SELECT set_config('app.workspace_id', $1, true)", [
          this.workspaceId,
        ]);
        await client.query(
          `DELETE FROM record WHERE collection_id = ANY($1::uuid[])`,
          [this.createdCollectionIds],
        );
        await client.query(
          `DELETE FROM collection_field WHERE collection_id = ANY($1::uuid[])`,
          [this.createdCollectionIds],
        );
        await client.query(
          `DELETE FROM collection WHERE id = ANY($1::uuid[])`,
          [this.createdCollectionIds],
        );
        await client.query("COMMIT");
      } catch (error) {
        await client.query("ROLLBACK");
        throw error;
      } finally {
        await client.end();
      }
      this.createdCollectionIds = [];
    }
    if (this.createdMemberUserIds.length > 0 && this.workspaceId) {
      const client = new pg.Client(DATABASE_URL);
      await client.connect();
      try {
        await client.query("BEGIN");
        await client.query(
          `DELETE FROM member WHERE workspace_id = $1 AND user_id = ANY($2::uuid[])`,
          [this.workspaceId, this.createdMemberUserIds],
        );
        await client.query(
          `DELETE FROM "user" WHERE id = ANY($1::uuid[])`,
          [this.createdMemberUserIds],
        );
        await client.query("COMMIT");
      } catch (error) {
        await client.query("ROLLBACK");
        throw error;
      } finally {
        await client.end();
      }
      this.createdMemberUserIds = [];
    }
    if (this.seededIssueIds.length > 0 && this.workspaceId) {
      const client = new pg.Client(DATABASE_URL);
      await client.connect();
      try {
        await client.query(
          `DELETE FROM issue WHERE workspace_id = $1 AND id = ANY($2::uuid[])`,
          [this.workspaceId, this.seededIssueIds],
        );
      } finally {
        await client.end();
      }
      this.seededIssueIds = [];
    }
    for (const id of this.createdIssueIds) {
      try {
        await this.deleteIssue(id);
      } catch {
        /* ignore — may already be deleted */
      }
    }
    this.createdIssueIds = [];
  }

  getToken() {
    return this.token;
  }

  getEmail() {
    if (!this.email) {
      throw new Error("Test API client is not logged in");
    }
    return this.email;
  }

  private async authedFetch(path: string, init?: RequestInit) {
    const headers: Record<string, string> = {
      "Content-Type": "application/json",
      ...((init?.headers as Record<string, string>) ?? {}),
    };
    if (this.token) headers["Authorization"] = `Bearer ${this.token}`;
    if (this.workspaceSlug) headers["X-Workspace-Slug"] = this.workspaceSlug;
    else if (this.workspaceId) headers["X-Workspace-ID"] = this.workspaceId;
    return fetch(`${API_BASE}${path}`, { ...init, headers });
  }
}
