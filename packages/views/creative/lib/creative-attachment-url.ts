import { attachmentIdFromDownloadURL, attachmentSameOriginDownloadPath, type Attachment } from "@multica/core/types";
import { resolvePublicFileUrl } from "@multica/core/workspace/avatar-url";

export function creativeAttachmentBrowserURL(
  attachment: Pick<Attachment, "id" | "url" | "download_url" | "markdown_url"> | undefined,
): string {
  if (!attachment) return "";
  const sameOriginDownload = attachmentSameOriginDownloadPath(attachment.id, [
    attachment.download_url,
    attachment.markdown_url,
  ]);
  if (sameOriginDownload) return sameOriginDownload;
  const signedDownload = attachment.download_url && !attachmentIdFromDownloadURL(attachment.download_url)
    ? attachment.download_url
    : "";
  const raw = signedDownload || attachment.url || attachment.markdown_url || attachment.download_url;
  return resolvePublicFileUrl(raw) ?? "";
}
