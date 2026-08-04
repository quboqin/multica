import { describe, expect, it } from "vitest";
import type { AutopilotRun } from "@multica/core/types";
import { crawlRunAnalysisProgress, matchCollectionPlans } from "./creative-collection-plans";

describe("matchCollectionPlans", () => {
  it("uses a CrawlRun autopilot run to locate its native collection plan", () => {
    const plans = [{ id: "plan-1", execution_mode: "run_only", title: "AppGrowing" }] as any[];
    const runs = new Map([["plan-1", [{ id: "autopilot-run-1", autopilot_id: "plan-1" } as AutopilotRun]]]);
    const crawlRuns = [{ id: "crawl-1", autopilot_run_id: "autopilot-run-1" }] as any[];

    expect(matchCollectionPlans(plans, runs, crawlRuns)).toEqual([{ plan: plans[0], crawlRun: crawlRuns[0] }]);
  });

  it("keeps a native run-only plan visible before its first CrawlRun", () => {
    const plans = [{ id: "plan-1", execution_mode: "run_only" }] as any[];
    expect(matchCollectionPlans(plans, new Map(), [])).toEqual([{ plan: plans[0], crawlRun: undefined }]);
  });

  it("does not attach an unrelated CrawlRun to a visible plan", () => {
    const plans = [{ id: "plan-1", execution_mode: "run_only" }] as any[];
    const runs = new Map([["plan-1", [{ id: "autopilot-run-1" } as AutopilotRun]]]);
    const crawlRuns = [{ id: "crawl-1", autopilot_run_id: "other-run" }] as any[];

    expect(matchCollectionPlans(plans, runs, crawlRuns)).toEqual([{ plan: plans[0], crawlRun: undefined }]);
  });

  it("uses only the newest CrawlRun for each plan", () => {
    const plans = [{ id: "plan-1", execution_mode: "run_only" }] as any[];
    const runs = new Map([["plan-1", [
      { id: "autopilot-run-1" } as AutopilotRun,
      { id: "autopilot-run-2" } as AutopilotRun,
    ]]]);
    const older = { id: "crawl-1", autopilot_run_id: "autopilot-run-1", created_at: "2026-08-03T00:00:00Z" } as any;
    const newer = { id: "crawl-2", autopilot_run_id: "autopilot-run-2", created_at: "2026-08-04T00:00:00Z" } as any;

    expect(matchCollectionPlans(plans, runs, [newer, older])).toEqual([{ plan: plans[0], crawlRun: newer }]);
  });

  it("keeps the last imported batch visible when the newest run only reused history", () => {
    const plans = [{ id: "plan-1", execution_mode: "run_only" }] as any[];
    const runs = new Map([["plan-1", [
      { id: "autopilot-run-imported" } as AutopilotRun,
      { id: "autopilot-run-reused" } as AutopilotRun,
    ]]]);
    const imported = { id: "crawl-imported", autopilot_run_id: "autopilot-run-imported", imported_count: 20, existing_count: 5, created_at: "2026-08-03T00:00:00Z" } as any;
    const reused = { id: "crawl-reused", autopilot_run_id: "autopilot-run-reused", imported_count: 0, existing_count: 25, created_at: "2026-08-04T00:00:00Z" } as any;

    expect(matchCollectionPlans(plans, runs, [reused, imported])).toEqual([{
      plan: plans[0],
      crawlRun: reused,
      lastImportedCrawlRun: imported,
    }]);
  });

  it("uses only newly imported materials as the analysis denominator", () => {
    expect(crawlRunAnalysisProgress({ imported_count: 20, candidate_metrics: { analyzed: 20, analysis_failed: 0 } } as any))
      .toEqual({ label: "新增分析完成 20/20", variant: "default" });
    expect(crawlRunAnalysisProgress({ imported_count: 0, candidate_metrics: { analyzed: 0, analysis_failed: 0 } } as any))
      .toEqual({ label: "无新增待分析", variant: "outline" });
  });
});
