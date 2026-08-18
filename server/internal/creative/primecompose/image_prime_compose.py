#!/usr/bin/env python
"""Apply approved full-canvas transparent Prime templates to generated bodies."""

from __future__ import annotations

import argparse
import json
import re
from pathlib import Path
from typing import Any

import cv2
import numpy as np
from PIL import Image


PACKAGE_CONTRACT_VERSION = 6
ENGINE_VERSION = 4
MAX_TEMPLATE_ASPECT_DEVIATION = 0.002
SIZE_PATTERN = re.compile(r"^[1-9]\d*x[1-9]\d*$", re.IGNORECASE)


def decode_qr_image(rgb: np.ndarray, scale: int = 1) -> dict[str, Any]:
    image = cv2.cvtColor(rgb, cv2.COLOR_RGB2BGR)
    if scale > 1:
        image = cv2.resize(image, None, fx=scale, fy=scale, interpolation=cv2.INTER_NEAREST)
    decoded, points, _ = cv2.QRCodeDetector().detectAndDecode(image)
    normalized_points = None
    if points is not None:
        normalized_points = (points.reshape(-1, 2) / scale).round(1).tolist()
    return {"decoded": decoded or "", "detected_points": normalized_points}


def decode_qr_evidence(rgb: np.ndarray) -> dict[str, Any]:
    height, width = rgb.shape[:2]
    regions = {
        "full_frame": (0, 0, width, height),
        "top_right": (width * 55 // 100, 0, width, height * 45 // 100),
        "top_left": (0, 0, width * 45 // 100, height * 45 // 100),
        "bottom_right": (width * 55 // 100, height * 55 // 100, width, height),
        "bottom_left": (0, height * 55 // 100, width * 45 // 100, height),
    }
    attempts: dict[str, dict[str, Any]] = {}
    for region_name, (left, top, right, bottom) in regions.items():
        crop = rgb[top:bottom, left:right]
        if crop.size == 0:
            continue
        for scale, suffix in ((1, "1x"), (2, "2x_nearest")):
            evidence = decode_qr_image(crop, scale=scale)
            if evidence["detected_points"] is not None and region_name != "full_frame":
                evidence["detected_points"] = [
                    [point_x + left, point_y + top] for point_x, point_y in evidence["detected_points"]
                ]
            attempts[f"{region_name}_{suffix}"] = evidence
    successful_attempt = next(
        (name for name, evidence in attempts.items() if evidence["decoded"]),
        None,
    )
    selected = attempts.get(successful_attempt, {"decoded": "", "detected_points": None})
    return {
        "detected": bool(selected["decoded"]),
        "decoded": selected["decoded"],
        "successful_attempt": successful_attempt,
        "detected_points": selected["detected_points"],
        "attempts": attempts,
    }


def require_size(value: object, field: str) -> tuple[int, int]:
    if not isinstance(value, str) or not SIZE_PATTERN.fullmatch(value):
        raise ValueError(f"{field} must be WIDTHxHEIGHT")
    width, height = (int(part) for part in value.lower().split("x", 1))
    return width, height


def require_layout(layout: object, size: str, field: str) -> dict[str, Any]:
    if not isinstance(layout, dict):
        raise ValueError(f"{field} must be an object")
    width, height = require_size(size, field)
    regions = layout.get("hard_regions")
    if not isinstance(regions, list):
        raise ValueError(f"{field}.hard_regions must be an array")
    ids: set[str] = set()
    for index, region in enumerate(regions):
        if not isinstance(region, dict):
            raise ValueError(f"{field}.hard_regions[{index}] must be an object")
        region_id = str(region.get("id") or "").strip()
        values = [region.get(key) for key in ("x1", "y1", "x2", "y2")]
        if not region_id or region_id in ids or not all(isinstance(value, int) for value in values):
            raise ValueError(f"{field}.hard_regions[{index}] is invalid")
        x1, y1, x2, y2 = values
        if x1 < 0 or y1 < 0 or x2 <= x1 or y2 <= y1 or x2 > width or y2 > height:
            raise ValueError(f"{field}.hard_regions[{index}] exceeds {size}")
        ids.add(region_id)
    top_end = layout.get("top_key_content_exclusion_end")
    bottom_start = layout.get("bottom_key_content_exclusion_start")
    if not isinstance(top_end, int) or not isinstance(bottom_start, int) or not 0 <= top_end <= height or not 0 <= bottom_start <= height:
        raise ValueError(f"{field} content boundaries must be within {size}")
    return layout


def require_template_set(manifest: dict[str, Any], expected_sizes: list[str]) -> list[dict[str, Any]]:
    value = manifest.get("prime_template_set")
    if not isinstance(value, dict) or value.get("schema_version") != 2 or value.get("selection_mode") != "automatic_family_contrast":
        raise ValueError("prime_template_set must be schema 2 with automatic_family_contrast")
    families = value.get("families")
    if not isinstance(families, list) or not families:
        raise ValueError("prime_template_set.families must be a non-empty array")
    result: list[dict[str, Any]] = []
    used_roles: set[str] = set()
    family_ids: set[str] = set()
    for index, family in enumerate(families):
        if not isinstance(family, dict):
            raise ValueError(f"prime_template_set.families[{index}] must be an object")
        family_id = str(family.get("id") or "").strip()
        label = str(family.get("label") or "").strip()
        description = str(family.get("description") or "").strip()
        templates = family.get("templates")
        if not re.fullmatch(r"[a-z][a-z0-9_]{0,47}", family_id) or not label or family_id in family_ids or not isinstance(templates, dict):
            raise ValueError(f"prime_template_set.families[{index}] is invalid")
        family_ids.add(family_id)
        normalized: dict[str, dict[str, str]] = {}
        for size in expected_sizes:
            candidate = templates.get(size)
            if not isinstance(candidate, dict):
                raise ValueError(f"prime_template_set.families[{index}].templates.{size} must be an object")
            source_role = str(candidate.get("source_role") or "").strip()
            if not source_role or source_role in used_roles:
                raise ValueError(f"prime_template_set.families[{index}].templates.{size} is invalid")
            used_roles.add(source_role)
            normalized[size] = {"source_role": source_role}
        for raw_size, candidate in templates.items():
            if not isinstance(raw_size, str) or not SIZE_PATTERN.fullmatch(raw_size):
                raise ValueError(f"prime_template_set.families[{index}].templates contains an invalid size")
            if raw_size in expected_sizes:
                continue
            if not isinstance(candidate, dict) or not str(candidate.get("source_role") or "").strip():
                raise ValueError(f"prime_template_set.families[{index}].templates.{raw_size} is invalid")
        result.append({"id": family_id, "label": label, "description": description, "templates": normalized})
    return result


def require_template_validation(manifest: dict[str, Any], expected_sizes: list[str], families: list[dict[str, Any]]) -> None:
    validation = manifest.get("prime_template_set_validation")
    if not isinstance(validation, dict) or validation.get("status") != "passed":
        raise ValueError("prime_template_set_validation must be passed")
    validated_families = validation.get("families")
    if not isinstance(validated_families, list) or len(validated_families) != len(families):
        raise ValueError("prime_template_set_validation.families is incomplete")
    validations_by_id = {
        str(item.get("id") or ""): item
        for item in validated_families if isinstance(item, dict)
    }
    if len(validations_by_id) != len(families):
        raise ValueError("prime_template_set_validation.families has invalid ids")
    for family in families:
        validated = validations_by_id.get(family["id"])
        templates = validated.get("templates") if isinstance(validated, dict) else None
        if not isinstance(templates, dict):
            raise ValueError(f"prime_template_set_validation family {family['id']} is incomplete")
        for size in expected_sizes:
            item = templates.get(size)
            if not isinstance(item, dict) or str(item.get("source_role") or "") != family["templates"][size]["source_role"]:
                raise ValueError(f"prime_template_set_validation family {family['id']} {size} does not match the approved template")


def validate_package_manifest(manifest: object) -> dict[str, Any]:
    if not isinstance(manifest, dict):
        raise ValueError("manifest must be an object")
    if manifest.get("package_contract_version") != PACKAGE_CONTRACT_VERSION:
        raise ValueError(f"package_contract_version must be {PACKAGE_CONTRACT_VERSION}")
    expected_sizes = manifest.get("expected_sizes")
    if not isinstance(expected_sizes, list) or not expected_sizes or len(set(expected_sizes)) != len(expected_sizes):
        raise ValueError("expected_sizes must be a unique non-empty array")
    for size in expected_sizes:
        require_size(size, "expected_sizes")
    families = require_template_set(manifest, expected_sizes)
    require_template_validation(manifest, expected_sizes, families)
    layout_contract = manifest.get("prime_layout_contract")
    if not isinstance(layout_contract, dict) or not isinstance(layout_contract.get("layouts"), dict):
        raise ValueError("prime_layout_contract.layouts must be an object")
    for size in expected_sizes:
        require_layout(layout_contract["layouts"].get(size), size, f"prime_layout_contract.layouts.{size}")
    sources = manifest.get("sources")
    if not isinstance(sources, dict):
        raise ValueError("sources must be an object")
    for family in families:
        for template in family["templates"].values():
            if not isinstance(sources.get(template["source_role"]), str) or not str(sources[template["source_role"]]).strip():
                raise ValueError(f"sources.{template['source_role']} is required")
    jobs = manifest.get("jobs")
    if not isinstance(jobs, list) or len(jobs) != len(expected_sizes):
        raise ValueError("jobs must contain one entry for every expected size")
    seen_sizes: set[str] = set()
    for index, job in enumerate(jobs):
        if not isinstance(job, dict):
            raise ValueError(f"jobs[{index}] must be an object")
        size = job.get("size")
        if size not in expected_sizes or size in seen_sizes:
            raise ValueError(f"jobs[{index}].size is invalid")
        if not all(isinstance(job.get(key), str) and str(job[key]).strip() for key in ("id", "input", "output")):
            raise ValueError(f"jobs[{index}] identity and paths are required")
        seen_sizes.add(size)
    return {"sizes": expected_sizes, "families": families, "sources": sources, "jobs": jobs, "layout_contract": layout_contract}


def load_rgb(path: Path) -> np.ndarray:
    with Image.open(path) as source:
        return np.asarray(source.convert("RGB"))


def visible_template_contrast(body: np.ndarray, template: Image.Image, y_start: int, y_end: int) -> tuple[float, int]:
    rgba = np.asarray(template.convert("RGBA"), dtype=np.float32)
    alpha = rgba[y_start:y_end, :, 3] / 255.0
    visible = alpha > 0.03
    if not np.any(visible):
        return 0.0, 0
    difference = np.linalg.norm(body[y_start:y_end].astype(np.float32) - rgba[y_start:y_end, :, :3], axis=2)
    return float(np.average(difference[visible], weights=alpha[visible])), int(np.count_nonzero(visible))


def key_template_bands(layout: dict[str, Any], height: int) -> list[tuple[str, int, int]]:
    bands = [("header", 0, int(layout["top_key_content_exclusion_end"])), ("footer", int(layout["bottom_key_content_exclusion_start"]), height)]
    return [(name, start, end) for name, start, end in bands if end > start]


def local_template_visibility(body: np.ndarray, template: Image.Image, layout: dict[str, Any]) -> dict[str, Any]:
    rgba = np.asarray(template.convert("RGBA"), dtype=np.float32)
    body_rgb = body.astype(np.float32)
    height, width = body.shape[:2]
    samples: list[dict[str, Any]] = []
    for band_name, start, end in key_template_bands(layout, height):
        alpha = rgba[start:end, :, 3] / 255.0
        for tile_index, tile in enumerate(np.array_split(np.arange(width), 4)):
            if tile.size == 0:
                continue
            left, right = int(tile[0]), int(tile[-1]) + 1
            tile_alpha = alpha[:, left:right]
            visible = tile_alpha > 0.03
            visible_pixels = int(np.count_nonzero(visible))
            if visible_pixels < max(12, int(tile_alpha.size * 0.01)):
                continue
            template_rgb = rgba[start:end, left:right, :3]
            difference = np.linalg.norm(body_rgb[start:end, left:right] - template_rgb, axis=2)
            score = float(np.average(difference[visible], weights=tile_alpha[visible]))
            samples.append({
                "band": band_name,
                "tile": tile_index,
                "contrast_score": round(score, 4),
                "visible_pixels": visible_pixels,
            })
    minimum = min((float(sample["contrast_score"]) for sample in samples), default=0.0)
    blocking = minimum < 24.0
    return {
        "minimum_local_contrast": round(minimum, 4),
        "threshold": 24.0,
        "blocking": blocking,
        "blocking_code": "official_prime_text_unreadable" if blocking else "",
        "samples": samples,
    }


def resize_template_to_canvas(template: Image.Image, canvas: tuple[int, int], candidate_id: str) -> tuple[Image.Image, dict[str, Any]]:
    source_width, source_height = template.size
    target_width, target_height = canvas
    deviation = abs((source_width / source_height) / (target_width / target_height) - 1)
    if deviation > MAX_TEMPLATE_ASPECT_DEVIATION:
        raise ValueError(
            f"template {candidate_id} aspect deviation {deviation:.4%} exceeds {MAX_TEMPLATE_ASPECT_DEVIATION:.4%} "
            f"for canvas {target_width}x{target_height}"
        )
    details = {
        "source_canvas": [source_width, source_height],
        "applied_canvas": [target_width, target_height],
        "aspect_deviation": round(deviation, 8),
        "resize_method": "identity" if template.size == canvas else "lanczos",
    }
    if template.size == canvas:
        return template, details
    return template.resize(canvas, Image.Resampling.LANCZOS), details


def evaluate_template(body: Image.Image, template_spec: dict[str, str], sources: dict[str, Any], root: Path, layout: dict[str, Any]) -> tuple[Image.Image, dict[str, Any]]:
    body_rgb = np.asarray(body.convert("RGB"))
    bands = key_template_bands(layout, body.height)
    source_role = template_spec["source_role"]
    path = root / str(sources[source_role])
    with Image.open(path) as source:
        template = source.convert("RGBA")
    template, resize = resize_template_to_canvas(template, body.size, source_role)
    band_scores = []
    for name, start, end in bands:
        contrast, visible_pixels = visible_template_contrast(body_rgb, template, start, end)
        if visible_pixels:
            band_scores.append({"name": name, "contrast_score": round(contrast, 4), "visible_pixels": visible_pixels})
    if not band_scores:
        raise ValueError(f"template {source_role} has no visible pixels in the key Prime bands")
    overall_contrast, visible_pixels = visible_template_contrast(body_rgb, template, 0, body.height)
    minimum_band_contrast = min(item["contrast_score"] for item in band_scores)
    average_band_contrast = sum(item["contrast_score"] for item in band_scores) / len(band_scores)
    return template, {
        "selected_source_role": source_role,
        "visible_pixels": visible_pixels,
        "minimum_band_contrast": round(minimum_band_contrast, 4),
        "average_band_contrast": round(average_band_contrast, 4),
        "overall_contrast": round(overall_contrast, 4),
        "resize": resize,
        "bands": band_scores,
    }


def select_template_family(prepared_jobs: list[dict[str, Any]], package: dict[str, Any], root: Path) -> tuple[dict[str, Any], dict[str, tuple[Image.Image, dict[str, Any]]], dict[str, Any]]:
    scored: list[tuple[dict[str, Any], dict[str, tuple[Image.Image, dict[str, Any]]], float, float, float]] = []
    for family in package["families"]:
        evaluated: dict[str, tuple[Image.Image, dict[str, Any]]] = {}
        minimum_scores: list[float] = []
        average_scores: list[float] = []
        overall_scores: list[float] = []
        for job in prepared_jobs:
            size = job["size"]
            template, evidence = evaluate_template(job["body"], family["templates"][size], package["sources"], root, package["layout_contract"]["layouts"][size])
            evaluated[size] = (template, evidence)
            minimum_scores.append(float(evidence["minimum_band_contrast"]))
            average_scores.append(float(evidence["average_band_contrast"]))
            overall_scores.append(float(evidence["overall_contrast"]))
        scored.append((family, evaluated, min(minimum_scores), sum(average_scores) / len(average_scores), sum(overall_scores) / len(overall_scores)))
    selected, evaluated, minimum, average, overall = max(scored, key=lambda item: (item[2], item[3], item[4]))
    return selected, evaluated, {
        "selection_mode": "automatic_family_contrast",
        "selection_reason": "highest_minimum_key_band_contrast_across_all_delivery_sizes",
        "selected_family_id": selected["id"],
        "selected_family_label": selected["label"],
        "minimum_band_contrast": round(minimum, 4),
        "average_band_contrast": round(average, 4),
        "overall_contrast": round(overall, 4),
        "candidates": [
            {
                "family_id": family["id"],
                "family_label": family["label"],
                "minimum_band_contrast": round(minimum_score, 4),
                "average_band_contrast": round(average_score, 4),
                "overall_contrast": round(overall_score, 4),
                "sizes": {
                    size: {
                        "source_role": evidence["selected_source_role"],
                        "minimum_band_contrast": evidence["minimum_band_contrast"],
                        "average_band_contrast": evidence["average_band_contrast"],
                        "overall_contrast": evidence["overall_contrast"],
                    }
                    for size, (_, evidence) in evaluated_family.items()
                },
            }
            for family, evaluated_family, minimum_score, average_score, overall_score in scored
        ],
    }


def compose_job(job: dict[str, Any], body: Image.Image, family: dict[str, Any], template: Image.Image, template_evidence: dict[str, Any], family_selection: dict[str, Any], layout: dict[str, Any], root: Path) -> dict[str, Any]:
    size = job["size"]
    input_path = root / str(job["input"])
    output_path = root / str(job["output"])
    final = Image.alpha_composite(body, template)
    output_path.parent.mkdir(parents=True, exist_ok=True)
    final.save(output_path, "PNG")
    visibility_audit = local_template_visibility(np.asarray(body.convert("RGB")), template, layout)
    qr = decode_qr_evidence(np.asarray(final.convert("RGB")))
    qr["validation_basis"] = "optional_template_qr_decode"
    return {
        "id": job["id"], "size": size, "input": str(input_path), "output": str(output_path), "canvas": {"width": body.width, "height": body.height},
        "template_selection": {**family_selection, "selected_source_role": template_evidence["selected_source_role"], "template": template_evidence},
        "template": {"family_id": family["id"], "family_label": family["label"], "source_role": template_evidence["selected_source_role"], "mode": "full_canvas_alpha_composite", "resize": template_evidence["resize"]},
        "visibility_audit": visibility_audit,
        "qr": qr,
        "compose": {"engine_version": ENGINE_VERSION, "mode": "full_transparent_template", "template_application": "unchanged_full_canvas_alpha_composite", "output_size": [body.width, body.height]},
    }


def compose_manifest(manifest_path: Path) -> dict[str, Any]:
    with manifest_path.open("r", encoding="utf-8") as source:
        manifest = json.load(source)
    package = validate_package_manifest(manifest)
    prepared_jobs, failures = [], []
    for job in package["jobs"]:
        try:
            size = job["size"]
            expected_size = require_size(size, f"job {job['id']}.size")
            input_path = manifest_path.parent / str(job["input"])
            with Image.open(input_path) as source:
                body = source.convert("RGBA")
            if body.size != expected_size:
                raise ValueError(f"job {job['id']} body dimensions {body.width}x{body.height} do not match {size}")
            prepared_jobs.append({**job, "body": body})
        except Exception as error:
            failures.append({"id": job.get("id", ""), "size": job.get("size", ""), "error": str(error)})
    results = []
    if not failures:
        try:
            family, evaluated, family_selection = select_template_family(prepared_jobs, package, manifest_path.parent)
            for job in prepared_jobs:
                template, evidence = evaluated[job["size"]]
                results.append(compose_job(job, job["body"], family, template, evidence, family_selection, package["layout_contract"]["layouts"][job["size"]], manifest_path.parent))
        except Exception as error:
            failures.extend({"id": job.get("id", ""), "size": job.get("size", ""), "error": str(error)} for job in prepared_jobs)
    return {
        "engine_version": ENGINE_VERSION,
        "package_contract_version": PACKAGE_CONTRACT_VERSION,
        "package_contract_valid": True,
        "variant_id": manifest.get("variant_id"),
        "variant_key": manifest.get("variant_key"),
        "revision": manifest.get("revision"),
        "expected_sizes": package["sizes"],
        "succeeded": len(results),
        "failed": len(failures),
        "results": results,
        "failures": failures,
    }


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--manifest", required=True)
    parser.add_argument("--engine-sha256", default="")
    args = parser.parse_args()
    try:
        report = compose_manifest(Path(args.manifest).resolve())
    except Exception as error:
        report = {"engine_version": ENGINE_VERSION, "package_contract_version": PACKAGE_CONTRACT_VERSION, "package_contract_valid": False, "error": str(error), "succeeded": 0, "failed": 0, "results": [], "failures": []}
        print(json.dumps(report, ensure_ascii=False))
        return 1
    report["engine_sha256"] = args.engine_sha256
    print(json.dumps(report, ensure_ascii=False))
    return 0 if report["failed"] == 0 else 1


if __name__ == "__main__":
    raise SystemExit(main())
