#!/usr/bin/env python3
"""Delegate one visible reference-analysis Issue per newly crawled image."""

from __future__ import annotations

import argparse
import json
import subprocess
import tempfile
from pathlib import Path
from typing import Any


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser()
    parser.add_argument("--target-issue", required=True)
    parser.add_argument("--coordinator-issue", required=True)
    parser.add_argument("--assignee", default="广告参考分析智能体")
    parser.add_argument("--max-concurrency", type=int, default=4)
    parser.add_argument("--profile", default="")
    parser.add_argument("--materials-file")
    parser.add_argument("--children-file")
    parser.add_argument("--output-dir", default="")
    parser.add_argument("--dry-run", action="store_true")
    return parser.parse_args()


def run_cli(base: list[str], args: list[str]) -> tuple[dict[str, Any], subprocess.CompletedProcess[str]]:
    completed = subprocess.run(
        [*base, *args],
        check=False,
        capture_output=True,
        text=True,
        encoding="utf-8",
    )
    payload: dict[str, Any] = {}
    if completed.stdout.strip():
        payload = json.loads(completed.stdout)
    if completed.returncode != 0 and not payload:
        raise RuntimeError(completed.stderr.strip() or f"multica {' '.join(args)} failed")
    return payload, completed


def load_json(path: str) -> dict[str, Any]:
    return json.loads(Path(path).read_text(encoding="utf-8-sig"))


def latest_run(materials: dict[str, Any]) -> dict[str, Any] | None:
    runs = materials.get("crawl_runs") or []
    return runs[0] if runs else None


def pending_candidates(
    materials: dict[str, Any], children_payload: dict[str, Any], target_issue: str
) -> tuple[dict[str, Any] | None, list[dict[str, Any]]]:
    run = latest_run(materials)
    if not run:
        return None, []
    run_id = str(run.get("id") or "")
    source_issue_id = str(run.get("issue_id") or target_issue)
    items = {str(item.get("candidate_id")): item for item in materials.get("items") or []}
    existing = {
        str((child.get("metadata") or {}).get("creative_candidate_id") or "")
        for child in children_payload.get("issues") or []
        if child.get("status") != "cancelled"
        and (child.get("metadata") or {}).get("workflow") == "creative_reference_analysis"
    }
    pending: list[dict[str, Any]] = []
    for candidate in materials.get("candidates") or []:
        candidate_id = str(candidate.get("id") or "")
        brief = (items.get(candidate_id) or {}).get("creative_brief") or {}
        if (
            not candidate_id
            or candidate_id in existing
            or str(candidate.get("source_issue_id") or "") != source_issue_id
            or str(candidate.get("source_run_id") or "") != run_id
            or candidate.get("is_new_in_run") is not True
            or candidate.get("asset_type") != "image"
            or candidate.get("archive_status") != "completed"
            or str(brief.get("primary_benefit") or "").strip()
        ):
            continue
        pending.append(candidate)
    return run, pending


def build_manifest(
    candidates: list[dict[str, Any]], target_issue: str, coordinator_issue: str,
    run_id: str, assignee: str, max_concurrency: int,
) -> dict[str, Any]:
    issues = []
    for candidate in candidates:
        candidate_id = str(candidate["id"])
        label = str(candidate.get("title") or candidate.get("competitor") or candidate_id)
        issues.append({
            "key": candidate_id,
            "title": f"参考分析 · {label}",
            "description": (
                f"目标候选池 Issue：{target_issue}\n"
                f"候选 ID：{candidate_id}\n"
                f"来源采集批次：{run_id}\n\n"
                "使用广告参考分析 Skill 读取平台归档图片的真实像素，识别视觉主题、金融利益点、"
                "业务语义、信息机制、视觉与色系锚点、必须保留项和允许变化项。将结构化创意简报"
                "回写目标候选图；采集标题、标签和媒体只能弱辅助。不生成图片。"
            ),
            "metadata": {
                "workflow": "creative_reference_analysis",
                "target_issue_id": target_issue,
                "creative_candidate_id": candidate_id,
                "source_run_id": run_id,
            },
        })
    return {
        "max_concurrency": max_concurrency,
        "defaults": {
            "parent": coordinator_issue,
            "status": "todo",
            "priority": "high",
            "assignee": assignee,
        },
        "issues": issues,
    }


def main() -> int:
    args = parse_args()
    if args.max_concurrency < 1 or args.max_concurrency > 8:
        raise SystemExit("--max-concurrency must be between 1 and 8")
    cli = ["multica"]
    if args.profile:
        cli.extend(["--profile", args.profile])

    if args.materials_file:
        materials = load_json(args.materials_file)
    else:
        materials, _ = run_cli(cli, ["creative", "materials", args.target_issue, "--output", "json"])
    if args.children_file:
        children = load_json(args.children_file)
    else:
        children, _ = run_cli(cli, ["issue", "children", args.coordinator_issue, "--compact", "--output", "json"])

    run, candidates = pending_candidates(materials, children, args.target_issue)
    manifest = build_manifest(
        candidates,
        args.target_issue,
        args.coordinator_issue,
        str((run or {}).get("id") or ""),
        args.assignee,
        args.max_concurrency,
    )
    if args.dry_run or not candidates:
        print(json.dumps({
            "target_issue_id": args.target_issue,
            "source_run_id": str((run or {}).get("id") or ""),
            "eligible": len(candidates),
            "manifest": manifest,
        }, ensure_ascii=False, indent=2))
        return 0

    output_dir = Path(args.output_dir) if args.output_dir else Path(tempfile.mkdtemp(prefix="multica-preanalysis-"))
    output_dir.mkdir(parents=True, exist_ok=True)
    manifest_path = output_dir / "preanalysis-manifest.json"
    manifest_path.write_text(json.dumps(manifest, ensure_ascii=False, indent=2), encoding="utf-8")
    batch, batch_process = run_cli(cli, ["issue", "create-batch", "--input-file", str(manifest_path), "--output", "json"])

    item_by_candidate = {
        str(item.get("candidate_id")): item for item in materials.get("items") or []
    }
    requested = 0
    request_errors: list[dict[str, str]] = []
    for result in batch.get("results") or []:
        if result.get("status") != "created":
            continue
        candidate_id = str(result.get("key") or "")
        analysis_issue_id = str(result.get("id") or "")
        current = dict((item_by_candidate.get(candidate_id) or {}).get("creative_brief") or {})
        current.update({
            "status": "requested",
            "source": "mixed" if current.get("source") in {"user", "mixed"} else "ai",
            "analysis_issue_id": analysis_issue_id,
        })
        brief_path = output_dir / f"brief-{candidate_id}.json"
        brief_path.write_text(json.dumps(current, ensure_ascii=False, indent=2), encoding="utf-8")
        try:
            _, process = run_cli(cli, [
                "creative", "material", "brief", args.target_issue, candidate_id,
                "--input-file", str(brief_path), "--output", "json",
            ])
            if process.returncode == 0:
                requested += 1
            else:
                request_errors.append({"candidate_id": candidate_id, "error": process.stderr.strip()})
        except Exception as error:  # Preserve the created Issue even if the status marker fails.
            request_errors.append({"candidate_id": candidate_id, "error": str(error)})

    summary = {
        "target_issue_id": args.target_issue,
        "source_run_id": str((run or {}).get("id") or ""),
        "eligible": len(candidates),
        "created": int(batch.get("created") or 0),
        "failed": int(batch.get("failed") or 0),
        "briefs_marked_requested": requested,
        "brief_marker_errors": request_errors,
        "manifest": str(manifest_path),
    }
    print(json.dumps(summary, ensure_ascii=False, indent=2))
    return 1 if batch_process.returncode != 0 or request_errors else 0


if __name__ == "__main__":
    raise SystemExit(main())
