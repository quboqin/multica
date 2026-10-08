"use client";

import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { ChevronRight, Table2 } from "lucide-react";
import { issueRecordLinksOptions } from "@multica/core/collections";
import { useCurrentWorkspace, useWorkspacePaths } from "@multica/core/paths";
import { cn } from "@multica/ui/lib/utils";
import { useT } from "../../i18n";
import { AppLink } from "../../navigation";

/**
 * Sidebar section listing the table records whose relation fields point at
 * this task — the reverse side of a record's "linked tasks". Each row opens
 * its table with that record's panel showing.
 *
 * Renders nothing while loading and when nothing links here: most tasks are
 * not linked from any table, and an empty header would be noise on all of them.
 */
export function LinkedRecordsSection({ issueId }: { issueId: string }) {
  const { t } = useT("issues");
  // Nullable read, like the other optional sidebar sections: outside a
  // workspace route this degrades to "not rendered".
  const wsId = useCurrentWorkspace()?.id ?? "";
  const paths = useWorkspacePaths();
  const [open, setOpen] = useState(true);
  const { data: links = [] } = useQuery(issueRecordLinksOptions(wsId, issueId));
  if (links.length === 0) return null;

  return (
    <div>
      <button
        type="button"
        className={cn(
          "mb-2 flex w-full items-center gap-1 rounded-md px-2 py-1 text-caption font-medium transition-colors hover:bg-accent/70",
          !open && "text-muted-foreground hover:text-foreground",
        )}
        aria-expanded={open}
        onClick={() => setOpen(!open)}
      >
        {t(($) => $.detail.section_linked_records)}
        <span className="font-normal text-muted-foreground tabular-nums">
          {links.length}
        </span>
        <ChevronRight
          className={cn(
            "!size-3 shrink-0 stroke-[2.5] text-muted-foreground transition-transform",
            open && "rotate-90",
          )}
        />
      </button>
      {open && (
        <ul className="pl-2">
          {links.map((link) => (
            <li key={link.id}>
              <AppLink
                href={`${paths.collectionDetail(link.collection_id)}?record=${link.record_id}`}
                title={`${link.collection_name} › ${link.field_name}`}
                className="group -mx-2 flex min-w-0 items-center gap-1.5 rounded-md px-2 py-1.5 text-caption transition-colors hover:bg-accent/50"
              >
                <Table2 className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
                <span className="max-w-[40%] shrink-0 truncate text-muted-foreground">
                  {link.collection_name}
                </span>
                <span className="truncate group-hover:text-foreground">
                  {link.record_title || t(($) => $.cortex_table.untitled)}
                </span>
              </AppLink>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
