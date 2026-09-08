"use client";

import { History, Sparkles } from "lucide-react";
import { PageHeader } from "../../layout/page-header";
import { useT } from "../../i18n";
import { PLATFORM_RELEASE_IDS, PLATFORM_VERSION, platformReleaseContent } from "../releases";

export function ChangelogPage() {
  const { i18n, t } = useT("changelog");
  const releases = PLATFORM_RELEASE_IDS.map((id) => ({
    id,
    ...platformReleaseContent(i18n.language, id),
  }));

  return (
    <div className="flex min-h-0 min-w-0 flex-1 flex-col">
      <PageHeader>
        <div className="flex items-center gap-2">
          <History className="h-4 w-4 text-muted-foreground" />
          <h1 className="text-sm font-medium">{t(($) => $.title)}</h1>
        </div>
      </PageHeader>

      <main className="min-h-0 flex-1 overflow-y-auto">
        <div className="mx-auto w-full max-w-3xl space-y-8 p-5 md:p-8">
          <header className="space-y-2">
            <p className="text-sm text-muted-foreground">{t(($) => $.subtitle)}</p>
            <p className="text-xs text-muted-foreground">
              {t(($) => $.current_version, { version: PLATFORM_VERSION })}
            </p>
          </header>

          <div className="divide-y border-y">
            {releases.map((release) => (
              <article key={release.id} className="space-y-5 py-7 first:pt-6 last:pb-6">
                <div className="flex flex-wrap items-start justify-between gap-4">
                  <div className="flex items-start gap-3">
                    <div className="mt-0.5 rounded-md border bg-muted/50 p-2 text-muted-foreground">
                      <Sparkles className="h-4 w-4" />
                    </div>
                    <div className="space-y-1">
                      <h2 className="text-base font-semibold">
                        {release.title}
                      </h2>
                      <p className="text-sm text-muted-foreground">
                        {release.summary}
                      </p>
                    </div>
                  </div>
                  <div className="shrink-0 text-right text-xs text-muted-foreground">
                    <p className="font-medium text-foreground">
                      {release.version}
                    </p>
                    <p>{release.date}</p>
                  </div>
                </div>

                <ul className="space-y-3 pl-11 text-sm text-muted-foreground">
                  {release.changes.map((change) => (
                    <li key={change} className="flex gap-3">
                      <span aria-hidden="true" className="text-primary">-</span>
                      <span>{change}</span>
                    </li>
                  ))}
                </ul>
              </article>
            ))}
          </div>
        </div>
      </main>
    </div>
  );
}
