"use client";

import { useEffect, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Download, Loader2, RotateCw } from "lucide-react";
import { api, ApiError, PreviewTooLargeError } from "@multica/core/api";
import { useCurrentWorkspace } from "@multica/core/paths";
import { Button } from "@multica/ui/components/ui/button";
import { useT } from "../i18n";

export function OfficePreview({ attachmentId, filename, onDownload }: {
  attachmentId: string;
  filename: string;
  onDownload: () => void;
}) {
  const { t } = useT("editor");
  const wsId = useCurrentWorkspace()?.id ?? "";
  const query = useQuery({
    queryKey: ["attachment-office-preview", wsId, attachmentId],
    queryFn: ({ signal }) => api.getAttachmentOfficePreview(attachmentId, { workspaceId: wsId, signal }),
    enabled: !!wsId,
    retry: false,
    staleTime: 0,
    gcTime: 0,
    refetchOnWindowFocus: false,
  });
  const [resource, setResource] = useState<{ blob: Blob; url: string } | null>(null);
  useEffect(() => {
    if (!query.data) return;
    const url = URL.createObjectURL(query.data);
    setResource({ blob: query.data, url });
    return () => URL.revokeObjectURL(url);
  }, [query.data]);
  if (query.error || !wsId) {
    const message = query.error instanceof PreviewTooLargeError
      ? t(($) => $.attachment.office_too_large)
      : query.error instanceof ApiError && query.error.status === 503
        ? t(($) => $.attachment.office_unavailable)
        : t(($) => $.attachment.preview_failed);
    return (
      <div className="dark flex h-full flex-col items-center justify-center gap-3 px-8 text-center">
        <p role="alert" className="text-body text-muted-foreground">{message}</p>
        <div className="flex gap-2">
          <Button variant="outline" onClick={() => void query.refetch()}><RotateCw />{t(($) => $.attachment.office_retry)}</Button>
          <Button variant="secondary" onClick={onDownload}><Download />{t(($) => $.image.download)}</Button>
        </div>
      </div>
    );
  }
  if (!resource || resource.blob !== query.data) {
    return <div role="status" className="dark flex h-full items-center justify-center gap-2 text-body text-muted-foreground"><Loader2 className="size-4 animate-spin" />{t(($) => $.attachment.preview_loading)}</div>;
  }
  return <iframe src={resource.url} title={filename} className="h-full w-full rounded-lg bg-background" />;
}
