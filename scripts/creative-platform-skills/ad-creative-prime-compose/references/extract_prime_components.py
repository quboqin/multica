#!/usr/bin/env python
"""One-time migration utility for extracting standalone assets from a legacy Prime sheet."""

from __future__ import annotations

import argparse
import json
import math
from pathlib import Path
from typing import Any

import cv2
import numpy as np
import qrcode
from PIL import Image


def require_pair(value: object, field: str) -> tuple[int, int]:
    if not isinstance(value, list) or len(value) != 2 or not all(isinstance(item, int) and item > 0 for item in value):
        raise ValueError(f"{field} must contain two positive integers")
    return value[0], value[1]


def require_rect(value: object, field: str) -> list[int]:
    if not isinstance(value, list) or len(value) != 4 or not all(isinstance(item, int) for item in value):
        raise ValueError(f"{field} must contain four integers")
    left, top, right, bottom = value
    if left < 0 or top < 0 or right <= left or bottom <= top:
        raise ValueError(f"{field} must be a non-empty rectangle")
    return value


def resolve_rect(rect: list[int], authored_size: tuple[int, int], source_size: tuple[int, int]) -> list[int]:
    left, top, right, bottom = require_rect(rect, "source_rect")
    authored_width, authored_height = authored_size
    source_width, source_height = source_size
    if right > authored_width or bottom > authored_height:
        raise ValueError("source_rect exceeds authored_size")
    return [
        math.floor(left * source_width / authored_width),
        math.floor(top * source_height / authored_height),
        math.ceil(right * source_width / authored_width),
        math.ceil(bottom * source_height / authored_height),
    ]


def remove_edge_background(image: Image.Image) -> Image.Image:
    pixels = np.asarray(image.convert("RGBA")).copy()
    rgb = pixels[..., :3]
    near_white = ((rgb.min(axis=2) >= 235) & ((rgb.max(axis=2) - rgb.min(axis=2)) <= 12)).astype(np.uint8)
    count, labels = cv2.connectedComponents(near_white, connectivity=8)
    edge_labels = set(np.unique(np.concatenate((labels[0], labels[-1], labels[:, 0], labels[:, -1]))).tolist())
    background = np.isin(labels, [label for label in edge_labels if 0 < label < count])
    pixels[background, 3] = 0
    return Image.fromarray(pixels, "RGBA")


def normalize_qr(image: Image.Image) -> tuple[Image.Image, str]:
    rgb = np.asarray(image.convert("RGB"))
    payload, _, _ = cv2.QRCodeDetector().detectAndDecode(cv2.cvtColor(rgb, cv2.COLOR_RGB2BGR))
    if not payload:
        raise ValueError("QR migration source could not be decoded")
    code = qrcode.QRCode(
        error_correction=qrcode.constants.ERROR_CORRECT_Q,
        box_size=10,
        border=1,
    )
    code.add_data(payload)
    code.make(fit=True)
    return code.make_image(fill_color="black", back_color="white").convert("RGBA"), payload


def resolve_path(base_dir: Path, value: object) -> Path:
    path = Path(str(value))
    return path if path.is_absolute() else base_dir / path


def extract_manifest(manifest_path: Path) -> dict[str, Any]:
    manifest = json.loads(manifest_path.read_text(encoding="utf-8-sig"))
    base_dir = manifest_path.resolve().parent
    source_path = resolve_path(base_dir, manifest.get("source"))
    authored_size = require_pair(manifest.get("authored_size"), "authored_size")
    components = manifest.get("components")
    if not isinstance(components, list) or not components:
        raise ValueError("components must be a non-empty array")

    results = []
    seen_roles: set[str] = set()
    with Image.open(source_path) as source:
        source_rgba = source.convert("RGBA")
        for index, component in enumerate(components):
            if not isinstance(component, dict):
                raise ValueError(f"components[{index}] must be an object")
            role = str(component.get("role") or "").strip()
            if not role or role in seen_roles:
                raise ValueError(f"component role must be non-empty and unique: {role!r}")
            seen_roles.add(role)
            output_path = resolve_path(base_dir, component.get("output"))
            authored_rect = require_rect(component.get("source_rect"), f"{role}.source_rect")
            resolved_rect = resolve_rect(authored_rect, authored_size, source_rgba.size)
            crop = source_rgba.crop(tuple(resolved_rect))
            background = str(component.get("background") or "transparent").lower()
            decoded_payload = ""
            if component.get("normalize_qr") is True:
                crop, decoded_payload = normalize_qr(crop)
                expected_payload = str(component.get("expected_payload") or "")
                if expected_payload and decoded_payload != expected_payload:
                    raise ValueError(f"{role} decoded payload does not match expected_payload")
                background = "opaque"
            elif background == "transparent":
                crop = remove_edge_background(crop)
            elif background != "opaque":
                raise ValueError(f"{role}.background must be transparent or opaque")
            output_path.parent.mkdir(parents=True, exist_ok=True)
            crop.save(output_path, "PNG", optimize=True)
            results.append({
                "role": role,
                "output": str(output_path),
                "authored_source_rect": authored_rect,
                "resolved_source_rect": resolved_rect,
                "width": crop.width,
                "height": crop.height,
                "background": background,
                "decoded_payload": decoded_payload,
            })
    return {"source": str(source_path), "components": results}


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--manifest", required=True)
    args = parser.parse_args()
    print(json.dumps(extract_manifest(Path(args.manifest)), ensure_ascii=False))


if __name__ == "__main__":
    main()
