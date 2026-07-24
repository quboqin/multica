"use client";

import { useEffect, useMemo, useState, type ReactNode } from "react";
import { BarChart3, CircleUser, FolderKanban, Users } from "lucide-react";
import { useQuery } from "@tanstack/react-query";
import {
  Area,
  AreaChart,
  CartesianGrid,
  XAxis,
  YAxis,
} from "recharts";
import { Skeleton } from "@multica/ui/components/ui/skeleton";
import {
  ChartContainer,
  ChartTooltip,
  ChartTooltipContent,
  type ChartConfig,
} from "@multica/ui/components/ui/chart";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@multica/ui/components/ui/select";
import { useAuthStore } from "@multica/core/auth";
import { useWorkspaceId } from "@multica/core/hooks";
import { api } from "@multica/core/api";
import { dashboardKeys } from "@multica/core/dashboard";
import { agentListOptions, memberListOptions } from "@multica/core/workspace/queries";
import { projectListOptions } from "@multica/core/projects/queries";
import { useCustomPricingStore } from "@multica/core/runtimes/custom-pricing-store";
import type { Agent, MemberWithUser } from "@multica/core/types";
import { PageHeader } from "../../layout/page-header";
import { ActorAvatar } from "../../common/actor-avatar";
import { ProjectIcon } from "../../projects/components/project-icon";
import { KpiCard } from "../../runtimes/components/shared";
import { addDaysIso, formatTokens, todayIso } from "../../runtimes/utils";
import { useT } from "../../i18n";
import { useViewingTimezone } from "../../common/use-viewing-timezone";
import {
  aggregateDailyCost,
  aggregateDailyTokens,
  aggregateAgentTokens,
  mergeAgentDashboardRows,
  type AgentDashboardRow,
  aggregateUserTokens,
  type UserUsageRow,
} from "../utils";

const TIME_RANGES = [
  { days: 1 },
  { days: 7 },
  { days: 30 },
] as const;
type TimeRange = (typeof TIME_RANGES)[number]["days"];
type UserUsageScope = "me" | "users";
type SortKey = "cost" | "tokens" | "tasks";
type TrendMetric = "tokens" | "cost";
type TrendAreaPoint = { date: string; label: string; value: number };

const ALL_PROJECTS = "__all__";
const PAGE_SIZE = 10;

function fmtMoney(n: number): string {
  if (n >= 100) return `$${n.toFixed(0)}`;
  return `$${n.toFixed(2)}`;
}

function Segmented<T extends string | number>({
  value,
  onChange,
  options,
}: {
  value: T;
  onChange: (v: T) => void;
  options: readonly { label: string; value: T }[];
}) {
  return (
    <div className="inline-flex items-center gap-0.5 rounded-md bg-muted p-0.5">
      {options.map((o) => (
        <button
          key={String(o.value)}
          type="button"
          onClick={() => onChange(o.value)}
          className={`rounded-sm px-2.5 py-1 text-xs font-medium transition-colors ${
            o.value === value
              ? "bg-background text-foreground shadow-sm"
              : "text-muted-foreground hover:text-foreground"
          }`}
        >
          {o.label}
        </button>
      ))}
    </div>
  );
}

export function UserUsagePage({ scope }: { scope: UserUsageScope }) {
  const { t } = useT("usage");
  const wsId = useWorkspaceId();
  const viewTZ = useViewingTimezone();
  const currentUserId = useAuthStore((s) => s.user?.id ?? "");
  const [days, setDays] = useState<TimeRange>(1);
  const [projectValue, setProjectValue] = useState<string>(ALL_PROJECTS);
  const [userSortBy, setUserSortBy] = useState<SortKey>("tokens");
  const [agentSortBy, setAgentSortBy] = useState<SortKey>("tokens");
  const [userPage, setUserPage] = useState(1);
  const [agentPage, setAgentPage] = useState(1);

  useCustomPricingStore((s) => s.pricings);

  useEffect(() => {
    setDays(1);
  }, [scope]);

  const { data: projects = [] } = useQuery(projectListOptions(wsId));
  const { data: members = [] } = useQuery(memberListOptions(wsId));
  const { data: agents = [] } = useQuery(agentListOptions(wsId));

  const projectId = useMemo(() => {
    if (projectValue === ALL_PROJECTS) return null;
    return projects.some((p) => p.id === projectValue) ? projectValue : null;
  }, [projectValue, projects]);

  const usageQuery = useQuery({
    queryKey: [...dashboardKeys.all(wsId), "user-scope", scope, days, projectId, viewTZ] as const,
    queryFn: () => {
      const params = { days, project_id: projectId ?? undefined, tz: viewTZ };
      return scope === "me"
        ? api.getDashboardUsageMe(params)
        : api.getDashboardUsageByUser(params);
    },
    enabled: !!wsId,
    staleTime: 60 * 1000,
  });

  const trendQuery = useQuery({
    queryKey: [...dashboardKeys.all(wsId), "user-daily-scope", scope, 7, projectId, viewTZ] as const,
    queryFn: () => {
      const params = { days: 7, project_id: projectId ?? undefined, tz: viewTZ };
      return scope === "me"
        ? api.getDashboardUsageMeDaily(params)
        : api.getDashboardUsageByUserDaily(params);
    },
    enabled: !!wsId,
    staleTime: 60 * 1000,
  });

  const agentUsageQuery = useQuery({
    queryKey: [...dashboardKeys.all(wsId), "agent-usage-ranking", days, projectId, viewTZ] as const,
    queryFn: () =>
      api.getDashboardUsageByAgent({
        days,
        project_id: projectId ?? undefined,
        tz: viewTZ,
      }),
    enabled: !!wsId && scope === "users",
    staleTime: 60 * 1000,
  });

  const agentRuntimeQuery = useQuery({
    queryKey: [...dashboardKeys.all(wsId), "agent-runtime-ranking", days, projectId, viewTZ] as const,
    queryFn: () =>
      api.getDashboardAgentRunTime({
        days,
        project_id: projectId ?? undefined,
        tz: viewTZ,
      }),
    enabled: !!wsId && scope === "users",
    staleTime: 60 * 1000,
  });

  const rows = useMemo(
    () => aggregateUserTokens(usageQuery.data ?? []),
    [usageQuery.data],
  );
  const agentRows = useMemo(
    () =>
      mergeAgentDashboardRows(
        aggregateAgentTokens(agentUsageQuery.data ?? []),
        agentRuntimeQuery.data ?? [],
      ),
    [agentUsageQuery.data, agentRuntimeQuery.data],
  );
  const trendDailyCost = useMemo(
    () => aggregateDailyCost(trendQuery.data ?? []),
    [trendQuery.data],
  );
  const trendDailyTokens = useMemo(
    () => aggregateDailyTokens(trendQuery.data ?? []),
    [trendQuery.data],
  );
  const visibleRows = useMemo(() => {
    const metric = userSortBy === "cost"
      ? (r: UserUsageRow) => r.cost
      : userSortBy === "tokens"
        ? (r: UserUsageRow) => r.tokens
        : (r: UserUsageRow) => r.taskCount;
    return rows.toSorted((a, b) => metric(b) - metric(a));
  }, [rows, userSortBy]);
  const visibleAgentRows = useMemo(() => {
    const metric = agentSortBy === "cost"
      ? (r: AgentDashboardRow) => r.cost
      : agentSortBy === "tokens"
        ? (r: AgentDashboardRow) => r.tokens
        : (r: AgentDashboardRow) => r.taskCount;
    return agentRows.toSorted((a, b) => metric(b) - metric(a));
  }, [agentRows, agentSortBy]);

  useEffect(() => {
    setUserPage(1);
    setAgentPage(1);
  }, [days, projectId, userSortBy, agentSortBy, scope]);

  const totals = useMemo(
    () =>
      rows.reduce(
        (acc, r) => ({
          cost: acc.cost + r.cost,
          tokens: acc.tokens + r.tokens,
          taskCount: acc.taskCount + r.taskCount,
          inputTokens: acc.inputTokens + r.inputTokens,
          outputTokens: acc.outputTokens + r.outputTokens,
        }),
        { cost: 0, tokens: 0, taskCount: 0, inputTokens: 0, outputTokens: 0 },
      ),
    [rows],
  );

  const title =
    scope === "me" ? t(($) => $.user_usage.title_me) : t(($) => $.user_usage.title_users);
  const subtitle =
    scope === "me"
      ? t(($) => $.user_usage.subtitle_me)
      : t(($) => $.user_usage.subtitle_users);
  const icon = scope === "me" ? (
    <CircleUser className="h-4 w-4 shrink-0 text-muted-foreground" />
  ) : (
    <Users className="h-4 w-4 shrink-0 text-muted-foreground" />
  );
  const isLoading =
    usageQuery.isLoading ||
    trendQuery.isLoading ||
    (scope === "users" && (agentUsageQuery.isLoading || agentRuntimeQuery.isLoading));
  const isEmpty =
    rows.length === 0 &&
    (trendQuery.data ?? []).length === 0 &&
    (scope !== "users" || agentRows.length === 0);
  const rangeLabel =
    days === 1
      ? t(($) => $.user_usage.range_today)
      : t(($) => $.user_usage.range_days, { days });
  const rangeOptions = TIME_RANGES.map((r) => ({
    label: r.days === 1 ? t(($) => $.user_usage.range_today) : `${r.days}d`,
    value: r.days,
  }));

  return (
    <div className="flex h-full flex-col">
      <PageHeader className="h-auto min-h-12 flex-wrap justify-between gap-y-1.5 px-5 py-1.5 sm:py-0">
        <div className="flex min-w-0 items-center gap-2">
          {icon}
          <h1 className="truncate text-sm font-medium">{title}</h1>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <ProjectFilter
            projects={projects}
            value={projectValue}
            onChange={setProjectValue}
          />
          <Segmented
            value={days}
            onChange={setDays}
            options={rangeOptions}
          />
        </div>
      </PageHeader>

      <div className="flex-1 overflow-y-auto">
        <div className="mx-auto max-w-5xl space-y-5 p-6">
          <p className="text-xs text-muted-foreground">{subtitle}</p>
          {isLoading ? (
            <UserUsageSkeleton />
          ) : isEmpty ? (
            <UserUsageEmpty />
          ) : (
            <>
              <div className="grid grid-cols-1 divide-y rounded-lg border bg-card sm:grid-cols-3 sm:divide-x sm:divide-y-0">
                <KpiCard
                  label={t(($) => $.user_usage.kpi_cost, { range: rangeLabel })}
                  value={fmtMoney(totals.cost)}
                />
                <KpiCard
                  label={t(($) => $.user_usage.kpi_tokens, { range: rangeLabel })}
                  value={formatTokens(totals.tokens)}
                  hint={t(($) => $.kpi.tokens_hint, {
                    input: formatTokens(totals.inputTokens),
                    output: formatTokens(totals.outputTokens),
                  })}
                />
                <KpiCard
                  label={t(($) => $.user_usage.kpi_tasks, { range: rangeLabel })}
                  value={String(totals.taskCount)}
                />
              </div>

              <UserUsageTrend
                dailyCost={trendDailyCost}
                dailyTokens={trendDailyTokens}
                tz={viewTZ}
                scope={scope}
              />

              {scope === "users" && (
                <>
                  <RankingCard
                    title={t(($) => $.user_usage.table_title_agents)}
                    caption={t(($) => $.leaderboard.caption, { count: visibleAgentRows.length })}
                    sortBy={agentSortBy}
                    onSortChange={setAgentSortBy}
                  >
                    <AgentUsageTable
                      rows={visibleAgentRows}
                      agents={agents}
                      sortBy={agentSortBy}
                      page={agentPage}
                      onPageChange={setAgentPage}
                    />
                  </RankingCard>

                  <RankingCard
                    title={t(($) => $.user_usage.table_title_users)}
                    caption={t(($) => $.user_usage.caption, { count: visibleRows.length })}
                    sortBy={userSortBy}
                    onSortChange={setUserSortBy}
                  >
                    <UserUsageTable
                      rows={visibleRows}
                      members={members}
                      currentUserId={currentUserId}
                      sortBy={userSortBy}
                      page={userPage}
                      onPageChange={setUserPage}
                    />
                  </RankingCard>
                </>
              )}
            </>
          )}
        </div>
      </div>
    </div>
  );
}

function ProjectFilter({
  projects,
  value,
  onChange,
}: {
  projects: { id: string; title: string; icon: string | null }[];
  value: string;
  onChange: (v: string) => void;
}) {
  const { t } = useT("usage");
  const allLabel = t(($) => $.filter.all_projects);
  const selected = projects.find((p) => p.id === value);
  const selectedTitle =
    value === ALL_PROJECTS ? allLabel : selected?.title ?? allLabel;

  return (
    <Select value={value} onValueChange={(v) => onChange(v ?? ALL_PROJECTS)}>
      <SelectTrigger size="sm" className="min-w-[180px]">
        <SelectValue>
          {() => (
            <>
              {selected ? (
                <ProjectIcon project={selected} size="sm" />
              ) : (
                <FolderKanban className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
              )}
              <span className="truncate">{selectedTitle}</span>
            </>
          )}
        </SelectValue>
      </SelectTrigger>
      <SelectContent align="start" alignItemWithTrigger={false} className="max-h-72">
        <SelectItem value={ALL_PROJECTS}>
          <FolderKanban className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
          <span className="truncate">{allLabel}</span>
        </SelectItem>
        {projects.map((p) => (
          <SelectItem key={p.id} value={p.id}>
            <ProjectIcon project={p} size="sm" />
            <span className="truncate">{p.title}</span>
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}

function UserUsageTrend({
  dailyCost,
  dailyTokens,
  tz,
  scope,
}: {
  dailyCost: ReturnType<typeof aggregateDailyCost>;
  dailyTokens: ReturnType<typeof aggregateDailyTokens>;
  tz: string;
  scope: UserUsageScope;
}) {
  const { t } = useT("usage");
  const [metric, setMetric] = useState<TrendMetric>("tokens");
  const costData = useMemo(() => fillDailyCostWindow(dailyCost, tz), [dailyCost, tz]);
  const tokensData = useMemo(() => fillDailyTokensWindow(dailyTokens, tz), [dailyTokens, tz]);
  const totalCost = costData.reduce((sum, d) => sum + d.total, 0);
  const totalTokens = tokensData.reduce(
    (sum, d) => sum + d.input + d.output + d.cacheRead + d.cacheWrite,
    0,
  );
  const isEmpty = metric === "cost" ? totalCost === 0 : totalTokens === 0;

  return (
    <div className="rounded-lg border bg-card p-4">
      <div className="mb-3 flex flex-wrap items-center justify-between gap-3">
        <div>
          <h4 className="text-sm font-semibold">
            {t(($) => $.user_usage.trend_title)}
          </h4>
          <p className="mt-1 text-xs text-muted-foreground">
            {scope === "me"
              ? t(($) => $.user_usage.trend_subtitle_me)
              : t(($) => $.user_usage.trend_subtitle_users)}
          </p>
        </div>
        <Segmented
          value={metric}
          onChange={setMetric}
          options={[
            { label: t(($) => $.daily.metric_tokens), value: "tokens" as const },
            { label: t(($) => $.daily.metric_cost), value: "cost" as const },
          ]}
        />
      </div>
      <div className="min-h-[240px]">
        {isEmpty ? (
          <div className="flex aspect-[3/1] flex-col items-center justify-center gap-2 rounded-md border border-dashed bg-muted/20 p-6 text-center">
            <BarChart3 className="h-5 w-5 text-muted-foreground/50" />
            <p className="text-xs text-muted-foreground">
              {t(($) => $.daily.no_data)}
            </p>
          </div>
        ) : metric === "cost" ? (
          <UserUsageAreaChart
            data={costData.map((d) => ({ date: d.date, label: d.label, value: d.total }))}
            metric="cost"
          />
        ) : (
          <UserUsageAreaChart
            data={tokensData.map((d) => ({
              date: d.date,
              label: d.label,
              value: d.input + d.output + d.cacheRead + d.cacheWrite,
            }))}
            metric="tokens"
          />
        )}
      </div>
    </div>
  );
}

const userUsageTrendConfig = {
  value: { label: "Usage", color: "var(--chart-1)" },
} satisfies ChartConfig;

function UserUsageAreaChart({
  data,
  metric,
}: {
  data: TrendAreaPoint[];
  metric: TrendMetric;
}) {
  const gradientId = metric === "cost" ? "user-usage-cost-area" : "user-usage-tokens-area";
  return (
    <ChartContainer config={userUsageTrendConfig} className="aspect-[3/1] w-full">
      <AreaChart data={data} margin={{ left: 0, right: 0, top: 8, bottom: 0 }}>
        <defs>
          <linearGradient id={gradientId} x1="0" y1="0" x2="0" y2="1">
            <stop offset="5%" stopColor="var(--color-value)" stopOpacity={0.36} />
            <stop offset="95%" stopColor="var(--color-value)" stopOpacity={0.04} />
          </linearGradient>
        </defs>
        <CartesianGrid vertical={false} />
        <XAxis
          dataKey="label"
          tickLine={false}
          axisLine={false}
          tickMargin={8}
          interval="preserveStartEnd"
        />
        <YAxis
          tickLine={false}
          axisLine={false}
          tickMargin={8}
          tickFormatter={(v: number) => metric === "cost" ? fmtMoney(v) : formatTokens(v)}
          width={56}
        />
        <ChartTooltip
          content={
            <ChartTooltipContent
              indicator="line"
              formatter={(value) =>
                typeof value === "number"
                  ? metric === "cost"
                    ? fmtMoney(value)
                    : formatTokens(value)
                  : value
              }
            />
          }
        />
        <Area
          type="monotone"
          dataKey="value"
          stroke="var(--color-value)"
          strokeWidth={2}
          fill={`url(#${gradientId})`}
          dot={{ r: 2, strokeWidth: 2 }}
          activeDot={{ r: 4 }}
        />
      </AreaChart>
    </ChartContainer>
  );
}

function fillDailyCostWindow(
  rows: ReturnType<typeof aggregateDailyCost>,
  tz: string,
): ReturnType<typeof aggregateDailyCost> {
  const byDate = new Map(rows.map((r) => [r.date, r] as const));
  return buildSevenDayWindow(tz).map((date) => {
    const row = byDate.get(date);
    return row ?? { date, label: formatDateLabel(date), input: 0, output: 0, cacheWrite: 0, total: 0 };
  });
}

function fillDailyTokensWindow(
  rows: ReturnType<typeof aggregateDailyTokens>,
  tz: string,
): ReturnType<typeof aggregateDailyTokens> {
  const byDate = new Map(rows.map((r) => [r.date, r] as const));
  return buildSevenDayWindow(tz).map((date) => {
    const row = byDate.get(date);
    return row ?? { date, label: formatDateLabel(date), input: 0, output: 0, cacheRead: 0, cacheWrite: 0 };
  });
}

function buildSevenDayWindow(tz: string): string[] {
  const start = addDaysIso(todayIso(tz), -6);
  return Array.from({ length: 7 }, (_, i) => addDaysIso(start, i));
}

function formatDateLabel(d: string): string {
  const date = new Date(d + "T00:00:00");
  return `${date.getMonth() + 1}/${date.getDate()}`;
}

function RankingCard({
  title,
  caption,
  sortBy,
  onSortChange,
  children,
}: {
  title: string;
  caption: string;
  sortBy: SortKey;
  onSortChange: (v: SortKey) => void;
  children: ReactNode;
}) {
  const { t } = useT("usage");
  return (
    <div className="rounded-lg border bg-card">
      <div className="flex flex-wrap items-center justify-between gap-3 border-b px-4 pt-4 pb-3">
        <h4 className="text-sm font-semibold">{title}</h4>
        <div className="flex items-center gap-3">
          <Segmented
            value={sortBy}
            onChange={onSortChange}
            options={[
              { label: t(($) => $.leaderboard.header_cost), value: "cost" as const },
              { label: t(($) => $.leaderboard.header_tokens), value: "tokens" as const },
              { label: t(($) => $.leaderboard.header_tasks), value: "tasks" as const },
            ]}
          />
          <span className="text-xs text-muted-foreground">{caption}</span>
        </div>
      </div>
      {children}
    </div>
  );
}

function AgentUsageTable({
  rows,
  agents,
  sortBy,
  page,
  onPageChange,
}: {
  rows: AgentDashboardRow[];
  agents: Agent[];
  sortBy: SortKey;
  page: number;
  onPageChange: (page: number) => void;
}) {
  const { t } = useT("usage");
  const agentById = useMemo(
    () => new Map(agents.map((a) => [a.id, a] as const)),
    [agents],
  );
  const colClass = (key: SortKey) =>
    `text-right ${sortBy === key ? "text-foreground" : "text-muted-foreground"}`;
  const { pageRows, totalPages, safePage } = usePagedRows(rows, page);

  return (
    <>
      <div className="grid grid-cols-[minmax(0,1.7fr)_5rem_5rem_4rem] items-center gap-3 border-b px-4 py-2 text-xs font-medium text-muted-foreground">
        <span>{t(($) => $.leaderboard.header_agent)}</span>
        <span className={colClass("tokens")}>{t(($) => $.leaderboard.header_tokens)}</span>
        <span className={colClass("cost")}>{t(($) => $.leaderboard.header_cost)}</span>
        <span className={colClass("tasks")}>{t(($) => $.leaderboard.header_tasks)}</span>
      </div>
      <div className="divide-y">
        {pageRows.map((row) => {
          const agent = agentById.get(row.agentId);
          const displayName = agent?.name || row.agentId;
          return (
            <div
              key={row.agentId}
              className="grid grid-cols-[minmax(0,1.7fr)_5rem_5rem_4rem] items-center gap-3 px-4 py-2"
            >
              <div className="flex min-w-0 items-center gap-2">
                <ActorAvatar
                  actorType="agent"
                  actorId={row.agentId}
                  size={22}
                  enableHoverCard
                />
                <div className="min-w-0">
                  <div className="truncate text-sm font-medium">{displayName}</div>
                  {agent?.description && (
                    <div className="truncate text-xs text-muted-foreground">
                      {agent.description}
                    </div>
                  )}
                </div>
              </div>
              <div className={`text-right text-xs tabular-nums ${sortBy === "tokens" ? "font-medium text-foreground" : "text-muted-foreground"}`}>
                {formatTokens(row.tokens)}
              </div>
              <div className={`text-right tabular-nums ${sortBy === "cost" ? "text-sm font-medium" : "text-xs text-muted-foreground"}`}>
                {fmtMoney(row.cost)}
              </div>
              <div className={`text-right text-xs tabular-nums ${sortBy === "tasks" ? "font-medium text-foreground" : "text-muted-foreground"}`}>
                {row.taskCount}
              </div>
            </div>
          );
        })}
      </div>
      <PaginationControls
        page={safePage}
        totalPages={totalPages}
        totalRows={rows.length}
        onPageChange={onPageChange}
      />
    </>
  );
}

function UserUsageTable({
  rows,
  members,
  currentUserId,
  sortBy,
  page,
  onPageChange,
}: {
  rows: UserUsageRow[];
  members: MemberWithUser[];
  currentUserId: string;
  sortBy: SortKey;
  page: number;
  onPageChange: (page: number) => void;
}) {
  const { t } = useT("usage");
  const memberByUserId = useMemo(
    () => new Map(members.map((m) => [m.user_id, m] as const)),
    [members],
  );
  const colClass = (key: SortKey) =>
    `text-right ${sortBy === key ? "text-foreground" : "text-muted-foreground"}`;
  const { pageRows, totalPages, safePage } = usePagedRows(rows, page);

  return (
    <>
      <div className="grid grid-cols-[minmax(0,1.7fr)_5rem_5rem_5rem_5rem_4rem] items-center gap-3 border-b px-4 py-2 text-xs font-medium text-muted-foreground">
        <span>{t(($) => $.user_usage.header_user)}</span>
        <span className="text-right">{t(($) => $.user_usage.header_input)}</span>
        <span className="text-right">{t(($) => $.user_usage.header_output)}</span>
        <span className={colClass("tokens")}>{t(($) => $.leaderboard.header_tokens)}</span>
        <span className={colClass("cost")}>{t(($) => $.leaderboard.header_cost)}</span>
        <span className={colClass("tasks")}>{t(($) => $.leaderboard.header_tasks)}</span>
      </div>
      <div className="divide-y">
        {pageRows.map((row) => {
          const member = memberByUserId.get(row.userId);
          const displayName = member?.name || member?.email || row.userId;
          return (
            <div
              key={row.userId}
              className="grid grid-cols-[minmax(0,1.7fr)_5rem_5rem_5rem_5rem_4rem] items-center gap-3 px-4 py-2"
            >
              <div className="flex min-w-0 items-center gap-2">
                <ActorAvatar
                  actorType="member"
                  actorId={row.userId}
                  size={22}
                  enableHoverCard
                />
                <div className="min-w-0">
                  <div className="truncate text-sm font-medium">
                    {displayName}
                    {row.userId === currentUserId && (
                      <span className="ml-1 text-xs font-normal text-muted-foreground">
                        {t(($) => $.user_usage.current_user)}
                      </span>
                    )}
                  </div>
                  {member?.email && (
                    <div className="truncate text-xs text-muted-foreground">
                      {member.email}
                    </div>
                  )}
                </div>
              </div>
              <div className="text-right text-xs tabular-nums text-muted-foreground">
                {formatTokens(row.inputTokens)}
              </div>
              <div className="text-right text-xs tabular-nums text-muted-foreground">
                {formatTokens(row.outputTokens)}
              </div>
              <div className={`text-right text-xs tabular-nums ${sortBy === "tokens" ? "font-medium text-foreground" : "text-muted-foreground"}`}>
                {formatTokens(row.tokens)}
              </div>
              <div className={`text-right tabular-nums ${sortBy === "cost" ? "text-sm font-medium" : "text-xs text-muted-foreground"}`}>
                {fmtMoney(row.cost)}
              </div>
              <div className={`text-right text-xs tabular-nums ${sortBy === "tasks" ? "font-medium text-foreground" : "text-muted-foreground"}`}>
                {row.taskCount}
              </div>
            </div>
          );
        })}
      </div>
      <PaginationControls
        page={safePage}
        totalPages={totalPages}
        totalRows={rows.length}
        onPageChange={onPageChange}
      />
    </>
  );
}

function usePagedRows<T>(rows: T[], page: number) {
  return useMemo(() => {
    const totalPages = Math.max(1, Math.ceil(rows.length / PAGE_SIZE));
    const safePage = Math.min(Math.max(1, page), totalPages);
    const start = (safePage - 1) * PAGE_SIZE;
    return {
      pageRows: rows.slice(start, start + PAGE_SIZE),
      totalPages,
      safePage,
    };
  }, [rows, page]);
}

function PaginationControls({
  page,
  totalPages,
  totalRows,
  onPageChange,
}: {
  page: number;
  totalPages: number;
  totalRows: number;
  onPageChange: (page: number) => void;
}) {
  const { t } = useT("usage");
  if (totalRows <= PAGE_SIZE) return null;
  return (
    <div className="flex items-center justify-between border-t px-4 py-3">
      <span className="text-xs text-muted-foreground">
        {t(($) => $.user_usage.pagination_status, { page, total: totalPages })}
      </span>
      <div className="flex items-center gap-2">
        <button
          type="button"
          className="rounded-md border px-2.5 py-1 text-xs font-medium text-muted-foreground transition-colors hover:text-foreground disabled:pointer-events-none disabled:opacity-50"
          disabled={page <= 1}
          onClick={() => onPageChange(page - 1)}
        >
          {t(($) => $.user_usage.pagination_prev)}
        </button>
        <button
          type="button"
          className="rounded-md border px-2.5 py-1 text-xs font-medium text-muted-foreground transition-colors hover:text-foreground disabled:pointer-events-none disabled:opacity-50"
          disabled={page >= totalPages}
          onClick={() => onPageChange(page + 1)}
        >
          {t(($) => $.user_usage.pagination_next)}
        </button>
      </div>
    </div>
  );
}

function UserUsageSkeleton() {
  return (
    <div className="space-y-5">
      <Skeleton className="h-28 rounded-lg" />
      <Skeleton className="h-56 rounded-lg" />
    </div>
  );
}

function UserUsageEmpty() {
  const { t } = useT("usage");
  return (
    <div className="flex flex-col items-center rounded-lg border border-dashed py-12 text-center">
      <BarChart3 className="h-6 w-6 text-muted-foreground/40" />
      <p className="mt-3 text-sm font-medium">{t(($) => $.user_usage.empty_title)}</p>
      <p className="mt-1 max-w-md text-xs text-muted-foreground">
        {t(($) => $.user_usage.empty_body)}
      </p>
    </div>
  );
}
