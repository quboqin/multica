#!/usr/bin/env python3
"""Fan out one native reference-analysis task per eligible Crawl Run image."""

from __future__ import annotations

import argparse
import json
import os
import shutil
import subprocess
import tempfile
from pathlib import Path
from typing import Any


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser()
    parser.add_argument("--crawl-run-id", required=True)
    parser.add_argument("--assignee-id", required=True)
    parser.add_argument("--analysis-version", type=int, default=1)
    parser.add_argument("--profile", default="")
    parser.add_argument(
        "--cli",
        default="",
        help="Absolute task-runtime multica executable; defaults to MULTICA_CLI or multica on PATH.",
    )
    parser.add_argument("--materials-file")
    parser.add_argument("--output-dir", default="")
    parser.add_argument("--dry-run", action="store_true")
    return parser.parse_args()


def resolve_cli(explicit_cli: str) -> str:
    if explicit_cli.strip():
        return explicit_cli.strip()
    if task_runtime_cli := os.environ.get("MULTICA_CLI", "").strip():
        return task_runtime_cli
    # Windows CreateProcess can prefer a stale multica.exe while PowerShell and
    # the daemon launch the current multica.com shim in the same directory.
    if os.name == "nt" and (windows_cli := shutil.which("multica.com")):
        return windows_cli
    return "multica"


def ensure_task_fanout_available(cli: list[str]) -> None:
    try:
        completed = subprocess.run(
            [*cli, "task", "fanout", "--help"],
            check=False,
            capture_output=True,
            text=True,
            encoding="utf-8",
        )
    except OSError as error:
        raise RuntimeError(
            f"multica task fanout unavailable; upgrade CLI ({error})"
        ) from error
    if completed.returncode == 0:
        return
    detail = (completed.stderr.strip() or completed.stdout.strip() or "command unavailable")
    raise RuntimeError(f"multica task fanout unavailable; upgrade CLI ({detail})")


def run_cli(base: list[str], args: list[str]) -> dict[str, Any]:
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
    if completed.returncode != 0:
        message = completed.stderr.strip() or payload.get("error") or "command failed"
        raise RuntimeError(f"multica {' '.join(args)}: {message}")
    return payload


def load_json(path: str) -> dict[str, Any]:
    return json.loads(Path(path).read_text(encoding="utf-8-sig"))


def eligible_candidates(materials: dict[str, Any], run_id: str) -> list[dict[str, Any]]:
    eligible: list[dict[str, Any]] = []
    for candidate in materials.get("candidates") or []:
        candidate_id = str(candidate.get("id") or "")
        analysis_status = str(candidate.get("analysis_status") or "")
        if (
            not candidate_id
            or str(candidate.get("source_run_id") or "") != run_id
            or candidate.get("is_new_in_run") is not True
            or candidate.get("asset_type") != "image"
            or analysis_status in {"queued", "running", "completed"}
        ):
            continue
        eligible.append(candidate)
    return eligible


def build_manifest(
    candidates: list[dict[str, Any]], run_id: str, analysis_version: int
) -> dict[str, Any]:
    return {
        "trigger_evidence_kind": "creative_crawl_run_analysis",
        "trigger_evidence_ref_id": run_id,
        "items": [
            {
                "item_key": f"{candidate['id']}:v{analysis_version}",
                "context": {
                    "type": "creative_domain_task",
                    "workflow": "creative_reference_analysis",
                    "crawl_run_id": run_id,
                    "candidate_id": str(candidate["id"]),
                    "analysis_version": analysis_version,
                },
            }
            for candidate in candidates
        ],
    }


def main() -> int:
    args = parse_args()
    if args.analysis_version < 1:
        raise SystemExit("--analysis-version must be positive")

    cli = [resolve_cli(args.cli)]
    if args.profile:
        cli.extend(["--profile", args.profile])

    # Do this before the library read so an outdated task-runtime CLI cannot
    # make a successful crawl look complete when it cannot enqueue analysis.
    if not args.dry_run:
        ensure_task_fanout_available(cli)

    materials = (
        load_json(args.materials_file)
        if args.materials_file
        else run_cli(
            cli,
            [
                "creative",
                "library",
                "list",
                "--run-id",
                args.crawl_run_id,
                "--output",
                "json",
            ],
        )
    )
    candidates = eligible_candidates(materials, args.crawl_run_id)
    manifest = build_manifest(candidates, args.crawl_run_id, args.analysis_version)

    if args.dry_run or not candidates:
        print(
            json.dumps(
                {
                    "crawl_run_id": args.crawl_run_id,
                    "eligible": len(candidates),
                    "manifest": manifest,
                },
                ensure_ascii=False,
                indent=2,
            )
        )
        return 0

    output_dir = (
        Path(args.output_dir)
        if args.output_dir
        else Path(tempfile.mkdtemp(prefix="multica-preanalysis-"))
    )
    output_dir.mkdir(parents=True, exist_ok=True)
    manifest_path = output_dir / "preanalysis-task-fanout.json"
    manifest_path.write_text(
        json.dumps(manifest, ensure_ascii=False, indent=2), encoding="utf-8"
    )

    result = run_cli(
        cli,
        [
            "task",
            "fanout",
            "--agent",
            args.assignee_id,
            "--input-file",
            str(manifest_path),
            "--output",
            "json",
        ],
    )
    tasks = result.get("tasks") or []
    accepted = len(tasks)
    summary = {
        "crawl_run_id": args.crawl_run_id,
        "eligible": len(candidates),
        "tasks_accepted": accepted,
        "task_ids": [task.get("id") for task in tasks if task.get("id")],
        "manifest": str(manifest_path),
    }
    print(json.dumps(summary, ensure_ascii=False, indent=2))
    return 1 if accepted != len(candidates) else 0


if __name__ == "__main__":
    raise SystemExit(main())
