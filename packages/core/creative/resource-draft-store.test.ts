import { beforeEach, expect, it } from "vitest";
import { applyCreativeResourceChanges, creativeResourceDraftKey, hasCreativeResourceChanges, useCreativeResourceDraftStore } from "./resource-draft-store";

beforeEach(() => useCreativeResourceDraftStore.setState({ changes: {} }));
it("preserves only local edits while accepting refreshed server fields", () => {
  const key = creativeResourceDraftKey("ws", "market");
  const before = { brand: "Original", prime_composition_mode: "deterministic" };
  useCreativeResourceDraftStore.getState().update(key, before, { ...before, prime_composition_mode: "model_integrated" });
  const changes = useCreativeResourceDraftStore.getState().changes[key]!;
  expect(Object.keys(changes)).toEqual(["prime_composition_mode"]);
  expect(applyCreativeResourceChanges({ ...before, brand: "Updated" }, changes)).toEqual({ brand: "Updated", prime_composition_mode: "model_integrated" });
  expect(hasCreativeResourceChanges(before, changes)).toBe(true);
  expect(useCreativeResourceDraftStore.getState().changes[creativeResourceDraftKey("other", "market")]).toBeUndefined();
});
it("retains edits made during a save and persists field removal", () => {
  const store = useCreativeResourceDraftStore.getState();
  const before = { prime_composition_mode: "model_integrated", prime_model_template_family: "light" };
  const after = { prime_composition_mode: "deterministic" };
  store.update("ws:market", before, after);
  const submitted = useCreativeResourceDraftStore.getState().changes["ws:market"]!;
  expect(applyCreativeResourceChanges(before, JSON.parse(JSON.stringify(submitted)))).toEqual(after);
  store.update("ws:market", after, { prime_composition_mode: "model_integrated" });
  store.clear("ws:market", submitted);
  expect(applyCreativeResourceChanges(after, useCreativeResourceDraftStore.getState().changes["ws:market"]!)).toEqual({ prime_composition_mode: "model_integrated" });
});
