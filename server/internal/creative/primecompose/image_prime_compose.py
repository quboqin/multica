#!/usr/bin/env python
"""Apply approved full-canvas transparent Prime templates to generated bodies."""

from __future__ import annotations

import argparse
import json
import re
from pathlib import Path
from typing import Any

import numpy as np
from PIL import Image


PACKAGE_CONTRACT_VERSION = 6
ENGINE_VERSION = 8
MAX_TEMPLATE_ASPECT_DEVIATION = 0.002
SIZE_PATTERN = re.compile(r"^[1-9]\d*x[1-9]\d*$", re.IGNORECASE)
MINIMUM_TEMPLATE_READABILITY_CONTRAST = 24.0
MINIMUM_RELATIVE_LUMINANCE_CONTRAST = 3.0
MINIMUM_BACKGROUND_POLARITY_MATCH = 0.90
MAX_BACKGROUND_TEXTURE_P90 = 0.18
MINIMUM_COMPONENT_ALPHA = 0.35
MAX_BRIGHT_OPAQUE_TILE_COVERAGE = 0.45
BRIGHT_TEMPLATE_LUMINANCE = 235.0


def skipped_qr_evidence() -> dict[str, Any]:
    return {
        "detected": False,
        "decoded": "",
        "successful_attempt": None,
        "detected_points": None,
        "attempts": {},
        "skipped": True,
        "reason": "qr_decode_disabled",
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
        role = job.get("prime_template_source_role", "")
        if role and (not isinstance(role, str) or not any(f["templates"][size]["source_role"] == role for f in families)):
            raise ValueError(f"jobs[{index}].prime_template_source_role is not approved for {size}")
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


def relative_luminance(rgb: np.ndarray) -> np.ndarray:
    srgb = np.clip(rgb.astype(np.float32) / 255.0, 0.0, 1.0)
    linear = np.where(srgb <= 0.04045, srgb / 12.92, ((srgb + 0.055) / 1.055) ** 2.4)
    return 0.2126 * linear[:, :, 0] + 0.7152 * linear[:, :, 1] + 0.0722 * linear[:, :, 2]


def weighted_percentile(values: np.ndarray, weights: np.ndarray, percentile: float) -> float:
    if values.size == 0:
        return 0.0
    order = np.argsort(values)
    ordered_values = values[order]
    ordered_weights = weights[order]
    total = float(np.sum(ordered_weights))
    if total <= 0.0:
        return float(np.percentile(ordered_values, percentile))
    cumulative = np.cumsum(ordered_weights)
    index = int(np.searchsorted(cumulative, total * percentile / 100.0, side="left"))
    return float(ordered_values[min(index, ordered_values.size - 1)])


def key_template_bands(layout: dict[str, Any], height: int) -> list[tuple[str, int, int]]:
    bands = [("header", 0, int(layout["top_key_content_exclusion_end"])), ("footer", int(layout["bottom_key_content_exclusion_start"]), height)]
    return [(name, start, end) for name, start, end in bands if end > start]


def template_component_mask(template: Image.Image, layout: dict[str, Any]) -> tuple[np.ndarray, dict[str, Any]]:
    rgba = np.asarray(template.convert("RGBA"), dtype=np.float32)
    height, width = rgba.shape[:2]
    alpha = rgba[:, :, 3] / 255.0
    band_mask = np.zeros((height, width), dtype=bool)
    for _, start, end in key_template_bands(layout, height):
        band_mask[start:end, :] = True
    visible = (alpha >= MINIMUM_COMPONENT_ALPHA) & band_mask
    if not np.any(visible):
        return visible, {
            "source": "template_alpha_and_dominant_foreground_polarity",
            "alpha_threshold": MINIMUM_COMPONENT_ALPHA,
            "visible_pixels": 0,
            "coverage": 0.0,
            "foreground_polarity": "unknown",
            "foreground_relative_luminance_median": 0.0,
        }
    luminance = relative_luminance(rgba[:, :, :3])
    median = weighted_percentile(luminance[visible], alpha[visible], 50.0)
    polarity = "light" if median >= 0.5 else "dark"
    if polarity == "light":
        component = visible & (luminance >= max(0.5, median - 0.25))
    else:
        component = visible & (luminance <= min(0.5, median + 0.25))
    component_pixels = int(np.count_nonzero(component))
    return component, {
        "source": "template_alpha_and_dominant_foreground_polarity",
        "alpha_threshold": MINIMUM_COMPONENT_ALPHA,
        "visible_pixels": component_pixels,
        "coverage": round(component_pixels / float(height * width), 8),
        "all_visible_template_pixels": int(np.count_nonzero(visible)),
        "foreground_polarity": polarity,
        "foreground_relative_luminance_median": round(
            weighted_percentile(luminance[component], alpha[component], 50.0), 6
        ) if component_pixels else 0.0,
    }


def background_texture_map(luminance: np.ndarray) -> np.ndarray:
    horizontal = np.zeros_like(luminance)
    vertical = np.zeros_like(luminance)
    horizontal[:, 1:] = np.abs(luminance[:, 1:] - luminance[:, :-1])
    vertical[1:, :] = np.abs(luminance[1:, :] - luminance[:-1, :])
    return np.maximum(horizontal, vertical)


def classify_background_polarity(luminance: float) -> str:
    if luminance <= 0.35:
        return "dark"
    if luminance >= 0.65:
        return "light"
    return "midtone"


def analyze_visible_component_support(body: np.ndarray, template: Image.Image, layout: dict[str, Any]) -> dict[str, Any]:
    rgba = np.asarray(template.convert("RGBA"), dtype=np.float32)
    alpha = rgba[:, :, 3] / 255.0
    body_luminance = relative_luminance(body)
    composited_rgb = rgba[:, :, :3] * alpha[:, :, None] + body.astype(np.float32) * (1.0 - alpha[:, :, None])
    foreground_luminance = relative_luminance(composited_rgb)
    texture = background_texture_map(body_luminance)
    component_mask, mask_evidence = template_component_mask(template, layout)
    foreground_polarity = mask_evidence["foreground_polarity"]
    if foreground_polarity == "light":
        support_requirement = "dark_low_texture"
    elif foreground_polarity == "dark":
        support_requirement = "light_low_texture"
    else:
        support_requirement = "unknown"
    height, width = body.shape[:2]
    samples: list[dict[str, Any]] = []
    total_component_pixels = int(np.count_nonzero(component_mask))
    minimum_sample_pixels = max(1, int(total_component_pixels * 0.005))
    for band_name, start, end in key_template_bands(layout, height):
        for tile_index, tile in enumerate(np.array_split(np.arange(width), 4)):
            if tile.size == 0:
                continue
            left, right = int(tile[0]), int(tile[-1]) + 1
            local_mask = component_mask[start:end, left:right]
            visible_pixels = int(np.count_nonzero(local_mask))
            if visible_pixels < minimum_sample_pixels:
                continue
            local_weights = alpha[start:end, left:right][local_mask]
            local_foreground = foreground_luminance[start:end, left:right][local_mask]
            local_background = body_luminance[start:end, left:right][local_mask]
            local_contrast = (np.maximum(local_foreground, local_background) + 0.05) / (
                np.minimum(local_foreground, local_background) + 0.05
            )
            if foreground_polarity == "light":
                polarity_match = local_foreground > local_background
            else:
                polarity_match = local_foreground < local_background
            polarity_match_ratio = float(np.average(polarity_match.astype(np.float32), weights=local_weights))
            background_median = weighted_percentile(local_background, local_weights, 50.0)
            local_texture = texture[start:end, left:right][local_mask]
            samples.append({
                "band": band_name,
                "tile": tile_index,
                "visible_pixels": visible_pixels,
                "relative_luminance_contrast_p10": round(weighted_percentile(local_contrast, local_weights, 10.0), 4),
                "relative_luminance_contrast_median": round(weighted_percentile(local_contrast, local_weights, 50.0), 4),
                "polarity_match_ratio": round(polarity_match_ratio, 4),
                "background_polarity": classify_background_polarity(background_median),
                "background_relative_luminance_median": round(background_median, 6),
                "background_texture_p90": round(weighted_percentile(local_texture, local_weights, 90.0), 6),
            })
    minimum_contrast = min((float(sample["relative_luminance_contrast_p10"]) for sample in samples), default=0.0)
    minimum_polarity_match = min((float(sample["polarity_match_ratio"]) for sample in samples), default=0.0)
    maximum_texture = max((float(sample["background_texture_p90"]) for sample in samples), default=1.0)
    if total_component_pixels:
        weights = alpha[component_mask]
        background_median = weighted_percentile(body_luminance[component_mask], weights, 50.0)
    else:
        background_median = 0.0
    inadequacy_codes: list[str] = []
    if not samples:
        inadequacy_codes.append("prime_visible_component_mask_missing")
    if minimum_contrast < MINIMUM_RELATIVE_LUMINANCE_CONTRAST:
        inadequacy_codes.extend(["prime_relative_luminance_contrast_below_threshold", "prime_template_inconspicuous"])
    if minimum_polarity_match < MINIMUM_BACKGROUND_POLARITY_MATCH:
        inadequacy_codes.append("prime_background_polarity_mismatch")
    if maximum_texture > MAX_BACKGROUND_TEXTURE_P90:
        inadequacy_codes.append("prime_background_too_textured")
    return {
        "foreground_polarity": foreground_polarity,
        "support_requirement": support_requirement,
        "visible_component_mask": mask_evidence,
        "background_support": {
            "polarity": classify_background_polarity(background_median),
            "relative_luminance_contrast": {
                "minimum_local_p10": round(minimum_contrast, 4),
                "threshold": MINIMUM_RELATIVE_LUMINANCE_CONTRAST,
                "basis": "alpha_composited_template_over_generated_body",
            },
            "polarity_match": {
                "minimum_local_ratio": round(minimum_polarity_match, 4),
                "threshold": MINIMUM_BACKGROUND_POLARITY_MATCH,
            },
            "texture": {
                "maximum_local_p90": round(maximum_texture, 6),
                "threshold": MAX_BACKGROUND_TEXTURE_P90,
                "metric": "relative_luminance_neighbor_difference",
            },
        },
        "blocking": bool(inadequacy_codes),
        "blocking_code": inadequacy_codes[0] if inadequacy_codes else "",
        "visual_adequacy": {
            "adequate": not inadequacy_codes,
            "inadequacy_codes": list(dict.fromkeys(inadequacy_codes)),
        },
        "samples": samples,
    }


def local_template_visibility(body: np.ndarray, template: Image.Image, layout: dict[str, Any]) -> dict[str, Any]:
    audit = analyze_visible_component_support(body, template, layout)
    audit["minimum_local_contrast"] = audit["background_support"]["relative_luminance_contrast"]["minimum_local_p10"]
    audit["threshold"] = MINIMUM_RELATIVE_LUMINANCE_CONTRAST
    return audit


def bright_opaque_tile_coverage(template: Image.Image, layout: dict[str, Any]) -> dict[str, Any]:
    """Measure bright, mostly opaque template areas that read as a solid patch."""
    rgba = np.asarray(template.convert("RGBA"), dtype=np.float32)
    height, width = rgba.shape[:2]
    luminance = 0.2126 * rgba[:, :, 0] + 0.7152 * rgba[:, :, 1] + 0.0722 * rgba[:, :, 2]
    mask = (rgba[:, :, 3] >= 230.0) & (luminance >= BRIGHT_TEMPLATE_LUMINANCE)
    samples: list[dict[str, Any]] = []
    for band_name, start, end in key_template_bands(layout, height):
        for tile_index, tile in enumerate(np.array_split(np.arange(width), 4)):
            if tile.size == 0:
                continue
            left, right = int(tile[0]), int(tile[-1]) + 1
            coverage = float(np.mean(mask[start:end, left:right]))
            samples.append({"band": band_name, "tile": tile_index, "bright_opaque_coverage": round(coverage, 4)})
    maximum = max((float(sample["bright_opaque_coverage"]) for sample in samples), default=0.0)
    return {
        "maximum_bright_opaque_tile_coverage": round(maximum, 4),
        "threshold": MAX_BRIGHT_OPAQUE_TILE_COVERAGE,
        "dominant_bright_patch": maximum >= MAX_BRIGHT_OPAQUE_TILE_COVERAGE,
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
    bright_patch = bright_opaque_tile_coverage(template, layout)
    visibility_audit = analyze_visible_component_support(body_rgb, template, layout)
    inadequacy_codes = list(visibility_audit["visual_adequacy"]["inadequacy_codes"])
    if bright_patch["dominant_bright_patch"]:
        inadequacy_codes.append("prime_template_dominant_bright_patch")
    return template, {
        "selected_source_role": source_role,
        "visible_pixels": visible_pixels,
        "minimum_band_contrast": round(minimum_band_contrast, 4),
        "average_band_contrast": round(average_band_contrast, 4),
        "overall_contrast": round(overall_contrast, 4),
        "resize": resize,
        "bands": band_scores,
        "bright_patch": bright_patch,
        "foreground_polarity": visibility_audit["foreground_polarity"],
        "support_requirement": visibility_audit["support_requirement"],
        "visible_component_mask": visibility_audit["visible_component_mask"],
        "background_support": visibility_audit["background_support"],
        "visibility_audit": visibility_audit,
        "visual_adequacy": {
            "adequate": not inadequacy_codes,
            "inadequacy_codes": list(dict.fromkeys(inadequacy_codes)),
        },
    }


def candidate_selection_evidence(family: dict[str, Any], size: str, evidence: dict[str, Any]) -> dict[str, Any]:
    details = {
        "source_role": evidence["selected_source_role"],
        "minimum_band_contrast": evidence["minimum_band_contrast"],
        "average_band_contrast": evidence["average_band_contrast"],
        "overall_contrast": evidence["overall_contrast"],
        "foreground_polarity": evidence["foreground_polarity"],
        "support_requirement": evidence["support_requirement"],
        "visible_component_mask": evidence["visible_component_mask"],
        "background_support": evidence["background_support"],
        "bright_patch": evidence["bright_patch"],
        "visual_adequacy": evidence["visual_adequacy"],
    }
    return {
        "family_id": family["id"],
        "family_label": family["label"],
        **details,
        "sizes": {size: details},
    }


def select_template_for_size(job: dict[str, Any], package: dict[str, Any], root: Path) -> tuple[dict[str, Any] | None, Image.Image | None, dict[str, Any] | None, dict[str, Any]]:
    size = job["size"]
    scored: list[tuple[dict[str, Any], Image.Image, dict[str, Any]]] = []
    candidate_failures: list[dict[str, Any]] = []
    for family in package["families"]:
        try:
            template, evidence = evaluate_template(job["body"], family["templates"][size], package["sources"], root, package["layout_contract"]["layouts"][size])
            scored.append((family, template, evidence))
        except Exception as error:
            candidate_failures.append({
                "family_id": family["id"],
                "family_label": family["label"],
                "source_role": family["templates"][size]["source_role"],
                "visual_adequacy": {"adequate": False, "inadequacy_codes": ["prime_template_evaluation_failed"]},
                "error": str(error),
            })
    rank = lambda item: (
        float(item[2]["background_support"]["relative_luminance_contrast"]["minimum_local_p10"]),
        float(item[2]["background_support"]["polarity_match"]["minimum_local_ratio"]),
        -float(item[2]["background_support"]["texture"]["maximum_local_p90"]),
        float(item[2]["average_band_contrast"]),
    )
    pinned_role = job.get("prime_template_source_role", "")
    if pinned_role:
        pinned = [item for item in scored if item[0]["templates"][size]["source_role"] == pinned_role]
        if not pinned:
            raise ValueError(f"Prime context template {pinned_role} could not be evaluated")
        selected, template, evidence = pinned[0]
        adequate = evidence["visual_adequacy"]["adequate"]
        return selected, template, evidence, {
            "size": size, "selection_scope": "delivery_size", "selection_mode": "frozen_prime_context",
            "selection_reason": "same_template_as_model_context", "selected_family_id": selected["id"],
            "selected_family_label": selected["label"], "selected_source_role": pinned_role,
            "visual_adequacy": {**evidence["visual_adequacy"], "status": "passed" if adequate else "qc_risk",
                                "reselected_without_regenerating_base": False},
            "candidates": [candidate_selection_evidence(family, size, e) for family, _, e in scored] + candidate_failures,
        }
    contrast_selected = max(scored, key=rank) if scored else None
    visually_adequate = [item for item in scored if item[2]["visual_adequacy"]["adequate"]]
    candidates = [candidate_selection_evidence(family, size, evidence) for family, _, evidence in scored] + candidate_failures
    if visually_adequate:
        selected, template, evidence = max(visually_adequate, key=rank)
        reselected = contrast_selected is not None and selected["id"] != contrast_selected[0]["id"]
        selection_reason = "visual_adequacy_reselected_from_highest_contrast_family" if reselected else "highest_adequate_visible_component_contrast_for_size"
        return selected, template, evidence, {
            "size": size,
            "selection_scope": "delivery_size",
            "selection_mode": "automatic_family_contrast",
            "selection_reason": selection_reason,
            "selected_family_id": selected["id"],
            "selected_family_label": selected["label"],
            "selected_source_role": evidence["selected_source_role"],
            "minimum_band_contrast": evidence["minimum_band_contrast"],
            "average_band_contrast": evidence["average_band_contrast"],
            "overall_contrast": evidence["overall_contrast"],
            "minimum_relative_luminance_contrast": evidence["background_support"]["relative_luminance_contrast"]["minimum_local_p10"],
            "foreground_polarity": evidence["foreground_polarity"],
            "support_requirement": evidence["support_requirement"],
            "background_polarity": evidence["background_support"]["polarity"],
            "visual_adequacy": {
                "status": "reselected" if reselected else "passed",
                "adequate": True,
                "contrast_preferred_family_id": contrast_selected[0]["id"] if contrast_selected else "",
                "selected_family_id": selected["id"],
                "reselected_without_regenerating_base": reselected,
                "inadequacy_codes": [],
            },
            "candidates": candidates,
        }
    if contrast_selected is None:
        return None, None, None, {
            "size": size,
            "selection_scope": "delivery_size",
            "selection_mode": "automatic_family_contrast",
            "selection_reason": "no_evaluable_template_for_size",
            "selected_family_id": "",
            "selected_family_label": "",
            "selected_source_role": "",
            "visual_adequacy": {
                "status": "failed_no_evaluable_template",
                "adequate": False,
                "failure_code": "prime_template_evaluation_failed",
                "inadequacy_codes": ["prime_template_evaluation_failed"],
                "contrast_preferred_family_id": "",
                "selected_family_id": "",
                "reselected_without_regenerating_base": False,
            },
            "candidates": candidates,
        }

    # Support measurements are quality evidence, not an availability gate. An
    # approved template with the strongest measured support still composes; the
    # final visual QC evaluates the actual Prime image.
    selected, template, evidence = contrast_selected
    return selected, template, evidence, {
        "size": size,
        "selection_scope": "delivery_size",
        "selection_mode": "automatic_family_contrast",
        "selection_reason": "highest_measured_support_with_qc_risk",
        "selected_family_id": selected["id"],
        "selected_family_label": selected["label"],
        "selected_source_role": evidence["selected_source_role"],
        "minimum_band_contrast": evidence["minimum_band_contrast"],
        "average_band_contrast": evidence["average_band_contrast"],
        "overall_contrast": evidence["overall_contrast"],
        "minimum_relative_luminance_contrast": evidence["background_support"]["relative_luminance_contrast"]["minimum_local_p10"],
        "foreground_polarity": evidence["foreground_polarity"],
        "support_requirement": evidence["support_requirement"],
        "background_polarity": evidence["background_support"]["polarity"],
        "visual_adequacy": {
            "status": "qc_risk",
            "adequate": False,
            "inadequacy_codes": evidence["visual_adequacy"]["inadequacy_codes"],
            "contrast_preferred_family_id": selected["id"],
            "selected_family_id": selected["id"],
            "reselected_without_regenerating_base": False,
        },
        "candidates": candidates,
    }


def compose_job(job: dict[str, Any], body: Image.Image, family: dict[str, Any], template: Image.Image, template_evidence: dict[str, Any], family_selection: dict[str, Any], layout: dict[str, Any], root: Path) -> dict[str, Any]:
    size = job["size"]
    input_path = root / str(job["input"])
    output_path = root / str(job["output"])
    final = Image.alpha_composite(body, template)
    output_path.parent.mkdir(parents=True, exist_ok=True)
    final.save(output_path, "PNG")
    visibility_audit = template_evidence["visibility_audit"]
    qr = skipped_qr_evidence()
    qr["validation_basis"] = "template_owned_qr_decode_disabled"
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
    selections: list[dict[str, Any]] = []
    selected_jobs: list[tuple[dict[str, Any], dict[str, Any], Image.Image, dict[str, Any], dict[str, Any]]] = []
    for job in prepared_jobs:
        family, template, evidence, selection = select_template_for_size(job, package, manifest_path.parent)
        selections.append({"id": job["id"], "size": job["size"], "template_selection": selection})
        if family is None or template is None or evidence is None:
            failures.append({
                "id": job["id"],
                "size": job["size"],
                "error_code": "prime_no_adequate_template_for_size",
                "error": f"no approved Prime template is visually adequate for {job['size']}",
                "template_selection": selection,
            })
        else:
            selected_jobs.append((job, family, template, evidence, selection))
    results = []
    if not failures:
        try:
            for job, family, template, evidence, selection in selected_jobs:
                results.append(compose_job(job, job["body"], family, template, evidence, selection, package["layout_contract"]["layouts"][job["size"]], manifest_path.parent))
        except Exception as error:
            failures.extend({"id": job.get("id", ""), "size": job.get("size", ""), "error_code": "prime_compose_failed", "error": str(error)} for job in prepared_jobs)
            results = []
    if failures:
        results = []
        for job in package["jobs"]:
            try:
                (manifest_path.parent / str(job["output"])).unlink(missing_ok=True)
            except OSError:
                pass
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
        "size_selections": selections,
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
