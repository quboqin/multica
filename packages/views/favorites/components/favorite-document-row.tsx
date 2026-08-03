"use client";

import { MessageSquare } from "lucide-react";
import type { Attachment as AttachmentRecord } from "@multica/core/types";
import { useWorkspacePaths } from "@multica/core/paths";
import { Skeleton } from "@multica/ui/components/ui/skeleton";
import { Attachment } from "../../editor";
import { useAttachmentHtmlText } from "../../editor/hooks/use-attachment-html-text";
import { getPreviewKind } from "../../editor/utils/preview";
import { useT } from "../../i18n";
import { AppLink } from "../../navigation";

export const FAVORITE_SUMMARY_CHARACTER_LIMIT = 160;

export function favoriteDocumentSummary(
  content: string,
  limit = FAVORITE_SUMMARY_CHARACTER_LIMIT,
): string {
  const normalized = content.replace(/\s+/g, " ").trim();
  const characters = Array.from(normalized);
  if (characters.length <= limit) return normalized;
  return `${characters.slice(0, limit).join("")}...`;
}

export function FavoriteDocumentRow({
  attachment,
}: {
  attachment: AttachmentRecord;
}) {
  const { t } = useT("layout");
  const paths = useWorkspacePaths();
  const isMarkdown =
    getPreviewKind(attachment.content_type, attachment.filename) === "markdown";
  const contentQuery = useAttachmentHtmlText(isMarkdown ? attachment.id : null);
  const summary = isMarkdown && contentQuery.data?.text
    ? favoriteDocumentSummary(contentQuery.data.text)
    : "";
  const sourceHref = attachment.issue_id
    ? `${paths.issueDetail(attachment.issue_id)}${
        attachment.comment_id
          ? `?${new URLSearchParams({ comment: attachment.comment_id }).toString()}`
          : ""
      }`
    : null;

  const sourceAction = sourceHref ? (
    <AppLink
      href={sourceHref}
      className="shrink-0 rounded-md p-1 text-muted-foreground transition-colors hover:bg-secondary hover:text-foreground"
      title={t(($) => $.favorites.page.open_source_conversation)}
      aria-label={t(($) => $.favorites.page.open_source_conversation)}
    >
      <MessageSquare className="size-3.5" />
    </AppLink>
  ) : null;

  const document = (
    <>
      <Attachment
        attachment={{ kind: "record", attachment }}
        trailingAction={sourceAction}
        className={
          isMarkdown
            ? "my-0 [&>div]:rounded-none [&>div]:border-0 [&>div]:bg-muted/40"
            : undefined
        }
      />
      {contentQuery.isLoading && isMarkdown ? (
        <div className="space-y-1 border-t bg-background px-3 py-2" aria-hidden="true">
          <Skeleton className="h-3 w-full" />
          <Skeleton className="h-3 w-2/3" />
        </div>
      ) : summary ? (
        <p className="line-clamp-3 border-t bg-background px-3 py-2 text-xs leading-5 text-muted-foreground">
          {summary}
        </p>
      ) : null}
    </>
  );

  return isMarkdown ? (
    <div className="overflow-hidden rounded-md border bg-muted/20">
      {document}
    </div>
  ) : (
    <div>{document}</div>
  );
}
