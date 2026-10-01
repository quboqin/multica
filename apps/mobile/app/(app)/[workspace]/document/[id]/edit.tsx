import { useEffect, useState } from "react";
import { Alert, KeyboardAvoidingView, Platform, ScrollView, TextInput, View } from "react-native";
import { Stack, router, useLocalSearchParams } from "expo-router";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { Text } from "@/components/ui/text";
import { AutosizeTextArea } from "@/components/ui/autosize-textarea";
import { ResourceState } from "@/components/resource/resource-state";
import { api, ApiError } from "@/data/api";
import { documentAccessOptions, documentKeys } from "@/data/queries/resources";
import { issueDetailOptions } from "@/data/queries/issues";
import { issueKeys } from "@/data/queries/issue-keys";
import { useWorkspaceStore } from "@/data/workspace-store";
import { buildDocumentUpdate, type DocumentBase } from "@/data/resource-edits";
import { useT } from "@/lib/i18n";

export default function EditDocumentPage() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const wsId = useWorkspaceStore((s) => s.currentWorkspaceId);
  const qc = useQueryClient();
  const { t } = useT("resources");
  const doc = useQuery(issueDetailOptions(wsId, id));
  const access = useQuery(documentAccessOptions(wsId, id));
  const [title, setTitle] = useState("");
  const [body, setBody] = useState("");
  const [base, setBase] = useState<DocumentBase | null>(null);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (!doc.data || doc.data.id !== id || doc.data.kind !== "doc" || base) return;
    const next = { title: doc.data.title, body: doc.data.description ?? "", revision: doc.data.document_revision ?? 1 };
    setBase(next); setTitle(next.title); setBody(next.body);
  }, [doc.data, id, base]);

  const save = async () => {
    if (!base || !title.trim() || !access.data?.can_edit || saving) return;
    setSaving(true);
    try {
      await api.updateIssue(id, buildDocumentUpdate(base, title, body));
      await Promise.all([
        qc.invalidateQueries({ queryKey: issueKeys.detail(wsId, id) }),
        qc.invalidateQueries({ queryKey: documentKeys.list(wsId) }),
      ]);
      router.back();
    } catch (error) {
      if (error instanceof ApiError && error.status === 409) {
        Alert.alert(t("conflict"), undefined, [
          { text: t("cancel"), style: "cancel" },
          { text: t("retry"), onPress: () => {
            void api.getIssue(id).then((latest) => {
              const next = { title: latest.title, body: latest.description ?? "", revision: latest.document_revision ?? 1 };
              setBase(next); setTitle(next.title); setBody(next.body);
            }).catch(() => Alert.alert(t("load_failed")));
          } },
        ]);
      } else Alert.alert(t("save_failed"));
    } finally { setSaving(false); }
  };

  const invalid = doc.data?.kind !== "doc" || !access.data?.can_edit;
  return <KeyboardAvoidingView className="flex-1 bg-background" behavior={Platform.OS === "ios" ? "padding" : undefined}>
    <Stack.Screen options={{ headerRight: () => <Button variant="ghost" disabled={saving || !base || invalid || !title.trim()} onPress={() => void save()}><Text>{t("save")}</Text></Button> }} />
    {doc.isLoading || access.isLoading || doc.isError || access.isError || invalid ?
      <ResourceState loading={doc.isLoading || access.isLoading} error={doc.isError || access.isError || !access.data} empty={t("read_only")} retry={() => { void doc.refetch(); void access.refetch(); }} /> :
      <ScrollView className="flex-1" keyboardShouldPersistTaps="handled" contentContainerClassName="px-4 py-5 gap-6">
        <View className="gap-2"><Text className="text-sm font-medium">{t("document_title")}</Text>
          <TextInput className="text-base text-foreground border border-border rounded-md px-3 py-2" value={title} onChangeText={setTitle} maxLength={2048} /></View>
        <View className="gap-2"><Text className="text-sm font-medium">{t("body")}</Text>
          <AutosizeTextArea className="border border-border rounded-md px-3 py-2" minHeight={220} maxHeight={500} value={body} onChangeText={setBody} /></View>
      </ScrollView>}
  </KeyboardAvoidingView>;
}
