#!/usr/bin/env python3
"""Run deterministic checks for a Prime-packaged creative image batch."""

from __future__ import annotations

import argparse
import json
import math
import re
from pathlib import Path

import cv2
import numpy as np
from PIL import Image, ImageDraw, ImageFont, ImageOps


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser()
    parser.add_argument("--manifest", required=True)
    parser.add_argument("--compose-result", required=True)
    parser.add_argument("--images-dir", required=True)
    parser.add_argument("--output", required=True)
    parser.add_argument("--contact-sheet", required=True)
    parser.add_argument("--hard-region-sheet", required=True)
    return parser.parse_args()


def decode_qr(path: Path) -> str:
    image = cv2.imread(str(path), cv2.IMREAD_COLOR)
    if image is None:
        return ""
    detector = cv2.QRCodeDetector()
    decoded, _, _ = detector.detectAndDecode(image)
    if decoded:
        return decoded
    enlarged = cv2.resize(image, None, fx=2, fy=2, interpolation=cv2.INTER_NEAREST)
    decoded, _, _ = detector.detectAndDecode(enlarged)
    return decoded or ""


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


def valid_layout_contract(layout: object) -> dict | None:
    if not isinstance(layout, dict):
        return None
    if not isinstance(layout.get("hard_regions"), list):
        return None
    if not isinstance(layout.get("top_key_content_exclusion_end"), int):
        return None
    if not isinstance(layout.get("bottom_key_content_exclusion_start"), int):
        return None
    return layout


def resolve_layout_contract(manifest: dict, job: dict, size: str) -> dict | None:
    # Prime compose manifests carry the exact size-specific contract on each
    # job. Keep top-level layout support for older archived manifests.
    direct = job.get("layout_contract") or job.get("layout") or job
    layout = valid_layout_contract(direct)
    if layout is not None:
        return layout
    contract = manifest.get("prime_layout_contract") or manifest.get("layout_contract") or {}
    layouts = contract.get("layouts") if isinstance(contract, dict) else None
    return valid_layout_contract(layouts.get(size) if isinstance(layouts, dict) else None)


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

    variant = str(job.get("variant") or str(job.get("id", "")).split("-", 1)[0]).upper()
    canvas = compose_item.get("canvas") or {}
    size = str(job.get("size") or "")
    if not size and canvas.get("width") and canvas.get("height"):
        size = f"{canvas['width']}x{canvas['height']}"

    revision = manifest.get("revision")
    if revision in (None, ""):
        match = re.search(r"_r(\d+)(?:\.[^.]+)?$", output_name, flags=re.IGNORECASE)
        revision = int(match.group(1)) if match else None

    approved_payload = (
        manifest.get("qr_validation", {}).get("approved_payload")
        or job.get("qr_payload")
        or compose_item.get("qr_payload")
        or ""
    )
    return {
        "image_path": image_path,
        "expected_output_name": output_name,
        "variant": variant,
        "size": size,
        "revision": revision,
        "approved_payload": approved_payload,
    }


def main() -> int:
    args = parse_args()
    manifest = json.loads(Path(args.manifest).read_text(encoding="utf-8-sig"))
    compose = json.loads(Path(args.compose_result).read_text(encoding="utf-8-sig"))
    images_dir = Path(args.images_dir).resolve()
    compose_by_id = {item.get("id"): item for item in compose.get("results", [])}

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
        expected_width, expected_height = (int(value) for value in size.lower().split("x", 1))
        layout = resolve_layout_contract(manifest, job, size)
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
        decoded = decode_qr(image_path) if exists else ""
        checks = {
            "exists": exists,
            "dimensions": actual_size == [expected_width, expected_height],
            "filename": bool(expected_output_name) and image_path.name == expected_output_name,
            "compose": compose_item.get("status") == "succeeded" and compose_item.get("passed") is True,
            "qr": bool(approved_payload) and decoded == approved_payload and compose_item.get("decoded") == approved_payload,
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
