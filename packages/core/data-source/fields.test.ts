// @vitest-environment node
import { describe, expect, it } from "vitest";
import { canChangeDataSourceField } from "./fields";
import type { DataSourceField } from "./types";

const field: DataSourceField<{ locked: boolean; value?: string }> = {
  id: "caption", label: "Caption", kind: "text", value: (row) => row.value,
  sortable: true, filterable: true, groupable: false,
  canSet: (row) => !row.locked,
  canClear: (row) => !row.locked && row.value !== undefined,
};

describe("field write capabilities", () => {
  it("honors source access, row access and clear independently of set", () => {
    expect(canChangeDataSourceField(false, field, { locked: false }, { op: "set", value: "x" })).toBe(false);
    expect(canChangeDataSourceField(true, field, { locked: true, value: "x" }, { op: "clear" })).toBe(false);
    expect(canChangeDataSourceField(true, field, { locked: false }, { op: "set", value: "x" })).toBe(true);
    expect(canChangeDataSourceField(true, field, { locked: false }, { op: "clear" })).toBe(false);
    expect(canChangeDataSourceField(true, field, { locked: false, value: "x" }, { op: "clear" })).toBe(true);
  });
});
