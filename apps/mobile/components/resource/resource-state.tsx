import { ActivityIndicator, View } from "react-native";
import { Button } from "@/components/ui/button";
import { Text } from "@/components/ui/text";
import { useT } from "@/lib/i18n";

export function ResourceState({ loading, error, empty, retry }: {
  loading?: boolean;
  error?: boolean;
  empty?: string;
  retry?: () => void;
}) {
  const { t } = useT("resources");
  if (loading) return <View className="flex-1 items-center justify-center"><ActivityIndicator /></View>;
  return <View className="flex-1 items-center justify-center gap-4 px-6">
    <Text className={error ? "text-destructive" : "text-muted-foreground"}>{error ? t("load_failed") : empty}</Text>
    {error && retry && <Button variant="outline" onPress={retry}><Text>{t("retry")}</Text></Button>}
  </View>;
}
