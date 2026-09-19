import { Node, mergeAttributes } from "@tiptap/core";
import {
  NodeViewWrapper,
  ReactNodeViewRenderer,
  type NodeViewProps,
} from "@tiptap/react";
import { lazy, Suspense, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { LayoutGrid } from "lucide-react";
import { issueViewDetailOptions } from "@multica/core/issue-views/queries";
import { useWorkspaceId } from "@multica/core/hooks";
import { Button } from "@multica/ui/components/ui/button";
import { InsertViewDialog } from "../../documents/insert-view-dialog";
import { useT } from "../../i18n";

const EmbeddedSavedView = lazy(
  () => import("../../documents/embedded-saved-view"),
);
function SavedViewNode({
  node,
  editor,
  updateAttributes,
  deleteNode,
}: NodeViewProps) {
  const wsId = useWorkspaceId();
  const { t } = useT("issues");
  const viewId = typeof node.attrs.viewId === "string" ? node.attrs.viewId : "";
  // A node the /view command just inserted opens its picker right away;
  // cancelling that first pick removes the empty block again.
  const fresh = node.attrs.autoOpen === true && !viewId;
  const [open, setOpen] = useState(fresh);
  const inserted = useRef(false);
  const { data: view, isError } = useQuery(
    issueViewDetailOptions(wsId, viewId),
  );
  const editable = editor.isEditable;
  return (
    <NodeViewWrapper
      contentEditable={false}
      className="my-4 overflow-hidden rounded-lg border"
      data-type="saved-view"
    >
      <div className="flex items-center gap-2 border-b bg-muted/30 px-3 py-2">
        <LayoutGrid className="size-3.5 shrink-0 text-muted-foreground" />
        <span className="min-w-0 flex-1 truncate text-label font-medium">
          {viewId
            ? view?.name ||
              (isError
                ? t(($) => $.cortex_docs.view_unavailable)
                : t(($) => $.cortex_docs.saved_view))
            : t(($) => $.cortex_docs.choose_view)}
        </span>
        {viewId && view && (
          <span className="inline-flex items-center gap-1 text-caption text-success">
            <span aria-hidden className="size-1.5 rounded-full bg-success" />
            {t(($) => $.cortex_docs.live)}
          </span>
        )}
        {editable && (
          <Button
            variant="ghost"
            size="xs"
            className="text-muted-foreground"
            onClick={() => setOpen(true)}
          >
            {viewId
              ? t(($) => $.cortex_docs.replace_view)
              : t(($) => $.cortex_docs.choose_view)}
          </Button>
        )}
      </div>
      {viewId && (
        <Suspense fallback={<p className="p-3">{t(($) => $.cortex_docs.loading)}</p>}>
          <EmbeddedSavedView key={`${wsId}:${viewId}`} viewId={viewId} />
        </Suspense>
      )}
      {editable && (
        <InsertViewDialog
          open={open}
          initialViewId={viewId || undefined}
          onOpenChange={(next) => {
            setOpen(next);
            if (!next && fresh && !inserted.current) deleteNode();
          }}
          onInsert={(id) => {
            inserted.current = true;
            updateAttributes({ viewId: id, autoOpen: false });
          }}
        />
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
    return {
      viewId: { default: "" },
      // Set only by the /view command; never parsed or serialized.
      autoOpen: { default: false, rendered: false, parseHTML: () => false },
    };
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
