"use client";

import { ArrowUpRight, FolderOpen } from "lucide-react";
import { useWorkspacePaths } from "@multica/core/paths";
import { AppLink } from "../../navigation";

export function CreativeMaterialMigrationNotice() {
  const paths = useWorkspacePaths();

  return (
    <section className="my-5 flex flex-wrap items-center justify-between gap-3 border bg-muted/10 px-4 py-3" data-testid="creative-material-migration-notice">
      <div className="flex min-w-0 items-center gap-2">
        <FolderOpen className="h-4 w-4 shrink-0 text-muted-foreground" />
        <p className="text-sm text-muted-foreground">历史素材流程已迁移到创意工厂素材库。</p>
      </div>
      <AppLink href={paths.creative()} className="inline-flex shrink-0 items-center gap-1 text-sm font-medium underline-offset-4 hover:underline">
        打开素材库
        <ArrowUpRight className="h-3.5 w-3.5" />
      </AppLink>
    </section>
  );
}
