import { redirect } from "next/navigation";

export default async function LegacyPlansKpiRoute({
  params,
}: {
  params: Promise<{ workspaceSlug: string }>;
}) {
  const { workspaceSlug } = await params;
  redirect(`/${encodeURIComponent(workspaceSlug)}/kpi`);
}
