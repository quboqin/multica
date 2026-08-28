#!/usr/bin/env python3
"""Normalize a provider image to an exact delivery canvas with recorded aspect handling."""

from __future__ import annotations

import argparse
import json
from pathlib import Path

from PIL import Image, ImageFilter


RESAMPLING = getattr(Image, "Resampling", Image)


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser()
    parser.add_argument("--input", required=True)
    parser.add_argument("--output", required=True)
    parser.add_argument("--width", required=True, type=int)
    parser.add_argument("--height", required=True, type=int)
    parser.add_argument("--model-size")
    parser.add_argument("--evidence")
    parser.add_argument("--max-aspect-deviation", type=float, default=0.10)
    parser.add_argument("--allow-aspect-fallback", action="store_true")
    parser.add_argument(
        "--aspect-fallback-mode",
        choices=("compress", "contain-edge-extend"),
        default="compress",
    )
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


def parse_canvas(value: str, field: str) -> tuple[int, int]:
    parts = value.strip().lower().split("x")
    if len(parts) != 2:
        raise SystemExit(f"{field} must be WIDTHxHEIGHT")
    try:
        width, height = (int(part) for part in parts)
    except ValueError as exc:
        raise SystemExit(f"{field} must be WIDTHxHEIGHT") from exc
    if width < 1 or height < 1:
        raise SystemExit(f"{field} must be WIDTHxHEIGHT")
    return width, height


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


def contain_with_edge_extension(source: Image.Image, width: int, height: int) -> tuple[Image.Image, tuple[int, int, int, int]]:
    scale = min(width / source.width, height / source.height)
    fitted_width = max(1, round(source.width * scale))
    fitted_height = max(1, round(source.height * scale))
    fitted = source.resize((fitted_width, fitted_height), RESAMPLING.LANCZOS)
    canvas = Image.new("RGB", (width, height), fitted.getpixel((fitted.width // 2, fitted.height // 2)))
    x1 = (width - fitted_width) // 2
    y1 = (height - fitted_height) // 2
    x2 = x1 + fitted_width
    y2 = y1 + fitted_height
    canvas.paste(fitted, (x1, y1))
    blur_radius = max(2, min(width, height) // 80)

    if x1 > 0:
        strip_width = min(fitted.width, max(2, fitted.width // 24))
        left = fitted.crop((0, 0, strip_width, fitted.height)).resize((x1, height), RESAMPLING.BICUBIC)
        canvas.paste(left.filter(ImageFilter.GaussianBlur(blur_radius)), (0, 0))
        if width > x2:
            right = fitted.crop((fitted.width - strip_width, 0, fitted.width, fitted.height)).resize((width - x2, height), RESAMPLING.BICUBIC)
            canvas.paste(right.filter(ImageFilter.GaussianBlur(blur_radius)), (x2, 0))
    if y1 > 0:
        strip_height = min(fitted.height, max(2, fitted.height // 24))
        top = fitted.crop((0, 0, fitted.width, strip_height)).resize((width, y1), RESAMPLING.BICUBIC)
        canvas.paste(top.filter(ImageFilter.GaussianBlur(blur_radius)), (0, 0))
        if height > y2:
            bottom = fitted.crop((0, fitted.height - strip_height, fitted.width, fitted.height)).resize((width, height - y2), RESAMPLING.BICUBIC)
            canvas.paste(bottom.filter(ImageFilter.GaussianBlur(blur_radius)), (0, y2))
    canvas.paste(fitted, (x1, y1))
    return canvas, (x1, y1, x2, y2)


def compress_to_canvas(source: Image.Image, width: int, height: int) -> tuple[Image.Image, tuple[int, int, int, int]]:
    return source.resize((width, height), RESAMPLING.LANCZOS), (0, 0, width, height)


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
    if not 0 <= args.max_aspect_deviation <= 0.25:
        raise SystemExit("max-aspect-deviation must be between 0 and 0.25")

    source_path = Path(args.input).resolve()
    output_path = Path(args.output).resolve()
    with Image.open(source_path) as source:
        source.load()
        source_width, source_height = source.size
        source_ratio = source_width / source_height
        target_ratio = args.width / args.height
        model_width, model_height = parse_canvas(args.model_size or f"{args.width}x{args.height}", "model-size")
        model_ratio = model_width / model_height
        source_aspect_deviation = abs(source_ratio / model_ratio - 1)
        aspect_fallback_used = source_aspect_deviation > args.max_aspect_deviation
        if aspect_fallback_used and not args.allow_aspect_fallback:
            raise SystemExit(
                f"source aspect ratio deviates {source_aspect_deviation:.2%} from model canvas {model_width}x{model_height}; regenerate this size"
            )
        source_rgb = source.convert("RGB")
        content_rect = (0, 0, args.width, args.height)
        if aspect_fallback_used:
            if args.aspect_fallback_mode == "contain-edge-extend":
                normalized, content_rect = contain_with_edge_extension(source_rgb, args.width, args.height)
                method = "contain-edge-extend"
            else:
                normalized, content_rect = compress_to_canvas(source_rgb, args.width, args.height)
                method = "aspect-compress"
        else:
            normalized = source_rgb.resize((args.width, args.height), RESAMPLING.LANCZOS)
            method = "direct-resize-lanczos"
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
        "model_canvas": {"width": model_width, "height": model_height},
        "source_aspect_deviation": round(source_aspect_deviation, 6),
        "max_aspect_deviation": args.max_aspect_deviation,
        "delivery_aspect_deformation": round(abs(source_ratio / target_ratio - 1), 6),
        "crop_fraction": 0.0,
        "extension_fraction": round(1 - ((content_rect[2] - content_rect[0]) * (content_rect[3] - content_rect[1])) / (args.width * args.height), 6),
        "aspect_fallback_used": aspect_fallback_used,
        "content_rect": {"x1": content_rect[0], "y1": content_rect[1], "x2": content_rect[2], "y2": content_rect[3]},
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
