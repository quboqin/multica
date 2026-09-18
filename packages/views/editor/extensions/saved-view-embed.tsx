import { Node, mergeAttributes } from "@tiptap/core";
import {
  NodeViewWrapper,
  ReactNodeViewRenderer,
  type NodeViewProps,
} from "@tiptap/react";
import { lazy, Suspense, useState } from "react";
import { useQueries, useQuery } from "@tanstack/react-query";
import { collectionListOptions } from "@multica/core/collections";
import { issueViewListOptions } from "@multica/core/issue-views/queries";
import { useWorkspaceId } from "@multica/core/hooks";
import { useT } from "../../i18n";

const EmbeddedSavedView = lazy(
  () => import("../../documents/embedded-saved-view"),
);
function SavedViewNode({ node, updateAttributes }: NodeViewProps) {
  const wsId = useWorkspaceId();
  const { t } = useT("issues");
  const [input, setInput] = useState(String(node.attrs.viewId ?? ""));
  const { data: taskViews = [] } = useQuery(
    issueViewListOptions(wsId, { scope_type: "workspace" }),
  );
  const { data: collections = [] } = useQuery(collectionListOptions(wsId));
  const collectionViews = useQueries({
    queries: collections.map((collection) =>
      issueViewListOptions(wsId, {
        scope_type: "workspace",
        collection_id: collection.id,
      }),
    ),
  });
  const views = [
    ...taskViews,
    ...collectionViews.flatMap((query) => query.data ?? []),
  ];
  const viewId = typeof node.attrs.viewId === "string" ? node.attrs.viewId : "";
  return (
    <NodeViewWrapper
      contentEditable={false}
      className="my-4 overflow-hidden rounded border"
      data-type="saved-view"
    >
      <div className="flex flex-wrap items-center gap-2 border-b bg-muted/30 p-2">
        <label className="text-caption">
          {t(($) => $.cortex.embed_view)}{" "}
          <select
            className="rounded border bg-background p-1"
            value={viewId}
            onChange={(event) => {
              setInput(event.target.value);
              updateAttributes({ viewId: event.target.value });
            }}
          >
            <option value="">—</option>
            {views.map((view) => (
              <option key={view.id} value={view.id}>
                {view.name}
              </option>
            ))}
          </select>
        </label>
        <details className="text-caption">
          <summary>{t(($) => $.cortex.view_id)}</summary>
          <input
            aria-label={t(($) => $.cortex.view_id)}
            className="min-w-64 rounded border bg-background p-1 text-caption"
            value={input}
            placeholder={t(($) => $.cortex.view_id)}
            onChange={(event) => setInput(event.target.value)}
            onBlur={() => {
              if (/^[0-9a-f-]{36}$/i.test(input))
                updateAttributes({ viewId: input });
            }}
          />
        </details>
      </div>
      {viewId && (
        <Suspense fallback={<p>{t(($) => $.cortex.loading)}</p>}>
          <EmbeddedSavedView key={`${wsId}:${viewId}`} viewId={viewId} />
        </Suspense>
      )}
    </NodeViewWrapper>
  );
}

/** Store only a view reference. Private data is fetched through its normal
 * authenticated endpoint every time the node is opened. */
export const SavedViewEmbed = Node.create({
  name: "savedViewEmbed",
  group: "block",
  atom: true,
  isolating: true,
  addAttributes() {
    return { viewId: { default: "" } };
  },
  parseHTML() {
    return [
      {
        tag: 'div[data-type="saved-view"]',
        getAttrs: (element) => ({
          viewId: element.getAttribute("data-view-id") ?? "",
        }),
      },
    ];
  },
  renderHTML({ node, HTMLAttributes }) {
    return [
      "div",
      mergeAttributes(HTMLAttributes, {
        "data-type": "saved-view",
        "data-view-id": node.attrs.viewId,
      }),
    ];
  },
  markdownTokenizer: {
    name: "savedViewEmbed",
    level: "block" as const,
    start: (source: string) => source.search(/^:::multica-view /m),
    tokenize(source: string) {
      const match = source.match(
        /^:::multica-view ([0-9a-f-]{36}|pending)[ \t]*(?:\n|$)/i,
      );
      if (!match) return undefined;
      return {
        type: "savedViewEmbed",
        raw: match[0],
        attributes: { viewId: match[1] === "pending" ? "" : match[1] },
      };
    },
  },
  parseMarkdown: (token, helpers) =>
    helpers.createNode("savedViewEmbed", token.attributes),
  renderMarkdown: (node) =>
    `:::multica-view ${node.attrs?.viewId || "pending"}\n`,
  addNodeView() {
    return ReactNodeViewRenderer(SavedViewNode);
  },
});
