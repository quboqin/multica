import { useState } from "react";
import { Alert, FlatList, Pressable, RefreshControl, View } from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import { Stack, router } from "expo-router";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Text } from "@/components/ui/text";
import { IconButton } from "@/components/ui/icon-button";
import { ResourceState } from "@/components/resource/resource-state";
import { api } from "@/data/api";
import { documentKeys, documentListOptions } from "@/data/queries/resources";
import { useWorkspaceStore } from "@/data/workspace-store";
import { useT } from "@/lib/i18n";

export default function DocumentsPage() {
  const wsId = useWorkspaceStore((s) => s.currentWorkspaceId);
  const slug = useWorkspaceStore((s) => s.currentWorkspaceSlug);
  const qc = useQueryClient();
  const { t } = useT("resources");
  const query = useQuery(documentListOptions(wsId));
  const [creating, setCreating] = useState(false);

  const create = () => Alert.prompt(t("new_document"), t("document_title"), async (value) => {
    const title = value?.trim();
    if (!title || creating) return;
    setCreating(true);
    try {
      const doc = await api.createDocument(title);
      if (!doc || doc.kind !== "doc" || !doc.id) throw new Error("Invalid document response");
      await qc.invalidateQueries({ queryKey: documentKeys.list(wsId) });
      if (slug) router.push(`/${slug}/document/${doc.id}`);
    } catch {
      Alert.alert(t("save_failed"));
    } finally {
      setCreating(false);
    }
  });

  return <SafeAreaView className="flex-1 bg-background" edges={[]}>
    <Stack.Screen options={{ headerRight: () => <IconButton name="add" disabled={creating} accessibilityLabel={t("new_document")} onPress={create} /> }} />
    {query.isLoading || query.isError || !query.data?.length ?
      <ResourceState loading={query.isLoading} error={query.isError} empty={t("empty_documents")} retry={() => void query.refetch()} /> :
      <FlatList data={query.data} keyExtractor={(doc) => doc.id}
        refreshControl={<RefreshControl refreshing={query.isRefetching} onRefresh={() => void query.refetch()} />}
        ItemSeparatorComponent={() => <View className="h-px bg-border ml-4" />}
        renderItem={({ item }) => <Pressable className="px-4 py-4" onPress={() => slug && router.push(`/${slug}/document/${item.id}`)}>
          <Text className="text-base font-medium">{item.title || t("untitled")}</Text>
        </Pressable>} />}
  </SafeAreaView>;
}
