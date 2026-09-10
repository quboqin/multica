import { describe, expect, it } from "vitest";
import type { AutopilotRun } from "@multica/core/types";
import { crawlRunForAutopilotRun, crawlRunImportBadgeLabel, crawlRunNeedsRefresh, latestCollectionCrawlRunId, matchCollectionPlans, newMaterialsFromCrawlRun, resolveCollectionCrawlRunId } from "./creative-collection-plans";

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

  it("does not show a previous crawl result while a newer autopilot run is active", () => {
    const plans = [{ id: "plan-1", execution_mode: "run_only" }] as any[];
    const previousRun = { id: "autopilot-run-old", status: "completed", created_at: "2026-08-03T00:00:00Z" } as AutopilotRun;
    const activeRun = { id: "autopilot-run-new", status: "running", created_at: "2026-08-04T00:00:00Z" } as AutopilotRun;
    const previousCrawl = { id: "crawl-old", autopilot_run_id: previousRun.id, status: "cancelled", created_at: "2026-08-03T00:01:00Z" } as any;

    expect(matchCollectionPlans(plans, new Map([["plan-1", [previousRun, activeRun]]]), [previousCrawl])).toEqual([{
      plan: plans[0],
      activeAutopilotRun: activeRun,
    }]);
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

  it("labels collection batches by imported materials only", () => {
    expect(crawlRunImportBadgeLabel({ status: "completed", imported_count: 20 } as any))
      .toBe("Imported 20 materials this run");
    expect(crawlRunImportBadgeLabel({ status: "completed", imported_count: 0 } as any))
      .toBe("No new materials this run");
  });

  it("does not present an active zero-import CrawlRun as a terminal empty result", () => {
    expect(crawlRunNeedsRefresh({ status: "running", imported_count: 0 } as any)).toBe(true);
    expect(crawlRunImportBadgeLabel({ status: "running", imported_count: 0 } as any)).toBe("Looking for new materials");

    expect(crawlRunNeedsRefresh({ status: "completed", imported_count: 0 } as any)).toBe(false);
    expect(crawlRunImportBadgeLabel({ status: "completed", imported_count: 0 } as any)).toBe("No new materials this run");
  });

  it("keeps only new material records when opening one exact crawl run", () => {
    const candidates = [
      { id: "new-pending", source_run_id: "crawl-1", is_new_in_run: true, asset_type: "image", analysis_status: "pending" },
      { id: "new-running", source_run_id: "crawl-1", is_new_in_run: true, asset_type: "image", analysis_status: "running" },
      { id: "new-completed", source_run_id: "crawl-1", is_new_in_run: true, asset_type: "image", analysis_status: "completed" },
      { id: "new-failed", source_run_id: "crawl-1", is_new_in_run: true, asset_type: "image", analysis_status: "failed" },
      { id: "new-video", source_run_id: "crawl-1", is_new_in_run: true, asset_type: "video", analysis_status: "pending" },
      { id: "reused-history", source_run_id: "crawl-1", is_new_in_run: false, asset_type: "image", analysis_status: "completed" },
    ] as any[];

    const newCandidates = newMaterialsFromCrawlRun(candidates);
    expect(newCandidates.map((candidate) => candidate.id)).toEqual(["new-pending", "new-running", "new-completed", "new-failed", "new-video"]);
  });
});

describe("resolveCollectionCrawlRunId", () => {
  it("keeps the selected CrawlRun when it belongs to a collection plan", () => {
    expect(resolveCollectionCrawlRunId("crawl-current", [{ id: "crawl-current" }] as any[])).toBe("crawl-current");
  });

  it("rejects a stale manual-import CrawlRun from the collection panel", () => {
    expect(resolveCollectionCrawlRunId("manual-import", [{ id: "crawl-current" }] as any[])).toBe("");
  });
});

describe("crawlRunForAutopilotRun", () => {
  it("never falls back to a manual import while a newly triggered autopilot run is still creating its CrawlRun", () => {
    const manual = { id: "manual-import", autopilot_run_id: "", created_at: "2026-08-08T10:00:00Z" } as any;
    const automatic = { id: "automatic-run", autopilot_run_id: "autopilot-run-1", created_at: "2026-08-08T11:00:00Z" } as any;

    expect(crawlRunForAutopilotRun([manual], "autopilot-run-2")).toBeUndefined();
    expect(crawlRunForAutopilotRun([manual, automatic], "autopilot-run-1")).toBe(automatic);
  });
});

describe("latestCollectionCrawlRunId", () => {
  it("opens the newest collection batch by default", () => {
    expect(latestCollectionCrawlRunId([
      { id: "crawl-old", created_at: "2026-08-08T10:00:00Z" },
      { id: "crawl-new", created_at: "2026-08-08T11:00:00Z" },
    ] as any[])).toBe("crawl-new");
  });
});
