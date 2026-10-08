// @vitest-environment node
import { describe, expect, it } from "vitest";
import type { Issue, IssueProperty } from "../types";
import { createIssueTableFields } from "./table-fields";

const property = (type: string): IssueProperty => ({
  id: "custom",
  workspace_id: "ws",
  name: "Category",
  type,
  config: { options: [{ id: "opt", name: "Ready", color: "#123456" }] },
  position: 0,
  archived: false,
  created_at: "",
  updated_at: "",
});

describe("task field adapter", () => {
  it("preserves task sort keys and custom field values/options", () => {
    const fields = createIssueTableFields(
      ["title", "identifier", "property:custom"],
      [property("select")],
      (id) => id,
    );
    expect(fields.map((field) => field.sortKey)).toEqual([
      "title",
      undefined,
      "property:custom",
    ]);
    expect(fields[2]?.options).toEqual([
      { id: "opt", label: "Ready", color: "#123456" },
    ]);
    expect(
      fields[2]?.value({
        id: "task",
        workspace_id: "ws",
        number: 1,
        identifier: "G1-1",
        title: "Example",
        description: null,
        status: "backlog",
        priority: "none",
        assignee_type: null,
        assignee_id: null,
        creator_type: "member",
        creator_id: "member",
        parent_issue_id: null,
        project_id: null,
        position: 1,
        stage: null,
        start_date: null,
        due_date: null,
        metadata: {},
        properties: { custom: "opt" },
        created_at: "",
        updated_at: "",
      } satisfies Issue),
    ).toBe("opt");
  });

  it.each(["multi_select", "checkbox", "actor", "multi_actor", "future_kind"])(
    "does not offer unsupported sorting for %s",
    (kind) => {
      const [field] = createIssueTableFields(
        ["property:custom"],
        [property(kind)],
        () => "Category",
      );
      expect(field?.sortKey).toBeUndefined();
      expect(field?.kind).toBe(kind === "future_kind" ? "readonly" : kind);
    },
  );

  it("keeps missing fields readable without inventing editing or sorting support", () => {
    const [field] = createIssueTableFields(
      ["property:custom"],
      [],
      () => "Unavailable",
    );
    expect(field).toMatchObject({ label: "Unavailable", kind: "readonly" });
    expect(field?.sortKey).toBeUndefined();
  });
});
