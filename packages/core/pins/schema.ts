import { z } from "zod";

export const PinnedItemSchema = z.object({
  id: z.string(),
  workspace_id: z.string(),
  user_id: z.string(),
  item_type: z.enum(["issue", "project", "view", "collection"]),
  item_id: z.string(),
  position: z.number(),
  created_at: z.string(),
});
// Unknown future pin kinds are hidden, never interpreted as another entity.
export const PinnedItemsSchema = z
  .array(PinnedItemSchema.nullable().catch(null))
  .transform((items) => items.filter((item) => item !== null));
