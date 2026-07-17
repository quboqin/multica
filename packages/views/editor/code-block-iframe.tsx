"use client";

/**
 * Shared HTML preview iframe.
 *
 * Used by:
 *   - InlineHtmlIframe inside AttachmentCard (HTML attachments inline preview)
 *   - CodeBlockView for fenced ```html blocks (editable Tiptap NodeView)
 *   - HtmlBlockPreview for fenced ```html blocks (ReadonlyContent)
 *   - AttachmentPreviewModal's full-screen HTML kind
 *
 * Sandbox semantics:
 *   sandbox="allow-scripts allow-popups allow-popups-to-escape-sandbox"
 *   (NOT "allow-same-origin")
 *   → iframe runs in an opaque origin: scripts execute (chart JS works),
 *     but cookie / localStorage / parent access / top-nav / forms remain
 *     blocked. Popups are allowed so generated reports can open external
 *     material links in a new tab.
 *
 * The server-side `text/plain` + `nosniff` defense at
 * /api/attachments/{id}/content remains untouched — we only feed iframe.srcDoc
 * the text body we fetched, never point iframe.src at the proxy URL.
 */

import { cn } from "@multica/ui/lib/utils";

interface CodeBlockIframeProps {
  /** Document source for srcDoc. Empty string renders a blank frame. */
  html: string;
  /** Iframe title for accessibility. */
  title: string;
  className?: string;
  /** Tailwind height token; defaults to h-[480px]. */
  heightClassName?: string;
}

export function CodeBlockIframe({
  html,
  title,
  className,
  heightClassName = "h-[480px]",
}: CodeBlockIframeProps) {
  return (
    <iframe
      // srcDoc keeps the body in the parent's process but isolated to an
      // opaque origin via sandbox. Critical that we never combine
      // `allow-scripts` with `allow-same-origin` — that pairing defeats the
      // sandbox per the HTML spec (notes on the sandbox attribute).
      srcDoc={html}
      sandbox="allow-scripts allow-popups allow-popups-to-escape-sandbox"
      title={title}
      className={cn(
        "w-full rounded-md border border-border bg-background",
        heightClassName,
        className,
      )}
    />
  );
}
