"use client";

import { useEffect, useState } from "react";
import { History, Sparkles } from "lucide-react";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogMedia,
  AlertDialogTitle,
} from "@multica/ui/components/ui/alert-dialog";
import { useWorkspacePaths } from "@multica/core/paths";
import { useNavigation } from "../navigation";
import { useT } from "../i18n";
import { PLATFORM_RELEASE_ID, PLATFORM_VERSION, platformReleaseContent } from "../changelog/releases";

const LAST_SEEN_VERSION_KEY = "multica:last-seen-platform-version";

export function ReleaseAnnouncement() {
  const { t: tLayout } = useT("layout");
  const { i18n: changelogI18n } = useT("changelog");
  const navigation = useNavigation();
  const paths = useWorkspacePaths();
  const version = PLATFORM_VERSION;
  const release = platformReleaseContent(changelogI18n.language, PLATFORM_RELEASE_ID);
  const [open, setOpen] = useState(false);

  useEffect(() => {
    if (!version || typeof window === "undefined") return;
    try {
      if (window.localStorage.getItem(LAST_SEEN_VERSION_KEY) === version) return;
      window.localStorage.setItem(LAST_SEEN_VERSION_KEY, version);
      setOpen(true);
    } catch {
      // Storage can be unavailable in private browsing; the dialog remains
      // usable for the current session without blocking the dashboard.
      setOpen(true);
    }
  }, [version]);

  if (!version) return null;

  return (
    <AlertDialog open={open} onOpenChange={setOpen}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogMedia>
            <Sparkles className="h-5 w-5 text-primary" />
          </AlertDialogMedia>
          <AlertDialogTitle>{tLayout(($) => $.release_announcement.title, { version })}</AlertDialogTitle>
          <AlertDialogDescription>{release.summary}</AlertDialogDescription>
        </AlertDialogHeader>
        <ul className="space-y-2 text-sm text-muted-foreground">
          {release.changes.map((change) => (
            <li key={change} className="flex gap-2">
              <span aria-hidden="true">-</span>
              <span>{change}</span>
            </li>
          ))}
        </ul>
        <AlertDialogFooter>
          <AlertDialogCancel>{tLayout(($) => $.release_announcement.later)}</AlertDialogCancel>
          <AlertDialogAction
            onClick={() => {
              setOpen(false);
              navigation.push(paths.changelog());
            }}
          >
            <History className="h-3.5 w-3.5" />
            {tLayout(($) => $.release_announcement.view_changelog)}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
