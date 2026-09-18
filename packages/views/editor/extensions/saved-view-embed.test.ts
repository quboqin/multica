import { Editor } from "@tiptap/core";
import StarterKit from "@tiptap/starter-kit";
import { Markdown } from "@tiptap/markdown";
import { expect, it } from "vitest";
import { SavedViewEmbed } from "./saved-view-embed";
import { BlockMathExtension, InlineMathExtension } from "./math";
it("round-trips embed references with Mermaid, math, and attachment links without embedding private payloads", () => {
  const editor = new Editor({
    extensions: [
      StarterKit,
      SavedViewEmbed,
      BlockMathExtension,
      InlineMathExtension,
      Markdown,
    ],
  });
  const markdown =
    "Before\n\n:::multica-view 10000000-0000-4000-8000-000000000001\n\n```mermaid\ngraph LR\n A --> B\n```\n\n$$\nx^2\n$$\n\n[report.pdf](/api/attachments/10000000-0000-4000-8000-000000000002/download)\n";
  editor.commands.setContent(markdown, { contentType: "markdown" });
  const json = editor.getJSON();
  expect(json.content?.some((node) => node.type === "savedViewEmbed")).toBe(
    true,
  );
  const serialized = editor.getMarkdown();
  expect(serialized).toContain(
    ":::multica-view 10000000-0000-4000-8000-000000000001",
  );
  expect(serialized).toContain("```mermaid");
  expect(serialized).toContain("x^2");
  expect(serialized).toContain("report.pdf");
  editor.commands.setContent(serialized, { contentType: "markdown" });
  expect(editor.getJSON()).toEqual(json);
  editor.destroy();
});
