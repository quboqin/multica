import { describe, expect, it } from "vitest";
import { resolveCreativeSquad } from "./squad";

const squads = [
  { id: "engineering", name: "研发", archived_at: null },
  { id: "creative", name: "素材流程小队", archived_at: null },
  { id: "market", name: "市场", archived_at: null },
];

describe("resolveCreativeSquad", () => {
  it("selects the factory binding even after the squad is renamed", () => {
    expect(resolveCreativeSquad(squads, "market")?.id).toBe("market");
  });

  it("defaults to the only material squad among unrelated squads", () => {
    expect(resolveCreativeSquad(squads)?.id).toBe("creative");
  });

  it("preserves the single-squad default", () => {
    expect(resolveCreativeSquad([squads[0]!])?.id).toBe("engineering");
  });

  it("does not guess when multiple material squads exist", () => {
    expect(resolveCreativeSquad([...squads, { id: "other", name: "素材二队" }])).toBeUndefined();
    expect(resolveCreativeSquad([squads[0]!, squads[2]!])).toBeUndefined();
  });

  it("does not redirect a missing or archived factory binding to another squad", () => {
    expect(resolveCreativeSquad(squads, "inaccessible")).toBeUndefined();
    expect(resolveCreativeSquad([{ ...squads[1]!, archived_at: "2026-09-10" }, squads[0]!], "creative")).toBeUndefined();
  });

  it("ignores archived squads and handles an empty response", () => {
    expect(resolveCreativeSquad([{ ...squads[1]!, archived_at: "2026-09-10" }, squads[0]!])?.id).toBe("engineering");
    expect(resolveCreativeSquad([])).toBeUndefined();
  });
});
