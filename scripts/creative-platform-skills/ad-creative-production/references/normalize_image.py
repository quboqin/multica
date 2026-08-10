#!/usr/bin/env python3
"""Normalize a provider image to an exact delivery canvas without stretching content."""

from __future__ import annotations

import argparse
import json
import math
from pathlib import Path

from PIL import Image, ImageChops, ImageDraw, ImageOps


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser()
    parser.add_argument("--input", required=True)
    parser.add_argument("--output", required=True)
    parser.add_argument("--width", required=True, type=int)
    parser.add_argument("--height", required=True, type=int)
    parser.add_argument("--evidence")
    parser.add_argument("--max-crop-fraction", type=float, default=0.03)
    parser.add_argument("--max-extension-fraction", type=float, default=0.0)
    parser.add_argument("--prime-layout-file")
    parser.add_argument("--prime-safe-audit", action="store_true")
    parser.add_argument("--prime-safe-fit", action="store_true")
    parser.add_argument("--prime-safe-margin", type=int)
    parser.add_argument("--prime-safe-x-margin", type=int)
    parser.add_argument("--prime-safe-y-margin", type=int)
    return parser.parse_args()


def prime_safe_margins(args: argparse.Namespace, layout: dict[str, object], width: int, height: int) -> tuple[int, int]:
    x_margin = args.prime_safe_x_margin
    y_margin = args.prime_safe_y_margin
    shared_margin = args.prime_safe_margin
    if x_margin is None and y_margin is None and shared_margin is None:
        top = layout.get("top_key_content_exclusion_end", 0)
        bottom = layout.get("bottom_key_content_exclusion_start", height)
        if not isinstance(top, int) or isinstance(top, bool):
            top = 0
        if not isinstance(bottom, int) or isinstance(bottom, bool):
            bottom = height
        vertical_reserve = round(max(top, height - bottom) / 2)
        horizontal_reserve = round(vertical_reserve * width / max(1, height) * 2 / 3)
        return (horizontal_reserve, vertical_reserve)
    if x_margin is None:
        x_margin = shared_margin if shared_margin is not None else 0
    if y_margin is None:
        y_margin = shared_margin if shared_margin is not None else 0
    return (x_margin, y_margin)


def contract_safe_frame(layout: dict[str, object]) -> tuple[int, int, int, int] | None:
    for key in ("safe_content_frame", "content_safe_frame"):
        frame = layout.get(key)
        if not isinstance(frame, dict):
            continue
        values = [frame.get(name) for name in ("x1", "y1", "x2", "y2")]
        if all(isinstance(value, int) and not isinstance(value, bool) for value in values):
            x1, y1, x2, y2 = values
            if x2 > x1 and y2 > y1:
                return (x1, y1, x2, y2)
    return None


def load_prime_layout(path: str | None, width: int, height: int) -> dict[str, object] | None:
    if not path:
        return None
    payload = json.loads(Path(path).read_text(encoding="utf-8"))
    if "layouts" in payload:
        layout = payload["layouts"].get(f"{width}x{height}")
    else:
        layout = payload
    if not isinstance(layout, dict):
        raise SystemExit("prime layout file must contain the target layout")
    return layout


def load_prime_safe_rect(
    path: str | None,
    width: int,
    height: int,
    args: argparse.Namespace,
) -> tuple[tuple[int, int, int, int] | None, tuple[int, int], str]:
    layout = load_prime_layout(path, width, height)
    if layout is None:
        return (None, (0, 0), "none")
    contract_frame = contract_safe_frame(layout)
    if contract_frame is not None and args.prime_safe_margin is None and args.prime_safe_x_margin is None and args.prime_safe_y_margin is None:
        top = layout.get("top_key_content_exclusion_end", 0)
        if not isinstance(top, int) or isinstance(top, bool):
            top = 0
        return (contract_frame, (contract_frame[0], max(0, contract_frame[1] - top)), "contract_safe_content_frame")
    x_margin, y_margin = prime_safe_margins(args, layout, width, height)
    top = layout.get("top_key_content_exclusion_end", 0)
    bottom = layout.get("bottom_key_content_exclusion_start", height)
    if not isinstance(top, int) or isinstance(top, bool):
        top = 0
    if not isinstance(bottom, int) or isinstance(bottom, bool):
        bottom = height
    x1 = x_margin
    y1 = min(height - 1, max(0, top + y_margin))
    x2 = max(x1 + 1, width - x_margin)
    y2 = max(y1 + 1, min(height, bottom - y_margin))
    if y2 - y1 < max(16, height // 4):
        raise SystemExit("prime safe content area is too small")
    if x2 - x1 < max(16, width // 4):
        raise SystemExit("prime safe content width is too small")
    return ((x1, y1, x2, y2), (x_margin, y_margin), "derived_from_prime_layout_contract")


def prime_safe_audit(
    rect: tuple[int, int, int, int],
    margins: tuple[int, int],
) -> dict[str, object]:
    x1, y1, x2, y2 = rect
    return {
        "prime_safe_audit": True,
        "prime_safe_fit": False,
        "edge_to_edge_canvas_preserved": True,
        "safe_content_margin": {"x": margins[0], "y": margins[1]},
        "safe_content_rect": {"x1": x1, "y1": y1, "x2": x2, "y2": y2},
    }


def main() -> int:
    args = parse_args()
    if args.width < 1 or args.height < 1:
        raise SystemExit("width and height must be positive")
    if not 0 <= args.max_crop_fraction <= 0.25:
        raise SystemExit("max-crop-fraction must be between 0 and 0.25")
    if not 0 <= args.max_extension_fraction <= 0.5:
        raise SystemExit("max-extension-fraction must be between 0 and 0.5")

    source_path = Path(args.input).resolve()
    output_path = Path(args.output).resolve()
    with Image.open(source_path) as source:
        source.load()
        source_width, source_height = source.size
        source_ratio = source_width / source_height
        target_ratio = args.width / args.height
        if source_ratio >= target_ratio:
            crop_fraction = 1 - (target_ratio / source_ratio)
        else:
            crop_fraction = 1 - (source_ratio / target_ratio)
        source_rgb = source.convert("RGB")
        extension_fraction = 0.0
        if crop_fraction <= args.max_crop_fraction:
            normalized = ImageOps.fit(
                source_rgb,
                (args.width, args.height),
                method=Image.Resampling.LANCZOS,
                centering=(0.5, 0.5),
            )
            method = "cover-center-crop-lanczos"
        else:
            if source_ratio < target_ratio:
                canvas_size = (math.ceil(source_height * target_ratio), source_height)
                extension_fraction = 1 - (source_width / canvas_size[0])
            else:
                canvas_size = (source_width, math.ceil(source_width / target_ratio))
                extension_fraction = 1 - (source_height / canvas_size[1])
            if extension_fraction > args.max_extension_fraction:
                raise SystemExit(
                    f"source aspect ratio requires {crop_fraction:.2%} crop or "
                    f"{extension_fraction:.2%} background extension; regenerate this size"
                )

            backdrop = Image.new("RGB", canvas_size)
            offset = ((canvas_size[0] - source_width) // 2, (canvas_size[1] - source_height) // 2)
            if canvas_size[0] > source_width:
                left_pad = offset[0]
                right_pad = canvas_size[0] - source_width - left_pad
                left_edge = source_rgb.crop((0, 0, 1, source_height))
                right_edge = source_rgb.crop((source_width - 1, 0, source_width, source_height))
                if left_pad:
                    left = left_edge.resize((left_pad, source_height))
                    background = ImageChops.lighter(left, right_edge.resize((left_pad, source_height)))
                    ramp = Image.new("L", (left_pad, source_height))
                    ramp_draw = ImageDraw.Draw(ramp)
                    for x in range(left_pad):
                        ramp_draw.line((x, 0, x, source_height), fill=round(255 * x / max(1, left_pad - 1)))
                    left = Image.composite(left, background, ramp)
                    backdrop.paste(left, (0, 0))
                if right_pad:
                    right = right_edge.resize((right_pad, source_height))
                    background = ImageChops.lighter(left_edge.resize((right_pad, source_height)), right)
                    ramp = Image.new("L", (right_pad, source_height))
                    ramp_draw = ImageDraw.Draw(ramp)
                    for x in range(right_pad):
                        ramp_draw.line((x, 0, x, source_height), fill=round(255 * x / max(1, right_pad - 1)))
                    right = Image.composite(background, right, ramp)
                    backdrop.paste(right, (left_pad + source_width, 0))
            else:
                top_pad = offset[1]
                bottom_pad = canvas_size[1] - source_height - top_pad
                top_edge = source_rgb.crop((0, 0, source_width, 1))
                bottom_edge = source_rgb.crop((0, source_height - 1, source_width, source_height))
                if top_pad:
                    top = top_edge.resize((source_width, top_pad))
                    background = ImageChops.lighter(top, bottom_edge.resize((source_width, top_pad)))
                    ramp = Image.new("L", (source_width, top_pad))
                    ramp_draw = ImageDraw.Draw(ramp)
                    for y in range(top_pad):
                        ramp_draw.line((0, y, source_width, y), fill=round(255 * y / max(1, top_pad - 1)))
                    top = Image.composite(top, background, ramp)
                    backdrop.paste(top, (0, 0))
                if bottom_pad:
                    bottom = bottom_edge.resize((source_width, bottom_pad))
                    background = ImageChops.lighter(top_edge.resize((source_width, bottom_pad)), bottom)
                    ramp = Image.new("L", (source_width, bottom_pad))
                    ramp_draw = ImageDraw.Draw(ramp)
                    for y in range(bottom_pad):
                        ramp_draw.line((0, y, source_width, y), fill=round(255 * y / max(1, bottom_pad - 1)))
                    bottom = Image.composite(background, bottom, ramp)
                    backdrop.paste(bottom, (0, top_pad + source_height))
            backdrop.paste(source_rgb, offset)
            normalized = backdrop.resize((args.width, args.height), Image.Resampling.LANCZOS)
            method = "contain-edge-fade-extension-lanczos"
        output_path.parent.mkdir(parents=True, exist_ok=True)
        safe_fit_evidence: dict[str, object] = {"prime_safe_audit": False, "prime_safe_fit": False}
        if args.prime_safe_audit or args.prime_safe_fit:
            safe_rect, safe_margins, safe_rect_source = load_prime_safe_rect(args.prime_layout_file, args.width, args.height, args)
            if safe_rect is None:
                raise SystemExit("prime safe audit requires --prime-layout-file")
            if safe_margins[0] < 0 or safe_margins[1] < 0:
                raise SystemExit("prime safe margins must be non-negative")
            safe_fit_evidence = prime_safe_audit(safe_rect, safe_margins)
            safe_fit_evidence["safe_content_rect_source"] = safe_rect_source
        normalized.save(output_path, format="PNG", optimize=True)

    evidence = {
        "source": str(source_path),
        "source_size": {"width": source_width, "height": source_height},
        "target": str(output_path),
        "target_size": {"width": args.width, "height": args.height},
        "crop_fraction": round(crop_fraction, 6),
        "max_crop_fraction": args.max_crop_fraction,
        "extension_fraction": round(extension_fraction, 6),
        "max_extension_fraction": args.max_extension_fraction,
        "method": method,
        **safe_fit_evidence,
    }
    if args.evidence:
        evidence_path = Path(args.evidence).resolve()
        evidence_path.parent.mkdir(parents=True, exist_ok=True)
        evidence_path.write_text(json.dumps(evidence, ensure_ascii=False, indent=2), encoding="utf-8")
    print(json.dumps(evidence, ensure_ascii=False))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
