import type { UpdateIssueRequest } from "@multica/core/types";

export interface DocumentBase {
  title: string;
  body: string;
  revision: number;
}

export function buildDocumentUpdate(base: DocumentBase, title: string, body: string): UpdateIssueRequest {
  return {
    title: title.trim(),
    title_base: base.title,
    description: body,
    description_base: base.body,
    expected_document_revision: base.revision,
  };
}

export function editableFieldValue(type: string, input: string): unknown {
  if (type === "number") {
    if (!input.trim()) return null;
    const value = Number(input);
    return Number.isFinite(value) ? value : undefined;
  }
  if (type === "text" || type === "date") return input || null;
  return undefined;
}
