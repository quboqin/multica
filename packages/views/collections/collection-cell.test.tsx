import { cleanup, fireEvent, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { renderWithI18n } from "../test/i18n";
import { CollectionFieldEditor } from "./collection-cell";

afterEach(() => { cleanup(); vi.restoreAllMocks(); });

it.each([ ["email", "person@example.com"], ["url", "https://example.com/path"] ])("edits a %s field with the matching input type", (type, value) => {
  const onChange = vi.fn();
  const onOpenChange = vi.fn();
  renderWithI18n(<CollectionFieldEditor
    record={{ id: "r", workspace_id: "w", collection_id: "c", title: "Row", fields: {}, links: {}, revision: 1, created_at: "" }}
    field={{ id: "contact", name: "Contact", type, config: { options: [] }, position: 0 }}
    open onOpenChange={onOpenChange} onChange={onChange}
  />);
  const input = screen.getByLabelText("Contact");
  expect(input).toHaveAttribute("type", type);
  fireEvent.change(input, { target: { value } });
  fireEvent.submit(input.closest("form")!);
  expect(onChange).toHaveBeenCalledWith(value);
  expect(onOpenChange).toHaveBeenCalledWith(false);
});
