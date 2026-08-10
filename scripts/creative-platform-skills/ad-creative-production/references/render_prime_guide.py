#!/usr/bin/env python3
"""Render a non-content Prime reserved-area guide for image-model layout input."""

from __future__ import annotations

import argparse
import json
from pathlib import Path
from typing import Any

from PIL import Image, ImageDraw

from validate_copy_snapshot import layout_safe_content_frame, load_prime_layout


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser()
    parser.add_argument("--layout-file", required=True)
    parser.add_argument("--output", required=True)
    parser.add_argument("--width", type=int)
    parser.add_argument("--height", type=int)
    parser.add_argument("--size-key")
    parser.add_argument("--evidence")
    return parser.parse_args()


def target_size(layout: dict[str, Any], width: int | None, height: int | None) -> tuple[int, int]:
    resolved_width = width if width is not None else layout.get("__canvas_width")
    resolved_height = height if height is not None else layout.get("__canvas_height")
    if not isinstance(resolved_width, int) or isinstance(resolved_width, bool):
        raise SystemExit("width is required when the layout does not declare a canvas width")
    if not isinstance(resolved_height, int) or isinstance(resolved_height, bool):
        raise SystemExit("height is required when the layout does not declare a canvas height")
    if resolved_width < 1 or resolved_height < 1:
        raise SystemExit("width and height must be positive")
    return (resolved_width, resolved_height)


def select_layout(layouts: list[dict[str, Any]], size_key: str | None, width: int | None, height: int | None) -> dict[str, Any]:
    if size_key:
        matches = [layout for layout in layouts if str(layout.get("__size_key") or "") == size_key]
        if len(matches) != 1:
            raise SystemExit(f"prime layout size {size_key!r} was not found exactly once")
        return matches[0]
    if width is not None and height is not None:
        matches = [
            layout for layout in layouts
            if layout.get("__canvas_width") in (None, width) and layout.get("__canvas_height") in (None, height)
        ]
        if len(matches) == 1:
            return matches[0]
    if len(layouts) == 1:
        return layouts[0]
    raise SystemExit("multiple prime layouts found; pass --size-key")


def int_rect(payload: dict[str, Any]) -> tuple[int, int, int, int] | None:
    values = [payload.get(key) for key in ("x1", "y1", "x2", "y2")]
    if not all(isinstance(value, int) and not isinstance(value, bool) for value in values):
        return None
    x1, y1, x2, y2 = values
    if x2 <= x1 or y2 <= y1:
        return None
    return (x1, y1, x2, y2)


def draw_translucent_rect(canvas: Image.Image, rect: tuple[int, int, int, int], fill: tuple[int, int, int, int]) -> None:
    overlay = Image.new("RGBA", canvas.size, (0, 0, 0, 0))
    draw = ImageDraw.Draw(overlay)
    draw.rectangle(rect, fill=fill)
    canvas.alpha_composite(overlay)


def render_guide(layout: dict[str, Any], width: int, height: int) -> tuple[Image.Image, dict[str, Any]]:
    image = Image.new("RGBA", (width, height), (246, 248, 244, 255))
    draw = ImageDraw.Draw(image)

    top = layout.get("top_key_content_exclusion_end")
    if isinstance(top, int) and not isinstance(top, bool) and top > 0:
        draw_translucent_rect(image, (0, 0, width, min(height, top)), (220, 54, 46, 78))

    bottom = layout.get("bottom_key_content_exclusion_start")
    if isinstance(bottom, int) and not isinstance(bottom, bool) and bottom < height:
        draw_translucent_rect(image, (0, max(0, bottom), width, height), (220, 54, 46, 78))

    hard_regions = []
    for region in layout.get("hard_regions") or []:
        if not isinstance(region, dict):
            continue
        rect = int_rect(region)
        if rect is None:
            continue
        draw_translucent_rect(image, rect, (220, 54, 46, 172))
        draw.rectangle(rect, outline=(160, 30, 24, 255), width=max(2, width // 540))
        hard_regions.append({"id": region.get("id"), "rect": rect})

    frame = layout_safe_content_frame({**layout, "__canvas_width": width, "__canvas_height": height})
    if frame:
        rect = (frame["x1"], frame["y1"], frame["x2"], frame["y2"])
        draw.rectangle(rect, outline=(36, 128, 72, 255), width=max(3, width // 360))

    evidence = {
        "width": width,
        "height": height,
        "size_key": layout.get("__size_key"),
        "hard_regions": hard_regions,
        "safe_content_frame": frame,
        "non_rendering_guide": True,
    }
    return image, evidence


def main() -> int:
    args = parse_args()
    layouts = load_prime_layout(args.layout_file)
    layout = select_layout(layouts, args.size_key, args.width, args.height)
    width, height = target_size(layout, args.width, args.height)
    image, evidence = render_guide(layout, width, height)
    output_path = Path(args.output)
    output_path.parent.mkdir(parents=True, exist_ok=True)
    image.convert("RGB").save(output_path)
    if args.evidence:
        evidence_path = Path(args.evidence)
        evidence_path.parent.mkdir(parents=True, exist_ok=True)
        evidence_path.write_text(json.dumps(evidence, ensure_ascii=False, indent=2), encoding="utf-8")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
