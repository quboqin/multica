"use client";
import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "@multica/core/api";
import {
  collectionKeys,
  collectionListOptions,
} from "@multica/core/collections";
import { useWorkspaceId } from "@multica/core/hooks";
import { useWorkspacePaths } from "@multica/core/paths";
import { Button } from "@multica/ui/components/ui/button";
import { Input } from "@multica/ui/components/ui/input";
import { AppLink, useNavigation } from "../navigation";
import { useT } from "../i18n";
export function CollectionsPage() {
  const wsId = useWorkspaceId();
  const paths = useWorkspacePaths();
  const nav = useNavigation();
  const { t } = useT("issues");
  const qc = useQueryClient();
  const { data: collections = [], error } = useQuery(
    collectionListOptions(wsId),
  );
  const [name, setName] = useState("");
  const create = useMutation({
    mutationFn: () => api.createCollection(name),
    onSuccess: (collection) => {
      void qc.invalidateQueries({ queryKey: collectionKeys.all(wsId) });
      nav.push(paths.collectionDetail(collection.id));
    },
  });
  return (
    <main className="flex-1 space-y-4 overflow-auto p-6">
      <h1 className="text-title">{t(($) => $.cortex.collections)}</h1>
      <form
        className="flex max-w-lg gap-2"
        onSubmit={(event) => {
          event.preventDefault();
          create.mutate();
        }}
      >
        <Input
          aria-label={t(($) => $.cortex.new_collection)}
          value={name}
          onChange={(event) => setName(event.target.value)}
        />
        <Button type="submit" disabled={!name.trim() || create.isPending}>
          {t(($) => $.cortex.new_collection)}
        </Button>
      </form>
      {(error || create.error) && (
        <p role="alert">{(error || create.error)?.message}</p>
      )}
      <div className="grid grid-cols-[repeat(auto-fill,minmax(240px,1fr))] gap-3">
        {collections.map((collection) => (
          <AppLink
            href={paths.collectionDetail(collection.id)}
            key={collection.id}
            className="rounded border p-4 text-body"
          >
            {collection.name}
          </AppLink>
        ))}
      </div>
    </main>
  );
}
