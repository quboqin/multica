"use client";

/**
 * AttachmentCard — shared file-card row UI (icon + filename + Eye + Download).
 *
 * Subcomponent of the unified `<Attachment>` dispatcher (see attachment.tsx).
 * Rendered for every attachment kind that does not have a richer inline
 * renderer (image / html). Kind-aware routing lives in `<Attachment>` — keep
 * that decision out of this file so this stays a single-purpose row UI.
 */

import type { ReactNode } from "react";
import {
  Download,
  Eye,
  FileText,
  FolderInput,
  Loader2,
  Star,
  Trash2,
} from "lucide-react";
import { cn } from "@multica/ui/lib/utils";
import { useT } from "../i18n";
import { getPreviewKind } from "./utils/preview";

interface AttachmentCardChromeProps {
  filename: string;
  uploading?: boolean;
  canPreview: boolean;
  canDownload: boolean;
  canDelete?: boolean;
  isFavorite?: boolean;
  favoritePending?: boolean;
  onPreview: () => void;
  onDownload: () => void;
  onDelete?: () => void;
  onToggleFavorite?: () => void;
  onChangeFavoriteCategory?: () => void;
  trailingAction?: ReactNode;
}

function AttachmentCardChrome({
  filename,
  uploading,
  canPreview,
  canDownload,
  canDelete,
  isFavorite,
  favoritePending,
  onPreview,
  onDownload,
  onDelete,
  onToggleFavorite,
  onChangeFavoriteCategory,
  trailingAction,
}: AttachmentCardChromeProps) {
  const { t } = useT("editor");
  return (
    <div
      className="flex items-center gap-2 rounded-md border border-border bg-muted/50 px-2.5 py-1 transition-colors hover:bg-muted"
      onMouseDown={(e) => e.stopPropagation()}
    >
      {uploading ? (
        <Loader2 className="size-4 shrink-0 animate-spin text-muted-foreground" />
      ) : (
        <FileText className="size-4 shrink-0 text-muted-foreground" />
      )}
      <div className="min-w-0 flex-1">
        <p className="truncate text-sm">
          {uploading
            ? t(($) => $.file_card.uploading, { filename })
            : filename}
        </p>
      </div>
      {!uploading && onToggleFavorite && (
        <button
          type="button"
          className="shrink-0 cursor-pointer rounded-md p-1 text-muted-foreground transition-colors hover:bg-secondary hover:text-foreground disabled:pointer-events-none disabled:opacity-50"
          title={t(($) => isFavorite ? $.attachment.unfavorite : $.attachment.favorite)}
          aria-label={t(($) => isFavorite ? $.attachment.unfavorite : $.attachment.favorite)}
          aria-pressed={isFavorite === true}
          disabled={favoritePending}
          onMouseDown={(e) => {
            e.preventDefault();
            e.stopPropagation();
            onToggleFavorite();
          }}
        >
          <Star className="size-3.5" fill={isFavorite ? "currentColor" : "none"} />
        </button>
      )}
      {!uploading && isFavorite && onChangeFavoriteCategory && (
        <button
          type="button"
          className="shrink-0 cursor-pointer rounded-md p-1 text-muted-foreground transition-colors hover:bg-secondary hover:text-foreground"
          title={t(($) => $.attachment.change_favorite_category)}
          aria-label={t(($) => $.attachment.change_favorite_category)}
          onClick={(e) => {
            e.stopPropagation();
            onChangeFavoriteCategory();
          }}
        >
          <FolderInput className="size-3.5" />
        </button>
      )}
      {!uploading && trailingAction}
      {!uploading && canPreview && (
        <button
          type="button"
          className="shrink-0 rounded-md p-1 text-muted-foreground transition-colors hover:bg-secondary hover:text-foreground"
          title={t(($) => $.attachment.preview)}
          aria-label={t(($) => $.attachment.preview)}
          onMouseDown={(e) => {
            e.preventDefault();
            e.stopPropagation();
            onPreview();
          }}
        >
          <Eye className="size-3.5" />
        </button>
      )}
      {!uploading && canDownload && (
        <button
          type="button"
          className="shrink-0 rounded-md p-1 text-muted-foreground transition-colors hover:bg-secondary hover:text-foreground"
          title={t(($) => $.image.download)}
          aria-label={t(($) => $.image.download)}
          onMouseDown={(e) => {
            e.preventDefault();
            e.stopPropagation();
            onDownload();
          }}
        >
          <Download className="size-3.5" />
        </button>
      )}
      {!uploading && canDelete && onDelete && (
        <button
          type="button"
          className="shrink-0 rounded-md p-1 text-muted-foreground transition-colors hover:bg-destructive/10 hover:text-destructive"
          title={t(($) => $.attachment.remove)}
          aria-label={t(($) => $.attachment.remove)}
          onMouseDown={(e) => {
            e.preventDefault();
            e.stopPropagation();
            onDelete();
          }}
        >
          <Trash2 className="size-3.5" />
        </button>
      )}
    </div>
  );
}

export interface AttachmentCardProps {
  /** Filename used for icon label and previewable-kind detection. */
  filename: string;
  /** Content type used in addition to filename for previewable-kind detection. */
  contentType?: string;
  /**
   * Attachment id — required when the preview proxy is ID-keyed (text kinds
   * like markdown / html / text). Media kinds (pdf/video/audio) preview from
   * the URL alone.
   */
  attachmentId?: string;
  /** Download URL — used as a non-null sentinel for the download button. */
  href?: string;
  /** True while a synchronous upload is in flight (file-card NodeView only). */
  uploading?: boolean;
  /** Pressed when the Eye button is clicked. */
  onPreview: () => void;
  /** Pressed when the Download button is clicked. */
  onDownload: () => void;
  /** Optional remove button, used by editable comment/file-card surfaces. */
  onDelete?: () => void;
  /** Current server-backed favorite state for this attachment. */
  isFavorite?: boolean;
  /** Disables the favorite control while its mutation is in flight. */
  favoritePending?: boolean;
  /** Optional favorite toggle; omitted for URL-only and in-flight files. */
  onToggleFavorite?: () => void;
  /** Opens the category picker for an existing favorite. */
  onChangeFavoriteCategory?: () => void;
  /** Optional action rendered with the file-card toolbar controls. */
  trailingAction?: ReactNode;
  className?: string;
}

export function AttachmentCard({
  filename,
  contentType = "",
  attachmentId,
  href,
  uploading,
  onPreview,
  onDownload,
  onDelete,
  isFavorite,
  favoritePending,
  onToggleFavorite,
  onChangeFavoriteCategory,
  trailingAction,
  className,
}: AttachmentCardProps) {
  const kind = filename ? getPreviewKind(contentType, filename) : null;
  // Media kinds (pdf/video/audio) are previewable from a URL alone — the
  // modal renders them as <video>/<audio>/<iframe src=url>. Text kinds
  // (markdown/html/text) need the ID-keyed `/api/attachments/{id}/content`
  // proxy, so they only preview when we have an attachmentId — otherwise
  // the Eye button would call tryOpen, get rejected, and do nothing.
  const isUrlPreviewableKind =
    kind === "pdf" || kind === "video" || kind === "audio";
  const canPreview =
    !!href && kind !== null && (!!attachmentId || isUrlPreviewableKind);

  return (
    <div className={cn("my-1", className)}>
      <AttachmentCardChrome
        filename={filename}
        uploading={uploading}
        canPreview={canPreview}
        canDownload={!!href}
        canDelete={!!onDelete}
        isFavorite={isFavorite}
        favoritePending={favoritePending}
        onPreview={onPreview}
        onDownload={onDownload}
        onDelete={onDelete}
        onToggleFavorite={onToggleFavorite}
        onChangeFavoriteCategory={onChangeFavoriteCategory}
        trailingAction={trailingAction}
      />
    </div>
  );
}
