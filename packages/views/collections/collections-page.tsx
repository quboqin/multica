"use client";

import { Plus, Table2 } from "lucide-react";
import { useCurrentWorkspace } from "@multica/core/paths";
import { Button } from "@multica/ui/components/ui/button";
import { CollectionNavigator, NewTablePopover } from "../cortex";
import { useT } from "../i18n";
import { CollectionImportDialog } from "./collection-import-dialog";

export function CollectionsPage() {
  return (
    <div className="flex min-h-0 min-w-0 flex-1">
      <CollectionNavigator />
      <CollectionsEmptyState />
    </div>
  );
}

function CollectionsEmptyState() {
  const { t } = useT("issues");
  const ws = useCurrentWorkspace();
  return (
    <main className="hidden min-w-0 flex-1 flex-col items-center justify-center gap-3 p-6 text-center md:flex">
      <Table2 className="size-8 text-muted-foreground" aria-hidden />
      <h1 className="text-title-sm font-medium">
        {t(($) => $.cortex_docs.tables_empty_title)}
      </h1>
      <NewTablePopover
        align="center"
        trigger={
          <Button disabled={!ws}>
            <Plus />
            {t(($) => $.cortex_docs.new_table)}
          </Button>
        }
      />
      <CollectionImportDialog />
    </main>
  );
}
