#!/usr/bin/env python3
"""Run deterministic checks for a Prime-packaged creative image batch."""

from __future__ import annotations

import argparse
import json
import math
import re
from pathlib import Path
from typing import Any

import cv2
import numpy as np
from PIL import Image, ImageDraw, ImageFont, ImageOps


SIZE_PATTERN = re.compile(r"(?:^|[-_])([1-9]\d*x[1-9]\d*)$", re.IGNORECASE)


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser()
    parser.add_argument("--manifest", required=True)
    parser.add_argument("--compose-result", required=True)
    parser.add_argument("--images-dir", required=True)
    parser.add_argument("--output", required=True)
    parser.add_argument("--contact-sheet", required=True)
    parser.add_argument("--hard-region-sheet", required=True)
    return parser.parse_args()


def decode_qr_image(image: np.ndarray, scale: int = 1) -> str:
    if scale > 1:
        image = cv2.resize(image, None, fx=scale, fy=scale, interpolation=cv2.INTER_NEAREST)
    decoded, _, _ = cv2.QRCodeDetector().detectAndDecode(image)
    return decoded or ""


def qr_hard_region(layout: dict | None) -> list[int] | None:
    if layout is None:
        return None
    for region in layout.get("hard_regions", []):
        if region.get("kind") == "qr":
            return [int(region[key]) for key in ("x1", "y1", "x2", "y2")]
    return None


def decode_qr_evidence(path: Path, layout: dict | None = None, expected_payload: str = "") -> dict[str, Any]:
    image = cv2.imread(str(path), cv2.IMREAD_COLOR)
    if image is None:
        return {"decoded": "", "successful_attempt": None, "attempts": {}}
    attempts = {
        "full_frame_1x": decode_qr_image(image),
        "full_frame_2x_nearest": decode_qr_image(image, scale=2),
    }
    region = qr_hard_region(layout)
    if region is not None:
        left, top, right, bottom = region
        margin = 12
        crop = image[
            max(0, top - margin) : min(image.shape[0], bottom + margin),
            max(0, left - margin) : min(image.shape[1], right + margin),
        ]
        if crop.size:
            attempts["hard_region_crop_1x"] = decode_qr_image(crop)
            attempts["hard_region_crop_2x_nearest"] = decode_qr_image(crop, scale=2)
    successful_attempt = next(
        (name for name, decoded in attempts.items() if decoded and (not expected_payload or decoded == expected_payload)),
        None,
    )
    return {
        "decoded": attempts.get(successful_attempt, "") if successful_attempt else "",
        "successful_attempt": successful_attempt,
        "attempts": attempts,
    }


def decode_qr(path: Path, layout: dict | None = None, expected_payload: str = "") -> str:
    return str(decode_qr_evidence(path, layout, expected_payload)["decoded"])


def edge_white_ratio(image: Image.Image) -> float:
    rgb = np.asarray(image.convert("RGB"))
    edge = np.concatenate((rgb[0:2].reshape(-1, 3), rgb[-2:].reshape(-1, 3), rgb[:, 0:2].reshape(-1, 3), rgb[:, -2:].reshape(-1, 3)))
    near_white = np.all(edge >= 248, axis=1)
    return float(np.mean(near_white)) if len(near_white) else 0.0


def make_contact_sheet(images: list[tuple[str, Path]], output: Path) -> None:
    columns = min(3, max(1, len(images)))
    rows = math.ceil(len(images) / columns)
    cell_width, cell_height, label_height, gap = 420, 340, 34, 16
    sheet = Image.new("RGB", (gap + columns * (cell_width + gap), gap + rows * (cell_height + label_height + gap)), "white")
    draw = ImageDraw.Draw(sheet)
    font = ImageFont.load_default()
    for index, (label, path) in enumerate(images):
        row, column = divmod(index, columns)
        x = gap + column * (cell_width + gap)
        y = gap + row * (cell_height + label_height + gap)
        with Image.open(path) as image:
            preview = ImageOps.contain(image.convert("RGB"), (cell_width, cell_height), Image.Resampling.LANCZOS)
        px = x + (cell_width - preview.width) // 2
        py = y + label_height + (cell_height - preview.height) // 2
        sheet.paste(preview, (px, py))
        draw.text((x, y + 8), label, fill="black", font=font)
    output.parent.mkdir(parents=True, exist_ok=True)
    sheet.save(output, format="PNG", optimize=True)


def valid_layout_contract(layout: object, size: str = "") -> dict | None:
    if not isinstance(layout, dict):
        return None
    regions = layout.get("hard_regions")
    if not isinstance(regions, list):
        return None
    top_end = layout.get("top_key_content_exclusion_end")
    bottom_start = layout.get("bottom_key_content_exclusion_start")
    if not isinstance(top_end, int) or isinstance(top_end, bool):
        return None
    if not isinstance(bottom_start, int) or isinstance(bottom_start, bool):
        return None
    dimensions = None
    if re.fullmatch(r"[1-9]\d*x[1-9]\d*", size, flags=re.IGNORECASE):
        dimensions = tuple(int(value) for value in size.lower().split("x", 1))
        if not 0 <= top_end <= dimensions[1] or not 0 <= bottom_start <= dimensions[1]:
            return None
    seen_ids: set[str] = set()
    for region in regions:
        if not isinstance(region, dict):
            return None
        region_id = str(region.get("id") or "").strip()
        coordinates = [region.get(key) for key in ("x1", "y1", "x2", "y2")]
        if not region_id or region_id in seen_ids or not all(isinstance(value, int) and not isinstance(value, bool) for value in coordinates):
            return None
        left, top, right, bottom = coordinates
        if left < 0 or top < 0 or right <= left or bottom <= top:
            return None
        if dimensions is not None and (right > dimensions[0] or bottom > dimensions[1]):
            return None
        seen_ids.add(region_id)
    return layout


def layout_contract_candidates(manifest: dict, job: dict, compose_item: dict, size: str) -> list[dict]:
    candidates = []
    for candidate in (compose_item.get("layout_contract"), job.get("layout_contract"), job.get("layout")):
        if candidate is not None and valid_layout_contract(candidate, size) is None:
            return []
        layout = valid_layout_contract(candidate, size)
        if layout is not None:
            candidates.append(layout)
    contract = manifest.get("prime_layout_contract") or manifest.get("layout_contract") or {}
    layouts = contract.get("layouts") if isinstance(contract, dict) else None
    raw_published = layouts.get(size) if isinstance(layouts, dict) else None
    if raw_published is not None and valid_layout_contract(raw_published, size) is None:
        return []
    published = valid_layout_contract(raw_published, size)
    if published is not None:
        candidates.append(published)
    return candidates


def resolve_layout_contract(manifest: dict, job: dict, compose_item: dict, size: str) -> dict | None:
    candidates = layout_contract_candidates(manifest, job, compose_item, size)
    if not candidates or any(candidate != candidates[0] for candidate in candidates[1:]):
        return None
    return candidates[0]


def make_hard_region_sheet(images: list[tuple[str, Path, dict]], output: Path) -> None:
    row_width, row_height, gap = 1500, 430, 18
    sheet = Image.new("RGB", (row_width, gap + len(images) * (row_height + gap)), "white")
    draw = ImageDraw.Draw(sheet)
    font = ImageFont.load_default()
    for index, (label, path, layout) in enumerate(images):
        y = gap + index * (row_height + gap)
        with Image.open(path) as source:
            image = source.convert("RGB")
        annotated = image.copy()
        overlay = ImageDraw.Draw(annotated)
        for region in layout.get("hard_regions", []):
            box = tuple(int(region[key]) for key in ("x1", "y1", "x2", "y2"))
            overlay.rectangle(box, outline=(220, 38, 38), width=max(2, image.width // 400))
        full = ImageOps.contain(annotated, (360, 360), Image.Resampling.LANCZOS)
        sheet.paste(full, (12 + (360 - full.width) // 2, y + 44 + (360 - full.height) // 2))
        top_end = max(1, min(image.height, int(layout["top_key_content_exclusion_end"])))
        bottom_start = max(0, min(image.height - 1, int(layout["bottom_key_content_exclusion_start"])))
        strips = [
            ("TOP CONTEXT (SOFT GUIDE, NON-BLOCKING)", image.crop((0, 0, image.width, top_end))),
            ("BOTTOM CONTEXT (SOFT GUIDE, NON-BLOCKING)", image.crop((0, bottom_start, image.width, image.height))),
        ]
        for strip_index, (strip_label, crop) in enumerate(strips):
            target_y = y + 44 + strip_index * 190
            preview = ImageOps.contain(crop, (1080, 150), Image.Resampling.LANCZOS)
            sheet.paste(preview, (400, target_y + 24 + (150 - preview.height) // 2))
            draw.text((400, target_y), strip_label, fill="black", font=font)
        draw.text((12, y + 12), label, fill="black", font=font)
    output.parent.mkdir(parents=True, exist_ok=True)
    sheet.save(output, format="PNG", optimize=True)


def resolve_job_contract(job: dict, compose_item: dict, manifest: dict, images_dir: Path) -> dict:
    output_value = job.get("output_file") or job.get("output") or compose_item.get("output") or ""
    output_name = Path(output_value).name
    image_path = images_dir / output_name
    if not image_path.is_file() and output_value:
        nested = images_dir / output_value
        if nested.is_file():
            image_path = nested

    job_id = str(job.get("id") or "")
    variant = str(
        job.get("variant_key")
        or job.get("variant")
        or compose_item.get("variant_key")
        or manifest.get("variant_key")
        or job_id.split("-", 1)[0]
    ).upper()
    canvas = compose_item.get("canvas") or {}
    size = str(job.get("size") or compose_item.get("size") or "")
    match = SIZE_PATTERN.search(job_id)
    if not size and match:
        size = match.group(1)
    if not size and canvas.get("width") and canvas.get("height"):
        size = f"{canvas['width']}x{canvas['height']}"

    revision = job.get("revision") or compose_item.get("revision") or manifest.get("revision")
    if revision in (None, ""):
        match = re.search(r"_r(\d+)(?:\.[^.]+)?$", output_name, flags=re.IGNORECASE)
        revision = int(match.group(1)) if match else None

    approved_payload = (
        manifest.get("qr_validation", {}).get("approved_payload")
        or manifest.get("approved_payload")
        or job.get("qr_payload")
        or compose_item.get("qr_payload")
        or ""
    )
    composition = job.get("prime_composition") or manifest.get("prime_composition") or {}
    qr_validation = manifest.get("qr_validation") or {}
    qr_mode = str(composition.get("qr_mode") or qr_validation.get("mode") or ("dynamic" if approved_payload else "none"))
    return {
        "image_path": image_path,
        "expected_output_name": output_name,
        "variant": variant,
        "size": size,
        "revision": revision,
        "approved_payload": approved_payload,
        "qr_mode": qr_mode,
    }


def index_compose_results(compose: dict) -> dict[str, dict]:
    return {
        str(item.get("id") or item.get("size_key")): item
        for item in compose.get("results", [])
        if isinstance(item, dict) and (item.get("id") or item.get("size_key"))
    }


def compose_item_succeeded(compose: dict, item: dict) -> bool:
    return item.get("status") == "succeeded" and item.get("passed") is True


def qr_check_passed(qr_mode: str, approved_payload: str, decoded: str, compose_item: dict) -> bool:
    if qr_mode == "none":
        return not approved_payload and not decoded
    return bool(approved_payload) and decoded == approved_payload and compose_item.get("decoded") == approved_payload


def package_contract_failures(manifest: dict, compose: dict, images_dir: Path) -> list[str]:
    failures: list[str] = []
    jobs = manifest.get("jobs")
    compose_results = compose.get("results")
    if not isinstance(jobs, list) or not jobs:
        return ["manifest_jobs_missing"]
    if not isinstance(compose_results, list) or not compose_results:
        return ["compose_results_missing"]

    compose_ids = [str(item.get("id") or item.get("size_key") or "") for item in compose_results if isinstance(item, dict)]
    if len(compose_ids) != len(compose_results) or any(not value for value in compose_ids):
        failures.append("compose_result_id_missing")
    if len(set(compose_ids)) != len(compose_ids):
        failures.append("compose_result_id_duplicate")
    compose_by_id = index_compose_results(compose)
    job_ids: list[str] = []
    resolved_sizes: list[str] = []
    resolved_revisions: list[int] = []
    resolved_variants: list[str] = []
    for job in jobs:
        if not isinstance(job, dict):
            failures.append("manifest_job_invalid")
            continue
        job_id = str(job.get("id") or "")
        job_ids.append(job_id)
        item = compose_by_id.get(job_id, {})
        if not item:
            failures.append(f"{job_id or 'unknown'}:compose_result_missing")
            continue
        contract = resolve_job_contract(job, item, manifest, images_dir)
        size = str(contract["size"])
        revision = contract["revision"]
        variant = str(contract["variant"])
        if not re.fullmatch(r"[1-9]\d*x[1-9]\d*", size, flags=re.IGNORECASE):
            failures.append(f"{job_id}:size_missing")
            continue
        if not isinstance(revision, int) or isinstance(revision, bool) or revision < 1:
            failures.append(f"{job_id}:revision_missing")
        if not variant:
            failures.append(f"{job_id}:variant_missing")
        layout = resolve_layout_contract(manifest, job, item, size)
        if layout is None:
            failures.append(f"{job_id}:layout_contract_missing_or_conflicting")
        canvas = item.get("canvas") or {}
        width, height = (int(value) for value in size.lower().split("x", 1))
        if canvas.get("width") != width or canvas.get("height") != height:
            failures.append(f"{job_id}:compose_canvas_mismatch")
        if not compose_item_succeeded(compose, item):
            failures.append(f"{job_id}:compose_not_passed")
        for field, resolved in (("size", size), ("revision", revision)):
            for source_name, source in (("manifest", manifest), ("job", job), ("compose", item)):
                explicit = source.get(field)
                if explicit not in (None, "") and explicit != resolved:
                    failures.append(f"{job_id}:{source_name}_{field}_mismatch")
        for source_name, source in (("manifest", manifest), ("job", job), ("compose", item)):
            explicit_variant = str(source.get("variant_key") or source.get("variant") or "").upper()
            if explicit_variant and explicit_variant != variant:
                failures.append(f"{job_id}:{source_name}_variant_mismatch")
        resolved_sizes.append(size)
        if isinstance(revision, int):
            resolved_revisions.append(revision)
        resolved_variants.append(variant)

    if len(set(job_ids)) != len(job_ids) or any(not value for value in job_ids):
        failures.append("manifest_job_id_missing_or_duplicate")
    if set(job_ids) != set(compose_ids):
        failures.append("manifest_compose_job_set_mismatch")
    if len(set(resolved_sizes)) != len(resolved_sizes):
        failures.append("resolved_size_duplicate")
    expected_sizes = manifest.get("expected_sizes")
    if isinstance(expected_sizes, list) and set(expected_sizes) != set(resolved_sizes):
        failures.append("expected_sizes_mismatch")
    if len(set(resolved_revisions)) > 1:
        failures.append("mixed_revisions")
    if len(set(resolved_variants)) > 1:
        failures.append("mixed_variants")
    if compose.get("failed") != 0 or compose.get("succeeded") != len(jobs):
        failures.append("compose_summary_not_passed")
    if manifest.get("package_contract_version") is not None:
        if manifest.get("package_contract_version") != 1 or compose.get("package_contract_version") != 1:
            failures.append("package_contract_version_mismatch")
        for field in ("variant_id", "variant_key", "revision", "expected_sizes"):
            if manifest.get(field) != compose.get(field):
                failures.append(f"package_{field}_mismatch")
        if compose.get("package_contract_valid") is not True:
            failures.append("compose_package_contract_invalid")
    return list(dict.fromkeys(failures))


def main() -> int:
    args = parse_args()
    manifest = json.loads(Path(args.manifest).read_text(encoding="utf-8-sig"))
    compose = json.loads(Path(args.compose_result).read_text(encoding="utf-8-sig"))
    images_dir = Path(args.images_dir).resolve()
    contract_failures = package_contract_failures(manifest, compose, images_dir)
    if contract_failures:
        make_contact_sheet([], Path(args.contact_sheet).resolve())
        make_hard_region_sheet([], Path(args.hard_region_sheet).resolve())
        evidence = {
            "candidate_id": manifest.get("candidate_id"),
            "revision": manifest.get("revision"),
            "market_pack": manifest.get("market_pack"),
            "copy_snapshot": manifest.get("copy_snapshot"),
            "package_contract_failures": contract_failures,
            "passed": False,
            "results": [],
        }
        output_path = Path(args.output).resolve()
        output_path.parent.mkdir(parents=True, exist_ok=True)
        output_path.write_text(json.dumps(evidence, ensure_ascii=False, indent=2), encoding="utf-8")
        print(json.dumps({"checked": 0, "passed": False, "output": str(output_path)}, ensure_ascii=False))
        return 1
    compose_by_id = index_compose_results(compose)

    results = []
    contact_images: list[tuple[str, Path]] = []
    hard_region_images: list[tuple[str, Path, dict]] = []
    for job in manifest.get("jobs", []):
        compose_item = compose_by_id.get(job.get("id"), {})
        contract = resolve_job_contract(job, compose_item, manifest, images_dir)
        image_path = contract["image_path"]
        expected_output_name = contract["expected_output_name"]
        variant = contract["variant"]
        size = contract["size"]
        revision = contract["revision"]
        approved_payload = contract["approved_payload"]
        qr_mode = contract["qr_mode"]
        expected_width, expected_height = (int(value) for value in size.lower().split("x", 1))
        layout = resolve_layout_contract(manifest, job, compose_item, size)
        exists = image_path.is_file()
        actual_size = None
        white_ratio = None
        if exists:
            with Image.open(image_path) as image:
                actual_size = list(image.size)
                white_ratio = edge_white_ratio(image)
            contact_images.append((f"{variant} {size}", image_path))
            if layout is not None:
                hard_region_images.append((f"{variant} {size}", image_path, layout))
        qr_evidence = decode_qr_evidence(image_path, layout, approved_payload) if exists else {
            "decoded": "",
            "successful_attempt": None,
            "attempts": {},
        }
        decoded = str(qr_evidence["decoded"])
        checks = {
            "exists": exists,
            "dimensions": actual_size == [expected_width, expected_height],
            "filename": bool(expected_output_name) and image_path.name == expected_output_name,
            "compose": compose_item_succeeded(compose, compose_item),
            "qr": qr_check_passed(qr_mode, approved_payload, decoded, compose_item),
            "full_bleed": white_ratio is not None and white_ratio < 0.5,
            "layout_contract": layout is not None,
        }
        blocking_check_names = ("exists", "dimensions", "filename", "compose", "qr", "layout_contract")
        blocking_checks_passed = all(checks[name] for name in blocking_check_names)
        results.append({
            "id": job["id"],
            "variant": variant,
            "size": size,
            "revision": revision,
            "file": str(image_path),
            "actual_size": actual_size,
            "decoded_qr": decoded,
            "independent_qr_decode": qr_evidence,
            "approved_payload": approved_payload,
            "edge_white_ratio": round(white_ratio, 6) if white_ratio is not None else None,
            "hard_region_review": {
                "top_key_content_exclusion_end": layout.get("top_key_content_exclusion_end") if layout else None,
                "bottom_key_content_exclusion_start": layout.get("bottom_key_content_exclusion_start") if layout else None,
                "soft_guides_non_blocking": True,
                "regions": layout.get("hard_regions", []) if layout else [],
                "semantic_review_required": True,
            },
            "checks": checks,
            "machine_warnings": [] if checks["full_bleed"] else ["edge_white_ratio_needs_visual_review"],
            "passed": blocking_checks_passed,
        })

    make_contact_sheet(contact_images, Path(args.contact_sheet).resolve())
    make_hard_region_sheet(hard_region_images, Path(args.hard_region_sheet).resolve())
    evidence = {
        "candidate_id": manifest.get("candidate_id"),
        "revision": manifest.get("revision"),
        "market_pack": manifest.get("market_pack"),
        "copy_snapshot": manifest.get("copy_snapshot"),
        "approved_payloads": sorted({item["approved_payload"] for item in results if item["approved_payload"]}),
        "compose_summary": {"succeeded": compose.get("succeeded"), "failed": compose.get("failed")},
        "hard_region_sheet": str(Path(args.hard_region_sheet).resolve()),
        "passed": bool(results) and all(item["passed"] for item in results),
        "results": results,
    }
    output_path = Path(args.output).resolve()
    output_path.parent.mkdir(parents=True, exist_ok=True)
    output_path.write_text(json.dumps(evidence, ensure_ascii=False, indent=2), encoding="utf-8")
    print(json.dumps({"checked": len(results), "passed": evidence["passed"], "output": str(output_path)}, ensure_ascii=False))
    return 0 if evidence["passed"] else 1


if __name__ == "__main__":
    raise SystemExit(main())
