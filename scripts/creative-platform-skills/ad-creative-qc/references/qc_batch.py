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
PACKAGE_CONTRACT_VERSION = 6


def decode_qr_image(image: np.ndarray, scale: int = 1) -> str:
    if scale > 1:
        image = cv2.resize(image, None, fx=scale, fy=scale, interpolation=cv2.INTER_NEAREST)
    decoded, _, _ = cv2.QRCodeDetector().detectAndDecode(image)
    return decoded or ""


def decode_qr_evidence(path: Path, layout: dict | None = None) -> dict[str, Any]:
    del layout
    image = cv2.imread(str(path), cv2.IMREAD_COLOR)
    if image is None:
        return {"detected": False, "decoded": "", "successful_attempt": None, "attempts": {}}
    height, width = image.shape[:2]
    regions = {
        "full_frame": (0, 0, width, height),
        "top_right": (width * 55 // 100, 0, width, height * 45 // 100),
        "top_left": (0, 0, width * 45 // 100, height * 45 // 100),
        "bottom_right": (width * 55 // 100, height * 55 // 100, width, height),
        "bottom_left": (0, height * 55 // 100, width * 45 // 100, height),
    }
    attempts: dict[str, str] = {}
    for region_name, (left, top, right, bottom) in regions.items():
        crop = image[top:bottom, left:right]
        if crop.size == 0:
            continue
        attempts[f"{region_name}_1x"] = decode_qr_image(crop)
        attempts[f"{region_name}_2x_nearest"] = decode_qr_image(crop, scale=2)
    successful_attempt = next((name for name, decoded in attempts.items() if decoded), None)
    return {
        "detected": successful_attempt is not None,
        "decoded": attempts.get(successful_attempt, "") if successful_attempt else "",
        "successful_attempt": successful_attempt,
        "attempts": attempts,
    }


def decode_qr(path: Path, layout: dict | None = None) -> str:
    return str(decode_qr_evidence(path, layout)["decoded"])


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser()
    parser.add_argument("--manifest", required=True)
    parser.add_argument("--compose-result", required=True)
    parser.add_argument("--images-dir", required=True)
    parser.add_argument("--output", required=True)
    parser.add_argument("--contact-sheet", required=True)
    return parser.parse_args()


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


def resolve_layout_contract(manifest: dict, job: dict, compose_item: dict, size: str) -> dict | None:
    contract = manifest.get("prime_layout_contract")
    if not isinstance(contract, dict):
        return None
    layouts = contract.get("layouts")
    if not isinstance(layouts, dict):
        return None
    return valid_layout_contract(layouts.get(size), size)


def resolve_job_contract(job: dict, compose_item: dict, manifest: dict, images_dir: Path) -> dict:
    output_value = str(job.get("output") or "")
    output_name = Path(output_value).name
    image_path = images_dir / output_name
    variant = str(manifest.get("variant_key") or "").upper()
    size = str(job.get("size") or "")
    revision = manifest.get("revision")
    return {
        "image_path": image_path,
        "expected_output_name": output_name,
        "variant": variant,
        "size": size,
        "revision": revision,
    }


def index_compose_results(compose: dict) -> dict[str, dict]:
    return {
        str(item.get("id") or ""): item
        for item in compose.get("results", [])
        if isinstance(item, dict) and item.get("id")
    }


def compose_item_succeeded(compose: dict, item: dict) -> bool:
    compose_evidence = item.get("compose")
    return (
        bool(item.get("id"))
        and isinstance(item.get("template_selection"), dict)
        and isinstance(compose_evidence, dict)
        and compose_evidence.get("mode") == "full_transparent_template"
        and compose_evidence.get("template_application") == "unchanged_full_canvas_alpha_composite"
    )


def package_contract_failures(manifest: dict, compose: dict, images_dir: Path) -> list[str]:
    failures: list[str] = []
    jobs = manifest.get("jobs")
    compose_results = compose.get("results")
    if not isinstance(jobs, list) or not jobs:
        return ["manifest_jobs_missing"]
    if not isinstance(compose_results, list) or not compose_results:
        return ["compose_results_missing"]

    compose_ids = [str(item.get("id") or "") for item in compose_results if isinstance(item, dict)]
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
    if manifest.get("package_contract_version") != PACKAGE_CONTRACT_VERSION or compose.get("package_contract_version") != PACKAGE_CONTRACT_VERSION:
        failures.append("package_contract_version_mismatch")
    for field in ("variant_id", "variant_key", "revision", "expected_sizes"):
        if manifest.get(field) != compose.get(field):
            failures.append(f"package_{field}_mismatch")
    if compose.get("package_contract_valid") is not True:
        failures.append("compose_package_contract_invalid")
    for item in compose_results:
        if not isinstance(item, dict):
            continue
        selection = item.get("template_selection")
        if not isinstance(selection, dict) or selection.get("selection_mode") != "automatic_family_contrast":
            failures.append(f"{item.get('id') or 'unknown'}:template_selection_missing_or_manual")
            continue
        size = str(item.get("size") or "")
        template_set = manifest.get("prime_template_set")
        families = template_set.get("families") if isinstance(template_set, dict) else None
        if not isinstance(families, list) or not families:
            failures.append(f"{item.get('id') or 'unknown'}:template_set_invalid")
            continue
        expected_families: dict[str, str] = {}
        for family in families:
            if not isinstance(family, dict):
                continue
            family_id = str(family.get("id") or "")
            templates = family.get("templates")
            template = templates.get(size) if isinstance(templates, dict) else None
            source_role = str(template.get("source_role") or "") if isinstance(template, dict) else ""
            if family_id and source_role:
                expected_families[family_id] = source_role
        candidates = selection.get("candidates")
        actual_families: dict[str, str] = {}
        if isinstance(candidates, list):
            for candidate in candidates:
                if not isinstance(candidate, dict):
                    continue
                family_id = str(candidate.get("family_id") or "")
                sizes = candidate.get("sizes")
                size_evidence = sizes.get(size) if isinstance(sizes, dict) else None
                source_role = str(size_evidence.get("source_role") or "") if isinstance(size_evidence, dict) else ""
                if family_id and source_role:
                    actual_families[family_id] = source_role
        selected_family = str(selection.get("selected_family_id") or "")
        selected_source_role = str(selection.get("selected_source_role") or "")
        if actual_families != expected_families or expected_families.get(selected_family) != selected_source_role:
            failures.append(f"{item.get('id') or 'unknown'}:template_selection_invalid")
    return list(dict.fromkeys(failures))


def main() -> int:
    args = parse_args()
    manifest = json.loads(Path(args.manifest).read_text(encoding="utf-8-sig"))
    compose = json.loads(Path(args.compose_result).read_text(encoding="utf-8-sig"))
    images_dir = Path(args.images_dir).resolve()
    contract_failures = package_contract_failures(manifest, compose, images_dir)
    if contract_failures:
        make_contact_sheet([], Path(args.contact_sheet).resolve())
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
    for job in manifest.get("jobs", []):
        compose_item = compose_by_id.get(job.get("id"), {})
        contract = resolve_job_contract(job, compose_item, manifest, images_dir)
        image_path = contract["image_path"]
        expected_output_name = contract["expected_output_name"]
        variant = contract["variant"]
        size = contract["size"]
        revision = contract["revision"]
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
        qr_evidence = decode_qr_evidence(image_path, layout) if exists else {
            "detected": False,
            "decoded": "",
            "successful_attempt": None,
            "attempts": {},
        }
        visibility_audit = compose_item.get("visibility_audit") if isinstance(compose_item.get("visibility_audit"), dict) else {}
        visibility_blocking = bool(visibility_audit.get("blocking"))
        checks = {
            "exists": exists,
            "dimensions": actual_size == [expected_width, expected_height],
            "filename": bool(expected_output_name) and image_path.name == expected_output_name,
            "compose": compose_item_succeeded(compose, compose_item),
            "full_bleed": white_ratio is not None and white_ratio < 0.5,
            "layout_contract": layout is not None,
            "prime_visibility_audit": not visibility_blocking,
        }
        blocking_check_names = ("exists", "dimensions", "filename", "compose", "layout_contract")
        blocking_checks_passed = all(checks[name] for name in blocking_check_names)
        results.append({
            "id": job["id"],
            "variant": variant,
            "size": size,
            "revision": revision,
            "file": str(image_path),
            "actual_size": actual_size,
            "edge_white_ratio": round(white_ratio, 6) if white_ratio is not None else None,
            "decoded_qr": qr_evidence["decoded"],
            "optional_qr_decode": qr_evidence,
            "checks": checks,
            "prime_visibility_audit": visibility_audit,
            "machine_warnings": (["edge_white_ratio_needs_visual_review"] if not checks["full_bleed"] else []) + (["prime_visibility_needs_visual_confirmation"] if visibility_blocking else []),
            "passed": blocking_checks_passed,
        })

    make_contact_sheet(contact_images, Path(args.contact_sheet).resolve())
    evidence = {
        "candidate_id": manifest.get("candidate_id"),
        "revision": manifest.get("revision"),
        "market_pack": manifest.get("market_pack"),
        "copy_snapshot": manifest.get("copy_snapshot"),
        "compose_summary": {"succeeded": compose.get("succeeded"), "failed": compose.get("failed")},
        "contact_sheet": str(Path(args.contact_sheet).resolve()),
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
