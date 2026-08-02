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
    for job in manifest.get("jobs", []):
        compose_item = compose_by_id.get(job.get("id"), {})
        contract = resolve_job_contract(job, compose_item, manifest, images_dir)
        image_path = contract["image_path"]
        variant = contract["variant"]
        size = contract["size"]
        revision = contract["revision"]
        approved_payload = contract["approved_payload"]
        expected_width, expected_height = (int(value) for value in size.lower().split("x", 1))
        exists = image_path.is_file()
        actual_size = None
        white_ratio = None
        if exists:
            with Image.open(image_path) as image:
                actual_size = list(image.size)
                white_ratio = edge_white_ratio(image)
            contact_images.append((f"{variant} {size}", image_path))
        decoded = decode_qr(image_path) if exists else ""
        checks = {
            "exists": exists,
            "dimensions": actual_size == [expected_width, expected_height],
            "filename": variant in image_path.name and (revision is None or f"_r{revision}" in image_path.name),
            "compose": compose_item.get("status") == "succeeded" and compose_item.get("passed") is True,
            "qr": bool(approved_payload) and decoded == approved_payload and compose_item.get("decoded") == approved_payload,
            "full_bleed": white_ratio is not None and white_ratio < 0.5,
        }
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
            "checks": checks,
            "passed": all(checks.values()),
        })

    make_contact_sheet(contact_images, Path(args.contact_sheet).resolve())
    evidence = {
        "candidate_id": manifest.get("candidate_id"),
        "revision": manifest.get("revision"),
        "market_pack": manifest.get("market_pack"),
        "copy_snapshot": manifest.get("copy_snapshot"),
        "approved_payloads": sorted({item["approved_payload"] for item in results if item["approved_payload"]}),
        "compose_summary": {"succeeded": compose.get("succeeded"), "failed": compose.get("failed")},
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
