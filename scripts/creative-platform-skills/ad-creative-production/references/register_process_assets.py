#!/usr/bin/env python3
"""Upload and register deterministic Creative Order process images."""

from __future__ import annotations

import argparse
import json
import subprocess
import tempfile
from pathlib import Path


ALLOWED_LABELS = {
    "Prime context",
    "模型原图",
    "规范化底图",
    "Prime 合成成图",
}


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser()
    parser.add_argument("--order-id", required=True)
    parser.add_argument("--variant-id", required=True)
    parser.add_argument("--revision", required=True, type=int)
    parser.add_argument("--task-id", default="")
    parser.add_argument("--workflow", default="creative_production")
    parser.add_argument("--cli", default="multica")
    parser.add_argument("--profile", default="")
    parser.add_argument("--image", action="append", nargs=3, metavar=("SIZE", "LABEL", "PATH"), required=True)
    return parser.parse_args()


def run_json(command: list[str]) -> dict[str, object]:
    completed = subprocess.run(command, check=False, capture_output=True, text=True, encoding="utf-8")
    if completed.returncode != 0:
        detail = (completed.stderr or completed.stdout).strip()
        raise RuntimeError(f"{' '.join(command[:4])} failed: {detail}")
    try:
        payload = json.loads(completed.stdout)
    except json.JSONDecodeError as exc:
        raise RuntimeError(f"{' '.join(command[:4])} returned invalid JSON: {exc}") from exc
    if not isinstance(payload, dict):
        raise RuntimeError(f"{' '.join(command[:4])} returned a non-object JSON payload")
    return payload


def process_metadata(size: str, label: str, path: Path) -> dict[str, object]:
    metadata = {"process_stage": label, "source_filename": path.name}
    guide_path = path.with_suffix(".json")
    if label == "Prime context" and guide_path.is_file():
        guide = json.loads(guide_path.read_text(encoding="utf-8"))
        if not isinstance(guide, dict) or guide.get("size_key") != size:
            raise ValueError("Prime context guide JSON must match the registered size")
        role = guide.get("prime_template_source_role")
        if not isinstance(role, str) or not role.strip():
            raise ValueError("Prime context guide JSON requires prime_template_source_role")
        metadata["prime_template_source_role"] = role
        metadata["prime_context"] = guide
    return metadata


def main() -> int:
    args = parse_args()
    if args.revision < 1:
        raise ValueError("revision must be positive")
    images: list[tuple[str, str, Path]] = []
    seen: set[tuple[str, str]] = set()
    for size, label, raw_path in args.image:
        path = Path(raw_path).resolve()
        if label not in ALLOWED_LABELS:
            raise ValueError(f"unsupported process-image label: {label}")
        if size not in {"1080x1080", "1200x628", "800x1000"}:
            raise ValueError(f"unsupported delivery size: {size}")
        if not path.is_file():
            raise ValueError(f"process image does not exist: {path}")
        if (size, label) in seen:
            raise ValueError(f"duplicate process image: {size} {label}")
        seen.add((size, label))
        images.append((size, label, path))

    results = []
    cli_prefix = [args.cli]
    if args.profile.strip():
        cli_prefix.extend(["--profile", args.profile.strip()])
    with tempfile.TemporaryDirectory(prefix="multica-process-assets-") as temporary_directory:
        temporary = Path(temporary_directory)
        for size, label, path in images:
            metadata = process_metadata(size, label, path)
            uploaded = run_json([*cli_prefix, "attachment", "upload", str(path), "--output", "json"])
            attachment_id = str(uploaded.get("id") or "").strip()
            if not attachment_id:
                raise RuntimeError(f"attachment upload did not return an id for {path.name}")
            registration = {
                "variant_id": args.variant_id,
                "task_id": args.task_id.strip(),
                "attachment_id": attachment_id,
                "size_key": size,
                "revision": args.revision,
                "workflow": args.workflow.strip(),
                "label": label,
                "filename": path.name,
                "metadata": metadata,
            }
            registration_path = temporary / f"{size}-{label}.json"
            registration_path.write_text(json.dumps(registration, ensure_ascii=False), encoding="utf-8")
            registered = run_json([
                *cli_prefix, "creative", "order", "diagnostic-asset-put", args.order_id,
                "--input-file", str(registration_path), "--output", "json",
            ])
            results.append({"size_key": size, "label": label, "attachment_id": attachment_id, "diagnostic_asset_id": registered.get("id", "")})
    print(json.dumps({"registered": results}, ensure_ascii=False))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
