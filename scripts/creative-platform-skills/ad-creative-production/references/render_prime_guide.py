#!/usr/bin/env python3
"""Render a non-content Prime reserved-area guide for image-model layout input."""

from __future__ import annotations

import argparse
import json
from pathlib import Path
from typing import Any

from PIL import Image, ImageChops, ImageDraw

from validate_copy_snapshot import layout_safe_content_frame, load_prime_layout


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser()
    parser.add_argument("--layout-file", required=True)
    parser.add_argument("--output", required=True)
    parser.add_argument("--width", type=int)
    parser.add_argument("--height", type=int)
    parser.add_argument("--size-key")
    parser.add_argument("--template-image", help="Official Prime template preview for visual context")
    parser.add_argument("--content-envelope-output", help="Optional non-rendering guide for conservative title/table reflow")
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


def layout_canvas_size(layout: dict[str, Any], fallback: tuple[int, int]) -> tuple[int, int]:
    width = layout.get("__canvas_width")
    height = layout.get("__canvas_height")
    if isinstance(width, int) and isinstance(height, int) and not isinstance(width, bool) and not isinstance(height, bool) and width > 0 and height > 0:
        return (width, height)
    return fallback


def scale_rect(rect: tuple[int, int, int, int], source: tuple[int, int], target: tuple[int, int]) -> tuple[int, int, int, int]:
    source_width, source_height = source
    target_width, target_height = target
    return (
        round(rect[0] * target_width / source_width),
        round(rect[1] * target_height / source_height),
        round(rect[2] * target_width / source_width),
        round(rect[3] * target_height / source_height),
    )


def draw_dashed_line(draw: ImageDraw.ImageDraw, start: tuple[int, int], end: tuple[int, int], fill: tuple[int, int, int, int], width: int, dash: int) -> None:
    x1, y1 = start
    x2, y2 = end
    length = max(abs(x2 - x1), abs(y2 - y1))
    if length == 0:
        return
    for offset in range(0, length, dash * 2):
        next_offset = min(length, offset + dash)
        start_fraction = offset / length
        end_fraction = next_offset / length
        draw.line(
            (
                round(x1 + (x2 - x1) * start_fraction),
                round(y1 + (y2 - y1) * start_fraction),
                round(x1 + (x2 - x1) * end_fraction),
                round(y1 + (y2 - y1) * end_fraction),
            ),
            fill=fill,
            width=width,
        )


def draw_dashed_rect(draw: ImageDraw.ImageDraw, rect: tuple[int, int, int, int], fill: tuple[int, int, int, int], width: int, dash: int) -> None:
    x1, y1, x2, y2 = rect
    draw_dashed_line(draw, (x1, y1), (x2, y1), fill, width, dash)
    draw_dashed_line(draw, (x2, y1), (x2, y2), fill, width, dash)
    draw_dashed_line(draw, (x2, y2), (x1, y2), fill, width, dash)
    draw_dashed_line(draw, (x1, y2), (x1, y1), fill, width, dash)


def render_guide(layout: dict[str, Any], width: int, height: int, template_image: str | None = None) -> tuple[Image.Image, dict[str, Any]]:
    if template_image:
        return render_prime_context(layout, width, height, Path(template_image))
    image = Image.new("RGBA", (width, height), (0, 0, 0, 0))
    draw = ImageDraw.Draw(image)
    boundary = (96, 96, 96, 192)
    line_width = max(1, width // 900)
    dash = max(8, width // 90)
    source_width, source_height = layout_canvas_size(layout, (width, height))

    hard_regions = []
    for region in layout.get("hard_regions") or []:
        if not isinstance(region, dict):
            continue
        rect = int_rect(region)
        if rect is None:
            continue
        rect = scale_rect(rect, (source_width, source_height), (width, height))
        draw_dashed_rect(draw, rect, boundary, max(1, width // 720), dash)
        hard_regions.append({"id": region.get("id"), "rect": rect})

    frame = layout_safe_content_frame({**layout, "__canvas_width": source_width, "__canvas_height": source_height})
    if frame:
        rect = scale_rect((frame["x1"], frame["y1"], frame["x2"], frame["y2"]), (source_width, source_height), (width, height))
        frame_width = max(1, width // 540)
        draw_dashed_rect(draw, rect, (64, 64, 64, 144), frame_width, dash)

    evidence = {
        "width": width,
        "height": height,
        "size_key": layout.get("__size_key"),
        "source_canvas": {"width": source_width, "height": source_height},
        "hard_regions": hard_regions,
        "safe_content_frame": frame,
        "non_rendering_guide": True,
        "render_style": "transparent_neutral_outlines",
    }
    return image, evidence


def render_prime_context(layout: dict[str, Any], width: int, height: int, template_path: Path) -> tuple[Image.Image, dict[str, Any]]:
    if not template_path.is_file():
        raise SystemExit(f"official Prime template does not exist: {template_path}")
    try:
        with Image.open(template_path) as source:
            template = source.convert("RGBA").resize((width, height), Image.Resampling.LANCZOS)
    except OSError as exc:
        raise SystemExit(f"cannot read official Prime template: {exc}") from exc

    # Keep the real component color/material cues, but limit the visual context
    # to the reserved Prime areas so the model does not copy the full template.
    alpha = template.getchannel("A").point(lambda value: round(value * 0.58))
    hard_mask = Image.new("L", (width, height), 0)
    mask_draw = ImageDraw.Draw(hard_mask)
    source_width, source_height = layout_canvas_size(layout, (width, height))
    hard_regions = []
    for region in layout.get("hard_regions") or []:
        if not isinstance(region, dict):
            continue
        rect = int_rect(region)
        if rect is None:
            continue
        scaled = scale_rect(rect, (source_width, source_height), (width, height))
        mask_draw.rectangle(scaled, fill=255)
        hard_regions.append({"id": region.get("id"), "rect": scaled})
    if hard_regions:
        alpha = ImageChops.multiply(alpha, hard_mask)
    template.putalpha(alpha)
    image = Image.new("RGBA", (width, height), (0, 0, 0, 0))
    image.alpha_composite(template)
    frame = layout_safe_content_frame({**layout, "__canvas_width": source_width, "__canvas_height": source_height})
    evidence = {
        "width": width,
        "height": height,
        "size_key": layout.get("__size_key"),
        "source_canvas": {"width": source_width, "height": source_height},
        "hard_regions": hard_regions,
        "safe_content_frame": frame,
        "non_rendering_context": True,
        "render_style": "official_prime_visual_context",
        "template_image": template_path.name,
        "template_alpha": 0.58,
    }
    return image, evidence


def protected_content_envelope(layout: dict[str, Any], width: int, height: int) -> dict[str, list[int]]:
    source_width, source_height = layout_canvas_size(layout, (width, height))
    frame = layout_safe_content_frame({**layout, "__canvas_width": source_width, "__canvas_height": source_height})
    if frame is None:
        raise SystemExit("official Prime layout has no safe content frame for reflow guide")
    x1, y1, x2, y2 = scale_rect(
        (frame["x1"], frame["y1"], frame["x2"], frame["y2"]),
        (source_width, source_height),
        (width, height),
    )
    title = [x1, max(y1, round(height * 0.20)), x2, min(y2, round(height * 0.398))]
    table = [x1, max(y1, round(height * 0.574)), x2, min(y2 - round(height * 0.072), round(height * 0.796))]
    if title[2] <= title[0] or title[3] <= title[1] or table[2] <= table[0] or table[3] <= table[1]:
        raise SystemExit("official Prime layout has no room for conservative reflow guide")
    return {"title": title, "table": table}


def render_content_envelope_context(
    layout: dict[str, Any], width: int, height: int, template_path: Path,
) -> tuple[Image.Image, dict[str, Any]]:
    image, evidence = render_prime_context(layout, width, height, template_path)
    envelope = protected_content_envelope(layout, width, height)
    overlay = Image.new("RGBA", (width, height), (0, 0, 0, 0))
    draw = ImageDraw.Draw(overlay)
    line_width = max(2, round(width / 270))
    for rect, color in ((envelope["title"], (28, 174, 233, 210)), (envelope["table"], (16, 185, 129, 210))):
        draw.rectangle(rect, fill=(color[0], color[1], color[2], 42), outline=color, width=line_width)
    image.alpha_composite(overlay)
    evidence["content_envelope"] = envelope
    evidence["non_rendering_context"] = True
    evidence["render_style"] = "official_prime_reflow_context"
    return image, evidence


def main() -> int:
    args = parse_args()
    layouts = load_prime_layout(args.layout_file)
    layout = select_layout(layouts, args.size_key, args.width, args.height)
    width, height = target_size(layout, args.width, args.height)
    image, evidence = render_guide(layout, width, height, args.template_image)
    output_path = Path(args.output)
    output_path.parent.mkdir(parents=True, exist_ok=True)
    image.save(output_path, format="PNG")
    if args.content_envelope_output:
        if not args.template_image:
            raise SystemExit("content envelope guide requires --template-image")
        envelope_image, envelope_evidence = render_content_envelope_context(layout, width, height, Path(args.template_image))
        envelope_path = Path(args.content_envelope_output)
        envelope_path.parent.mkdir(parents=True, exist_ok=True)
        envelope_image.save(envelope_path, format="PNG")
        evidence["content_envelope"] = envelope_evidence["content_envelope"]
        evidence["content_envelope_render_style"] = envelope_evidence["render_style"]
    if args.evidence:
        evidence_path = Path(args.evidence)
        evidence_path.parent.mkdir(parents=True, exist_ok=True)
        evidence_path.write_text(json.dumps(evidence, ensure_ascii=False, indent=2), encoding="utf-8")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
