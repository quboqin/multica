import { expect, it } from "vitest";
import { CreativeOrderItemSchema, CreativeOrderSchema } from "../api/schemas";

it("accepts missing candidate progress from older servers", () => {
  expect(CreativeOrderItemSchema.parse({ id: "item" }).candidate_progress).toBeUndefined();
});
it("drops malformed candidate progress without losing the order item", () => {
  const item=CreativeOrderItemSchema.parse({ id:"item", candidate_progress: { state:"selection_ready", target:"six" } });
  expect(item.id).toBe("item");
  expect(item.candidate_progress).toBeUndefined();
});
it("preserves valid counts and unknown states at the API boundary", () => {
  const p={ state:"future", target:6, expected:8, planned:6, generated:6, primed:5, settled:5 };
  expect(CreativeOrderItemSchema.parse({ candidate_progress:p }).candidate_progress).toMatchObject(p);
});

it("keeps order data when recovery history is malformed", () => {
  const order = CreativeOrderSchema.parse({ id: "order", recoveries: [{ attempt: "three" }] });
  expect(order.id).toBe("order");
  expect(order.recoveries).toBeUndefined();
});

it("accepts missing recovery history and future recovery states", () => {
  expect(CreativeOrderSchema.parse({ id: "order" }).recoveries).toBeUndefined();
  expect(CreativeOrderSchema.parse({ recoveries: [{ id: "recovery", status: "future" }] }).recoveries?.[0]?.status).toBe("future");
});
