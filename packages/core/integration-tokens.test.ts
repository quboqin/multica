import { describe, expect, it } from "vitest";
import {
  DEFAULT_INTEGRATION_TOKEN_KEYS,
  integrationTokenEnvKey,
  integrationTokenPlaceholder,
} from "./integration-tokens";

describe("integration token helpers", () => {
  it("keeps the built-in credential keys available for profile and MCP UI", () => {
    expect(DEFAULT_INTEGRATION_TOKEN_KEYS).toEqual([
      "git_token",
      "feishu_mcp_token",
      "paones_token",
      "jingwei_token",
    ]);
  });

  it("normalizes profile token keys into env names", () => {
    expect(integrationTokenEnvKey("notion-token")).toBe("NOTION_TOKEN");
    expect(integrationTokenEnvKey(" notion token ")).toBe("NOTION_TOKEN");
  });

  it("builds MCP placeholders without duplicating the Multica prefix", () => {
    expect(integrationTokenPlaceholder("notion_token")).toBe(
      "${MULTICA_INTEGRATION_NOTION_TOKEN}",
    );
    expect(integrationTokenPlaceholder("MULTICA_INTEGRATION_NOTION_TOKEN")).toBe(
      "${MULTICA_INTEGRATION_NOTION_TOKEN}",
    );
  });
});
