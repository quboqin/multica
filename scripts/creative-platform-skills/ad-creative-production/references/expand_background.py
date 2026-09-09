#!/usr/bin/env python3
"""Fit an existing unbranded design inside Prime's safe frame; edit only its surround.

No text is rendered here. The final compose restores the saved body even if the
image model ignores its mask. Run only for an explicitly assigned rework fallback.
"""
from __future__ import annotations

import argparse
import hashlib
import json
from pathlib import Path

from PIL import Image, ImageDraw

from render_prime_guide import layout_canvas_size, scale_rect, select_layout
from validate_copy_snapshot import layout_safe_content_frame, load_prime_layout

RESAMPLING = getattr(Image, "Resampling", Image)


def pixel_hash(image: Image.Image) -> str:
    return hashlib.sha256(image.convert("RGB").tobytes()).hexdigest()


def prepare(source: Path, layout: dict, size: tuple[int, int], directory: Path) -> dict:
    width, height = size
    if width <= 0 or height <= 0 or width % 16 or height % 16:
        raise ValueError("model canvas dimensions must be positive multiples of 16")
    source_canvas = layout_canvas_size(layout, size)
    frame = layout_safe_content_frame({**layout, "__canvas_width": source_canvas[0], "__canvas_height": source_canvas[1]})
    if not frame:
        raise ValueError("official layout must declare a safe content frame")
    x1, y1, x2, y2 = scale_rect(tuple(frame[key] for key in ("x1", "y1", "x2", "y2")), source_canvas, size)
    if not (0 <= x1 < x2 <= width and 0 <= y1 < y2 <= height):
        raise ValueError("safe content frame must lie inside the canvas")
    with Image.open(source) as image:
        original = image.convert("RGB")
    # Fit the whole source, with no subjective crop that could lose approved copy.
    scale = min((x2 - x1) / original.width, (y2 - y1) / original.height, 1.0)
    if scale < 0.65:
        raise ValueError("background expansion would shrink the body too far; require manual layout review")
    body = original.resize((max(1, int(original.width * scale)), max(1, int(original.height * scale))), RESAMPLING.LANCZOS)
    left, top = x1 + (x2 - x1 - body.width) // 2, y1 + (y2 - y1 - body.height) // 2
    box = [left, top, left + body.width, top + body.height]
    # A neutral field is an editable input, never a fallback final background.
    canvas = Image.new("RGB", size, original.getpixel((0, 0)))
    canvas.paste(body, (left, top))
    mask = Image.new("RGBA", size, (0, 0, 0, 0))
    ImageDraw.Draw(mask).rectangle((left, top, box[2] - 1, box[3] - 1), fill=(255, 255, 255, 255))
    directory.mkdir(parents=True, exist_ok=True)
    body.save(directory / "body.png")
    canvas.save(directory / "input.png")
    mask.save(directory / "mask.png")
    evidence = {"strategy": "background_expansion", "canvas": [width, height], "safe_content_frame": [x1, y1, x2, y2],
                "body_box": box, "source_pixel_sha256": pixel_hash(original), "body_pixel_sha256": pixel_hash(body),
                "scale": scale, "text_rendered_by_script": False}
    (directory / "evidence.json").write_text(json.dumps(evidence, indent=2), encoding="utf-8")
    return evidence


def compose(directory: Path, generated_background: Path, output: Path) -> dict:
    evidence = json.loads((directory / "evidence.json").read_text(encoding="utf-8"))
    size = tuple(evidence["canvas"])
    box = tuple(evidence["body_box"])
    with Image.open(directory / "body.png") as image:
        body = image.convert("RGB")
    if pixel_hash(body) != evidence["body_pixel_sha256"] or body.size != (box[2] - box[0], box[3] - box[1]):
        raise ValueError("protected body changed after preparation")
    with Image.open(generated_background) as image:
        background = image.convert("RGB")
        # Providers may return a nearby canvas. Normalize background before restoring text.
        background = background.resize(size, RESAMPLING.LANCZOS)
    background.paste(body, (box[0], box[1]))
    if pixel_hash(background.crop(box)) != evidence["body_pixel_sha256"]:
        raise ValueError("protected body was not preserved")
    output.parent.mkdir(parents=True, exist_ok=True)
    background.save(output, "PNG")
    evidence.update({"body_preserved": True, "composed_pixel_sha256": pixel_hash(background)})
    output.with_suffix(".json").write_text(json.dumps(evidence, indent=2), encoding="utf-8")
    return evidence


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest="command", required=True)
    prep = commands.add_parser("prepare")
    prep.add_argument("--source", type=Path, required=True)
    prep.add_argument("--layout-file", required=True)
    prep.add_argument("--size-key", required=True)
    prep.add_argument("--width", type=int, required=True)
    prep.add_argument("--height", type=int, required=True)
    prep.add_argument("--directory", type=Path, required=True)
    final = commands.add_parser("compose")
    final.add_argument("--directory", type=Path, required=True)
    final.add_argument("--background", type=Path, required=True)
    final.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    if args.command == "prepare":
        layout = select_layout(load_prime_layout(args.layout_file), args.size_key, None, None)
        result = prepare(args.source, layout, (args.width, args.height), args.directory)
    else:
        result = compose(args.directory, args.background, args.output)
    print(json.dumps(result))


if __name__ == "__main__":
    main()
