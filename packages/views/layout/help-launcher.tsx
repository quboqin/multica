"use client";

import { CircleHelp, History } from "lucide-react";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@multica/ui/components/ui/dropdown-menu";
import { useWorkspacePaths } from "@multica/core/paths";
import { useT } from "../i18n";
import { AppLink } from "../navigation";

export function HelpLauncher() {
  const { t } = useT("layout");
  const paths = useWorkspacePaths();
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        aria-label={t(($) => $.help.trigger)}
        title={t(($) => $.help.trigger)}
        className="inline-flex size-7 items-center justify-center rounded-full text-muted-foreground transition-colors cursor-pointer hover:bg-accent hover:text-foreground data-popup-open:bg-accent data-popup-open:text-foreground"
      >
        <CircleHelp className="size-4" />
      </DropdownMenuTrigger>
      <DropdownMenuContent
        align="end"
        side="top"
        sideOffset={8}
        className="min-w-40"
      >
        <DropdownMenuItem
          render={
            <AppLink href={paths.changelog()} />
          }
        >
          <History className="h-3.5 w-3.5" />
          {t(($) => $.help.changelog)}
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
