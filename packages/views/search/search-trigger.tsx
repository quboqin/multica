"use client";

import { Search } from "lucide-react";
import { SidebarMenuButton } from "@multica/ui/components/ui/sidebar";
import {
  useShortcut,
} from "@multica/core/shortcuts";
import { useSearchStore } from "./search-store";
import { useT } from "../i18n";
import { ShortcutKeycaps } from "../common/shortcut-keycaps";

export function SearchTrigger() {
  const { t } = useT("search");
  const shortcut = useShortcut("openSearch");
  return (
    <SidebarMenuButton
      className="text-muted-foreground"
      aria-label={t(($) => $.trigger.label)}
      tooltip={t(($) => $.trigger.label)}
      onClick={() => useSearchStore.getState().setOpen(true)}
    >
      <Search />
      <span className="group-data-[collapsible=icon]:sr-only">{t(($) => $.trigger.label)}</span>
      {shortcut ? (
        <ShortcutKeycaps shortcut={shortcut} decorative className="pointer-events-none ml-auto group-data-[collapsible=icon]:hidden" />
      ) : null}
    </SidebarMenuButton>
  );
}
