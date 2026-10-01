import { useCallback } from "react";
import { RefreshControl, ScrollView, View } from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import { Stack, router, useLocalSearchParams } from "expo-router";
import { useQuery } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { Text } from "@/components/ui/text";
import { ResourceState } from "@/components/resource/resource-state";
import { documentAccessOptions } from "@/data/queries/resources";
import { issueAttachmentsOptions, issueDetailOptions } from "@/data/queries/issues";
import { useWorkspaceStore } from "@/data/workspace-store";
import { useIssueRealtime } from "@/data/realtime/use-issue-realtime";
import { Markdown } from "@/lib/markdown/markdown";
import { useT } from "@/lib/i18n";

export default function DocumentDetailPage() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const wsId = useWorkspaceStore((s) => s.currentWorkspaceId);
  const slug = useWorkspaceStore((s) => s.currentWorkspaceSlug);
  const { t } = useT("resources");
  const doc = useQuery(issueDetailOptions(wsId, id));
  const access = useQuery(documentAccessOptions(wsId, id));
  const attachments = useQuery(issueAttachmentsOptions(wsId, id));
  useIssueRealtime(id, () => router.back());
  const refresh = useCallback(() => { void doc.refetch(); void access.refetch(); }, [doc, access]);
  const document = doc.data?.id === id && doc.data.kind === "doc" ? doc.data : null;
  const canEdit = access.data?.can_edit === true;

  return <SafeAreaView className="flex-1 bg-background" edges={[]}>
    <Stack.Screen options={{ title: document?.title ?? t("untitled"), headerRight: canEdit && document ? () =>
      <Button variant="ghost" onPress={() => slug && router.push(`/${slug}/document/${id}/edit`)}><Text>{t("edit")}</Text></Button> : undefined }} />
    {doc.isLoading || access.isLoading || doc.isError || access.isError || !document ?
      <ResourceState loading={doc.isLoading || access.isLoading} error={doc.isError || access.isError || !document} retry={refresh} /> :
      <ScrollView className="flex-1" contentContainerClassName="px-4 py-5 pb-12"
        refreshControl={<RefreshControl refreshing={doc.isRefetching || access.isRefetching} onRefresh={refresh} />}>
        <Text className="text-2xl font-semibold mb-4">{document.title}</Text>
        {!canEdit && <Text className="text-sm text-muted-foreground mb-4">{t("read_only")}</Text>}
        {document.description ? <Markdown content={document.description} attachments={attachments.data} /> :
          <View><Text className="text-muted-foreground">{t("no_content")}</Text></View>}
      </ScrollView>}
  </SafeAreaView>;
}
