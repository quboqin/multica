"use client";

import { useRef, useState, type FormEvent } from "react";
import { useQuery } from "@tanstack/react-query";
import { AlertCircle, Database, TableProperties } from "lucide-react";
import { useWorkspaceId } from "@multica/core";
import {
  collectionListOptions,
  useCreateCollection,
} from "@multica/core/collections";
import { useFeatureEnabled } from "@multica/core/config";
import { CORTEX_COLLECTIONS_FLAG } from "@multica/core/feature-flags";
import {
  useRequiredWorkspaceSlug,
  useWorkspacePaths,
} from "@multica/core/paths";
import { useCurrentMember } from "@multica/core/permissions";
import { Button } from "@multica/ui/components/ui/button";
import { Input } from "@multica/ui/components/ui/input";
import { AppLink, useNavigation } from "../navigation";
import { useT } from "../i18n";
import {
  CollectionPageHeader,
  CollectionPageState,
} from "../layout/collection-page";

function requestId() {
  return globalThis.crypto.randomUUID();
}

export function CollectionsPage() {
  const { t } = useT("collections");
  const enabled = useFeatureEnabled(CORTEX_COLLECTIONS_FLAG, false);
  const workspaceId = useWorkspaceId();
  const workspaceSlug = useRequiredWorkspaceSlug();
  const paths = useWorkspacePaths();
  const { push } = useNavigation();
  const { role } = useCurrentMember(workspaceId);
  const canCreate = role === "owner" || role === "admin";
  const collections = useQuery({
    ...collectionListOptions(workspaceId, workspaceSlug, { limit: 200 }),
    enabled,
  });
  const createCollection = useCreateCollection();
  const [name, setName] = useState("");
  const [error, setError] = useState<string | null>(null);
  const pendingRequest = useRef<{ intent: string; id: string } | null>(null);

  if (!enabled) {
    return (
      <CollectionPageState
        icon={Database}
        title={t(($) => $.not_available)}
        description={t(($) => $.not_available_description)}
      />
    );
  }

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    const trimmed = name.trim();
    if (!trimmed || createCollection.isPending) return;
    setError(null);
    const intent = JSON.stringify([trimmed, t(($) => $.default_note), t(($) => $.default_quantity), t(($) => $.default_checked)]);
    if (pendingRequest.current?.intent !== intent) {
      pendingRequest.current = { intent, id: requestId() };
    }
    try {
      const result = await createCollection.mutateAsync({
        workspaceContext: { workspaceId, workspaceSlug },
        input: {
          clientRequestId: pendingRequest.current.id,
          name: trimmed,
          fields: [
            { name: t(($) => $.default_note), type: "text" },
            { name: t(($) => $.default_quantity), type: "number" },
            { name: t(($) => $.default_checked), type: "checkbox" },
          ],
        },
      });
      pendingRequest.current = null;
      push(paths.collectionDetail(result.collection.id));
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : String(reason));
      await collections.refetch();
    }
  };

  const items = collections.data?.collections ?? [];
  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <CollectionPageHeader
        icon={TableProperties}
        title={t(($) => $.title)}
        count={collections.data?.total}
        description={t(($) => $.description)}
        actions={
          canCreate ? (
            <form className="flex items-center gap-2" onSubmit={submit}>
              <Input
                aria-label={t(($) => $.name)}
                placeholder={t(($) => $.name)}
                value={name}
                maxLength={160}
                disabled={createCollection.isPending}
                onChange={(event) => setName(event.currentTarget.value)}
              />
              <Button
                type="submit"
                size="sm"
                disabled={!name.trim() || createCollection.isPending}
              >
                {createCollection.isPending
                  ? t(($) => $.creating)
                  : t(($) => $.create)}
              </Button>
            </form>
          ) : null
        }
      />
      {error ? (
        <p role="alert" className="px-6 py-2 text-sm text-destructive">
          {error}
        </p>
      ) : null}
      {collections.isError ? (
        <CollectionPageState
          icon={AlertCircle}
          title={collections.error.message}
          tone="destructive"
          role="alert"
          actions={
            <Button type="button" onClick={() => void collections.refetch()}>
              {t(($) => $.retry)}
            </Button>
          }
        />
      ) : collections.isPending ? (
        <CollectionPageState icon={Database} title={t(($) => $.loading)} />
      ) : items.length === 0 ? (
        <CollectionPageState
          icon={Database}
          title={t(($) => $.empty)}
          description={t(($) => $.empty_description)}
        />
      ) : (
        <ul className="grid gap-3 p-6 sm:grid-cols-2 xl:grid-cols-3">
          {items.map((collection) => (
            <li key={collection.id}>
              <AppLink
                href={paths.collectionDetail(collection.id)}
                className="block rounded-lg border p-4 transition-colors hover:bg-muted/40"
              >
                <span className="font-medium">{collection.name}</span>
              </AppLink>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
