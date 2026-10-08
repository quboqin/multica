import { useState } from "react";
import { Alert, FlatList, Pressable, RefreshControl, View } from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import { Stack, router } from "expo-router";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Text } from "@/components/ui/text";
import { IconButton } from "@/components/ui/icon-button";
import { ResourceState } from "@/components/resource/resource-state";
import { api } from "@/data/api";
import { collectionKeys, collectionListOptions } from "@/data/queries/resources";
import { useWorkspaceStore } from "@/data/workspace-store";
import { useT } from "@/lib/i18n";

export default function CollectionsPage() {
  const wsId = useWorkspaceStore((s) => s.currentWorkspaceId);
  const slug = useWorkspaceStore((s) => s.currentWorkspaceSlug);
  const qc = useQueryClient();
  const { t } = useT("resources");
  const query = useQuery(collectionListOptions(wsId));
  const [creating, setCreating] = useState(false);

  const create = () => Alert.prompt(t("new_collection"), t("collection_name"), async (value) => {
    const name = value?.trim();
    if (!name || creating) return;
    setCreating(true);
    try {
      const collection = await api.createCollection(name);
      if (!collection) throw new Error("Invalid collection response");
      await qc.invalidateQueries({ queryKey: collectionKeys.list(wsId) });
      if (slug && collection) router.push(`/${slug}/collection/${collection.id}`);
    } catch {
      Alert.alert(t("save_failed"));
    } finally {
      setCreating(false);
    }
  });

  return <SafeAreaView className="flex-1 bg-background" edges={[]}>
    <Stack.Screen options={{ headerRight: () => <IconButton name="add" disabled={creating} accessibilityLabel={t("new_collection")} onPress={create} /> }} />
    {query.isLoading || query.isError || !query.data?.length ?
      <ResourceState loading={query.isLoading} error={query.isError} empty={t("empty_collections")} retry={() => void query.refetch()} /> :
      <FlatList data={query.data} keyExtractor={(item) => item.id}
        refreshControl={<RefreshControl refreshing={query.isRefetching} onRefresh={() => void query.refetch()} />}
        ItemSeparatorComponent={() => <View className="h-px bg-border ml-4" />}
        renderItem={({ item }) => <Pressable className="px-4 py-4" onPress={() => slug && router.push(`/${slug}/collection/${item.id}`)}>
          <Text className="text-base font-medium">{item.icon ? `${item.icon} ` : ""}{item.name}</Text>
          {!!item.description && <Text className="mt-1 text-sm text-muted-foreground" numberOfLines={2}>{item.description}</Text>}
        </Pressable>} />}
  </SafeAreaView>;
}
