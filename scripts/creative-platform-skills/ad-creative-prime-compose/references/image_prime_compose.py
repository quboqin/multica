#!/usr/bin/env python
"""Deterministically package an ad body with a full Prime frame and a dynamic QR."""

import argparse
import json
from pathlib import Path

import cv2
import numpy as np
import qrcode
from PIL import Image


# The delivery canvases are contractual. A QR replaces only the pre-rendered
# code in the template's existing QR slot; all surrounding artwork stays intact.
SLOTS = {
    (1080, 1080): {
        "name": "square",
        "xy": (1002, 22),
        "clear_box": (983, 29, 1053, 99),
        "max_qr_width": 70,
    },
    (1200, 628): {
        "name": "landscape",
        "xy": (1128, 13),
        "clear_box": (1133, 20, 1185, 73),
        "max_qr_width": 70,
    },
    (800, 1000): {
        "name": "portrait_4_5",
        "xy": (727, 20),
        "clear_box": (724, 22, 779, 76),
        "max_qr_width": 70,
    },
}


def render_qr(payload: str) -> Image.Image:
    code = qrcode.QRCode(
        error_correction=qrcode.constants.ERROR_CORRECT_Q,
        box_size=2,
        border=1,
    )
    code.add_data(payload)
    code.make(fit=True)
    return code.make_image(fill_color="black", back_color="white").convert("RGB")


def template_asset_layer(template: Image.Image, size: tuple[int, int]) -> Image.Image:
    """Convert flattened Prime artwork into an overlay without its white canvas."""
    frame = template.convert("RGBA").resize(size, Image.Resampling.LANCZOS)
    pixels = np.asarray(frame).copy()
    rgb = pixels[..., :3].astype(np.int16)
    neutral = np.max(rgb, axis=2) - np.min(rgb, axis=2) < 10
    brightness = np.min(rgb, axis=2)
    fade = np.clip((255 - brightness) * 4, 0, 255).astype(np.uint8)
    pixels[..., 3] = np.where(neutral, np.minimum(pixels[..., 3], fade), pixels[..., 3])
    return Image.fromarray(pixels, "RGBA")


def clear_template_qr(frame: Image.Image, clear_box: tuple[int, int, int, int]) -> Image.Image:
    """Remove the flattened template QR before placing the configured QR payload."""
    pixels = np.asarray(frame).copy()
    left, top, right, bottom = clear_box
    pixels[top:bottom, left:right, 3] = 0
    return Image.fromarray(pixels, "RGBA")


def decode_qr(rgb: np.ndarray, scale: int = 1) -> dict:
    image = cv2.cvtColor(rgb, cv2.COLOR_RGB2BGR)
    if scale > 1:
        image = cv2.resize(
            image,
            (image.shape[1] * scale, image.shape[0] * scale),
            interpolation=cv2.INTER_NEAREST,
        )
    decoded, points, _ = cv2.QRCodeDetector().detectAndDecode(image)
    normalized_points = None
    if points is not None:
        normalized_points = (points.reshape(-1, 2) / scale).round(1).tolist()
    return {"decoded": decoded, "detected_points": normalized_points}


def compose(input_path: Path, template_path: Path, output_path: Path, payload: str) -> dict:
    body = Image.open(input_path).convert("RGBA")
    slot = SLOTS.get(body.size)
    if slot is None:
        valid = ", ".join(f"{width}x{height}" for width, height in SLOTS)
        raise ValueError(f"input must use a contractual delivery size: {valid}")

    template = Image.open(template_path)
    frame = clear_template_qr(
        template_asset_layer(template, body.size), slot["clear_box"]
    )
    canvas = Image.alpha_composite(body, frame).convert("RGB")
    qr = render_qr(payload)
    if qr.width > slot["max_qr_width"]:
        raise ValueError("QR payload cannot fit the template QR slot without covering the terms")
    x, y = slot["xy"]
    if x + qr.width > canvas.width or y + qr.height > canvas.height:
        raise ValueError("configured QR slot cannot contain the rendered QR")
    canvas.paste(qr, (x, y))
    output_path.parent.mkdir(parents=True, exist_ok=True)
    canvas.save(output_path, "PNG", optimize=True)

    final = np.asarray(Image.open(output_path).convert("RGB"))
    full_frame = decode_qr(final)
    full_frame_2x = decode_qr(final, scale=2)
    margin = 12
    crop_box = {
        "x": max(0, x - margin),
        "y": max(0, y - margin),
        "right": min(canvas.width, x + qr.width + margin),
        "bottom": min(canvas.height, y + qr.height + margin),
    }
    crop = final[
        crop_box["y"] : crop_box["bottom"], crop_box["x"] : crop_box["right"]
    ]
    slot_crop = decode_qr(crop)
    if slot_crop["detected_points"] is not None:
        slot_crop["detected_points"] = [
            [point_x + crop_box["x"], point_y + crop_box["y"]]
            for point_x, point_y in slot_crop["detected_points"]
        ]
    attempts = {
        "full_frame_1x": full_frame,
        "full_frame_2x_nearest": full_frame_2x,
        "slot_crop_1x": {**slot_crop, "crop_box": crop_box},
    }
    successful_attempt = next(
        (name for name, evidence in attempts.items() if evidence["decoded"] == payload),
        None,
    )
    selected = attempts[successful_attempt] if successful_attempt else full_frame
    result = {
        "output": str(output_path),
        "template": str(template_path),
        "canvas": {"width": canvas.width, "height": canvas.height},
        "template_kind": slot["name"],
        "qr_payload": payload,
        "qr": {
            "x": x,
            "y": y,
            "width": qr.width,
            "height": qr.height,
            "cleared_template_box": slot["clear_box"],
        },
        "decoded": selected["decoded"],
        "passed": successful_attempt is not None,
        "detected_points": selected["detected_points"],
        "successful_attempt": successful_attempt,
        "decode_attempts": attempts,
    }
    if not result["passed"]:
        raise RuntimeError(json.dumps(result, ensure_ascii=False))
    return result


def resolve_manifest_path(base_dir: Path, value: str) -> Path:
    path = Path(value)
    return path if path.is_absolute() else base_dir / path


def compose_manifest(manifest_path: Path) -> dict:
    payload = json.loads(manifest_path.read_text(encoding="utf-8"))
    jobs = payload.get("jobs")
    if not isinstance(jobs, list) or not 1 <= len(jobs) <= 50:
        raise ValueError("manifest jobs must contain between 1 and 50 entries")
    base_dir = manifest_path.resolve().parent
    results = []
    seen_ids = set()
    for index, job in enumerate(jobs, start=1):
        if not isinstance(job, dict):
            raise ValueError(f"manifest job {index} must be an object")
        job_id = str(job.get("id") or f"job-{index}").strip()
        if not job_id or job_id in seen_ids:
            raise ValueError(f"manifest job id must be non-empty and unique: {job_id!r}")
        seen_ids.add(job_id)
        try:
            result = compose(
                resolve_manifest_path(base_dir, str(job["input"])),
                resolve_manifest_path(base_dir, str(job["template"])),
                resolve_manifest_path(base_dir, str(job["output"])),
                str(job["qr_payload"]),
            )
            results.append({"id": job_id, "status": "succeeded", **result})
        except Exception as error:
            results.append({"id": job_id, "status": "failed", "error": str(error)})
    failed = sum(result["status"] == "failed" for result in results)
    return {
        "succeeded": len(results) - failed,
        "failed": failed,
        "results": results,
    }


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--manifest")
    parser.add_argument("--input")
    parser.add_argument("--template")
    parser.add_argument("--output")
    parser.add_argument("--qr-payload")
    args = parser.parse_args()
    if args.manifest:
        if any((args.input, args.template, args.output, args.qr_payload)):
            parser.error("--manifest cannot be combined with single-image arguments")
        result = compose_manifest(Path(args.manifest))
        print(json.dumps(result, ensure_ascii=False))
        if result["failed"]:
            raise SystemExit(1)
        return
    if not all((args.input, args.template, args.output, args.qr_payload)):
        parser.error("single-image mode requires --input, --template, --output and --qr-payload")
    print(json.dumps(compose(Path(args.input), Path(args.template), Path(args.output), args.qr_payload), ensure_ascii=False))


if __name__ == "__main__":
    main()
