"use client";

import { MoreHorizontal } from "lucide-react";
import type { Issue } from "@multica/core/types";
import { useWorkspacePaths } from "@multica/core/paths";
import { Button } from "@multica/ui/components/ui/button";
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@multica/ui/components/ui/dropdown-menu";
import { AppLink } from "../navigation";
import { useT } from "../i18n";

export function DocumentBreadcrumb({ trail }: { trail: readonly Issue[] }) {
  const { t } = useT("issues");
  const paths = useWorkspacePaths();
  const parents = trail.slice(0, -1);
  const current = trail.at(-1);
  const title = (doc: Issue) => doc.title || t(($) => $.cortex_docs.untitled);
  return (
    <nav aria-label={t(($) => $.cortex_docs.breadcrumb)} className="flex min-w-0 flex-1 items-center gap-1.5 overflow-hidden text-label text-muted-foreground">
      <AppLink href={paths.documents()} className="min-w-0 max-w-24 truncate hover:text-foreground" title={t(($) => $.cortex_docs.documents)}>
        {t(($) => $.cortex_docs.documents)}
      </AppLink>
      {parents.length > 0 && (
        <>
          <span aria-hidden className="shrink-0">/</span>
          <DropdownMenu>
            <DropdownMenuTrigger render={<Button variant="ghost" size="icon-xs" className="shrink-0" aria-label={t(($) => $.cortex_docs.parent_documents)} />}>
              <MoreHorizontal />
            </DropdownMenuTrigger>
            <DropdownMenuContent align="start" className="max-w-[min(24rem,90vw)]">
              {parents.map((doc) => (
                <DropdownMenuItem key={doc.id} render={<AppLink href={paths.documentDetail(doc.id)} />}>
                  <span className="truncate" title={title(doc)}>{title(doc)}</span>
                </DropdownMenuItem>
              ))}
            </DropdownMenuContent>
          </DropdownMenu>
        </>
      )}
      {current && (
        <>
          <span aria-hidden className="shrink-0">/</span>
          <AppLink href={paths.documentDetail(current.id)} aria-current="page" title={title(current)} className="min-w-0 truncate font-medium text-foreground">
            {title(current)}
          </AppLink>
        </>
      )}
    </nav>
  );
}
