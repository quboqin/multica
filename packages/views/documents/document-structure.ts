import type { Issue } from "@multica/core/types";

export type DocumentLifecycle = "draft" | "reviewing" | "published";

/**
 * Documents store their lifecycle as the status keys draft / reviewing /
 * published (server/internal/handler/document.go maps them to the
 * unstarted / started / done categories). A renamed or custom key falls back
 * to its category so the chip still says something truthful.
 */
export function documentLifecycle(
  issue: Pick<Issue, "status" | "status_category">,
): DocumentLifecycle {
  if (issue.status === "reviewing") return "reviewing";
  if (issue.status === "published") return "published";
  if (issue.status === "draft") return "draft";
  if (issue.status_category === "started") return "reviewing";
  if (issue.status_category === "done") return "published";
  return "draft";
}

export interface OutlineHeading {
  level: 1 | 2 | 3;
  text: string;
  /** Occurrence of this exact text among earlier headings, for DOM lookup. */
  occurrence: number;
}

/** Strip inline Markdown so outline labels match the rendered heading text. */
export function plainInline(text: string): string {
  return text
    .replace(/!?\[([^\]]*)\]\([^)]*\)/g, "$1")
    .replace(/`([^`]*)`/g, "$1")
    .replace(/(\*\*|__|~~|==)(.*?)\1/g, "$2")
    .replace(/(^|[^\w*])[*_]([^*_]+)[*_](?=$|[^\w*])/g, "$1$2")
    .replace(/\\([\\`*_{}[\]()#+\-.!])/g, "$1")
    .trim();
}

/** H1–H3 headings of a Markdown body, skipping fenced code and embeds. */
export function parseOutline(markdown: string): OutlineHeading[] {
  const headings: OutlineHeading[] = [];
  const seen = new Map<string, number>();
  let fence: string | null = null;
  for (const line of markdown.split("\n")) {
    const fenceMatch = line.match(/^\s{0,3}(`{3,}|~{3,}|\${2})/);
    if (fenceMatch) {
      const marker = fenceMatch[1]!;
      if (fence === null) fence = marker;
      else if (marker[0] === fence[0] && marker.length >= fence.length)
        fence = null;
      continue;
    }
    if (fence !== null) continue;
    const match = line.match(/^\s{0,3}(#{1,3})\s+(.+?)\s*#*\s*$/);
    if (!match) continue;
    const text = plainInline(match[2]!);
    if (!text) continue;
    const occurrence = seen.get(text) ?? 0;
    seen.set(text, occurrence + 1);
    headings.push({
      level: match[1]!.length as 1 | 2 | 3,
      text,
      occurrence,
    });
  }
  return headings;
}

export interface DocumentReference {
  kind: "issue" | "view";
  id: string;
  label: string;
  count: number;
}

const MENTION_ISSUE = /\[([^\]]*)\]\(mention:\/\/issue\/([0-9a-z-]+)\)/gi;
const VIEW_EMBED = /^:::multica-view ([0-9a-f-]{36})[ \t]*$/gim;

/** Issue/doc mentions and saved-view embeds found in a body, deduplicated. */
export function parseReferences(markdown: string): DocumentReference[] {
  const byKey = new Map<string, DocumentReference>();
  const add = (kind: DocumentReference["kind"], id: string, label: string) => {
    const key = `${kind}:${id}`;
    const existing = byKey.get(key);
    if (existing) existing.count += 1;
    else byKey.set(key, { kind, id, label, count: 1 });
  };
  for (const match of markdown.matchAll(VIEW_EMBED)) add("view", match[1]!, "");
  for (const match of markdown.matchAll(MENTION_ISSUE))
    add("issue", match[2]!, plainInline(match[1]!).replace(/^@/, ""));
  return [...byKey.values()];
}

/** How many times `markdown` mentions the issue/document `id`. */
export function countMentionsOf(markdown: string, id: string): number {
  let count = 0;
  for (const match of markdown.matchAll(MENTION_ISSUE))
    if (match[2] === id) count += 1;
  return count;
}

/** Other documents whose body mentions `id`, with the mention count. */
export function incomingReferences(
  documents: readonly Issue[],
  id: string,
): { doc: Issue; count: number }[] {
  return documents
    .filter((doc) => doc.id !== id && doc.description)
    .map((doc) => ({ doc, count: countMentionsOf(doc.description ?? "", id) }))
    .filter((entry) => entry.count > 0);
}

/** Scroll the rendered editor to the heading an outline entry points at. */
export function scrollToHeading(
  root: ParentNode,
  heading: Pick<OutlineHeading, "text" | "occurrence">,
): boolean {
  const matches = [...root.querySelectorAll("h1, h2, h3")].filter(
    (element) => element.textContent?.trim() === heading.text,
  );
  const target = matches[heading.occurrence] ?? matches[0];
  if (!target) return false;
  target.scrollIntoView({ behavior: "smooth", block: "start" });
  return true;
}
