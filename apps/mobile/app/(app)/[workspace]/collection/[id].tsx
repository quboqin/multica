import { useState } from "react";
import { Alert, Pressable, RefreshControl, ScrollView, View } from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import { Stack, useLocalSearchParams } from "expo-router";
import { useInfiniteQuery, useQuery, useQueryClient } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { Text } from "@/components/ui/text";
import { ResourceState } from "@/components/resource/resource-state";
import { api, ApiError } from "@/data/api";
import { collectionDetailOptions, collectionKeys, collectionRecordsOptions } from "@/data/queries/resources";
import type { CollectionRecord } from "@/data/resource-schemas";
import { editableFieldValue } from "@/data/resource-edits";
import { useWorkspaceStore } from "@/data/workspace-store";
import { useT } from "@/lib/i18n";

function fieldText(value: unknown): string {
  if (value == null || value === "") return "";
  if (typeof value === "string" || typeof value === "number" || typeof value === "boolean") return String(value);
  if (Array.isArray(value)) return value.map(fieldText).filter(Boolean).join(", ");
  return "";
}

export default function CollectionDetailPage() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const wsId = useWorkspaceStore((s) => s.currentWorkspaceId);
  const qc = useQueryClient();
  const { t } = useT("resources");
  const detail = useQuery(collectionDetailOptions(wsId, id));
  const records = useInfiniteQuery(collectionRecordsOptions(wsId, id));
  const [pending, setPending] = useState(false);
  const collection = detail.data?.collection;
  const canEdit = detail.data?.access?.can_edit === true;
  const canManage = detail.data?.access?.can_manage === true;
  const items = records.data?.pages.flatMap((page) => page.records) ?? [];
  const refresh = () => { void detail.refetch(); void records.refetch(); };

  const createRecord = () => Alert.prompt(t("new_record"), t("record_title"), async (value) => {
    if (!canEdit || pending) return;
    setPending(true);
    try {
      const saved = await api.createCollectionRecord(id, value?.trim() ?? "");
      if (!saved) throw new Error("Invalid record response");
      await qc.invalidateQueries({ queryKey: collectionKeys.records(wsId, id) });
      await qc.invalidateQueries({ queryKey: collectionKeys.list(wsId) });
    } catch { Alert.alert(t("save_failed")); }
    finally { setPending(false); }
  });

  const editName = () => Alert.prompt(t("collection_name"), undefined, async (value) => {
    const name = value?.trim();
    if (!name || !canManage || pending) return;
    setPending(true);
    try {
      const saved = await api.updateCollection(id, { name });
      if (!saved) throw new Error("Invalid collection response");
      await qc.invalidateQueries({ queryKey: collectionKeys.all(wsId) });
    } catch { Alert.alert(t("save_failed")); }
    finally { setPending(false); }
  }, "plain-text", collection?.name);

  const editDescription = () => Alert.prompt(t("description"), undefined, async (value) => {
    if (value == null || !canManage || pending) return;
    setPending(true);
    try {
      const saved = await api.updateCollection(id, { description: value });
      if (!saved) throw new Error("Invalid collection response");
      await qc.invalidateQueries({ queryKey: collectionKeys.all(wsId) });
    } catch { Alert.alert(t("save_failed")); }
    finally { setPending(false); }
  }, "plain-text", collection?.description);

  const renameRecord = (record: CollectionRecord) => Alert.prompt(t("record_title"), undefined, async (value) => {
    const title = value?.trim();
    if (title == null || !canEdit || pending) return;
    setPending(true);
    try {
      const saved = await api.updateCollectionRecord(id, record.id, title, record.title);
      if (!saved) throw new Error("Invalid record response");
      await qc.invalidateQueries({ queryKey: collectionKeys.records(wsId, id) });
    } catch (error) {
      Alert.alert(error instanceof ApiError && error.status === 409 ? t("conflict") : t("save_failed"));
      if (error instanceof ApiError && error.status === 409) void records.refetch();
    } finally { setPending(false); }
  }, "plain-text", record.title);

  const editField = (record: CollectionRecord, field: { id: string; name: string; type: string }) => {
    if (!canEdit || pending) return;
    const current = record.fields[field.id] ?? null;
    const write = async (value: unknown) => {
      setPending(true);
      try {
        const saved = await api.setCollectionRecordField(id, record.id, field.id, value, current);
        if (!saved) throw new Error("Invalid record response");
        await qc.invalidateQueries({ queryKey: collectionKeys.records(wsId, id) });
      } catch (error) {
        Alert.alert(error instanceof ApiError && error.status === 409 ? t("conflict") : t("save_failed"));
        if (error instanceof ApiError && error.status === 409) void records.refetch();
      } finally { setPending(false); }
    };
    if (field.type === "checkbox") {
      Alert.alert(field.name, undefined, [
        { text: t("cancel"), style: "cancel" },
        { text: current === true ? t("false") : t("true"), onPress: () => void write(current !== true) },
      ]);
      return;
    }
    if (!["text", "number", "date"].includes(field.type)) return;
    Alert.prompt(field.name, undefined, (input) => {
      if (input == null) return;
      const value = editableFieldValue(field.type, input);
      if (value === undefined) { Alert.alert(t("save_failed")); return; }
      void write(value);
    }, "plain-text", fieldText(current), field.type === "number" ? "decimal-pad" : "default");
  };

  return <SafeAreaView className="flex-1 bg-background" edges={[]}>
    <Stack.Screen options={{ title: collection?.name ?? t("untitled"), headerRight: canEdit ? () =>
      <Button variant="ghost" disabled={pending} onPress={createRecord}><Text>{t("create")}</Text></Button> : undefined }} />
    {detail.isLoading || records.isLoading || detail.isError || records.isError || !collection || collection.id !== id ?
      <ResourceState loading={detail.isLoading || records.isLoading} error={detail.isError || records.isError || !collection} retry={refresh} /> :
      <ScrollView className="flex-1" contentContainerClassName="px-4 py-5 pb-12 gap-5"
        refreshControl={<RefreshControl refreshing={detail.isRefetching || records.isRefetching} onRefresh={refresh} />}>
        <View className="gap-2">
          <Pressable disabled={!canManage} onPress={editName}><Text className="text-2xl font-semibold">{collection.icon ? `${collection.icon} ` : ""}{collection.name}</Text></Pressable>
          <Pressable disabled={!canManage} onPress={editDescription}><Text className="text-muted-foreground">{collection.description || (canManage ? t("description") : "")}</Text></Pressable>
          {!canEdit && <Text className="text-sm text-muted-foreground">{t("read_only")}</Text>}
        </View>
        {items.length === 0 && <Text className="text-muted-foreground">{t("empty_records")}</Text>}
        {items.map((record) => <View key={record.id} className="border border-border rounded-lg p-4 gap-2">
          <Pressable disabled={!canEdit || pending} onPress={() => renameRecord(record)}><Text className="text-base font-medium">{record.title || t("untitled")}</Text></Pressable>
          {detail.data?.fields.map((field) => <Pressable key={field.id} disabled={!canEdit || pending || !["text", "number", "date", "checkbox"].includes(field.type)} onPress={() => editField(record, field)}>
            <Text className="text-sm text-muted-foreground" numberOfLines={2}>{field.name}: {fieldText(record.fields[field.id]) || "—"}</Text>
          </Pressable>)}
        </View>)}
        {records.hasNextPage && <Button variant="outline" disabled={records.isFetchingNextPage} onPress={() => void records.fetchNextPage()}><Text>{t("load_more")}</Text></Button>}
      </ScrollView>}
  </SafeAreaView>;
}
