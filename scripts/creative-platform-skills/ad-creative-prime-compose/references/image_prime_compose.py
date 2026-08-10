#!/usr/bin/env python
"""Deterministically compose schema-v2 Prime components onto creative bodies."""

from __future__ import annotations

import argparse
import json
import re
from pathlib import Path
from typing import Any

import cv2
import numpy as np
import qrcode
from PIL import Image, ImageColor, ImageDraw, ImageFilter, ImageFont


SUPPORTED_KINDS = {"image", "text", "qr"}
SUPPORTED_QR_MODES = {"none", "static", "dynamic"}
PACKAGE_CONTRACT_VERSION = 1
SIZE_PATTERN = re.compile(r"^[1-9]\d*x[1-9]\d*$", re.IGNORECASE)


def render_qr(payload: str, box_size: int = 2) -> Image.Image:
    code = qrcode.QRCode(
        error_correction=qrcode.constants.ERROR_CORRECT_Q,
        box_size=box_size,
        border=1,
    )
    code.add_data(payload)
    code.make(fit=True)
    return code.make_image(fill_color="black", back_color="white").convert("RGBA")


def render_qr_for_rect(payload: str, width: int, height: int) -> Image.Image:
    unit = render_qr(payload, box_size=1)
    scale = min(width // unit.width, height // unit.height)
    if scale < 1:
        raise ValueError("dynamic QR payload cannot fit destination_rect")
    return unit.resize((unit.width * scale, unit.height * scale), Image.Resampling.NEAREST)


def decode_qr(rgb: np.ndarray, scale: int = 1) -> dict[str, Any]:
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


def qr_decode_evidence(final: np.ndarray, payload: str, rect: list[int]) -> dict[str, Any]:
    full_frame = decode_qr(final)
    full_frame_2x = decode_qr(final, scale=2)
    margin = 12
    left, top, right, bottom = rect
    crop_box = {
        "x": max(0, left - margin),
        "y": max(0, top - margin),
        "right": min(final.shape[1], right + margin),
        "bottom": min(final.shape[0], bottom + margin),
    }
    crop = final[crop_box["y"] : crop_box["bottom"], crop_box["x"] : crop_box["right"]]
    slot_crop = decode_qr(crop)
    slot_crop_2x = decode_qr(crop, scale=2)
    for evidence in (slot_crop, slot_crop_2x):
        if evidence["detected_points"] is not None:
            evidence["detected_points"] = [
                [point_x + crop_box["x"], point_y + crop_box["y"]]
                for point_x, point_y in evidence["detected_points"]
            ]
    attempts = {
        "full_frame_1x": full_frame,
        "full_frame_2x_nearest": full_frame_2x,
        "slot_crop_1x": {**slot_crop, "crop_box": crop_box},
        "slot_crop_2x_nearest": {**slot_crop_2x, "crop_box": crop_box},
    }
    successful_attempt = next(
        (name for name, evidence in attempts.items() if evidence["decoded"] == payload),
        None,
    )
    selected = attempts[successful_attempt] if successful_attempt else full_frame
    return {
        "decoded": selected["decoded"],
        "passed": successful_attempt is not None,
        "detected_points": selected["detected_points"],
        "successful_attempt": successful_attempt,
        "decode_attempts": attempts,
    }


def require_rect(value: object, field: str) -> list[int]:
    if not isinstance(value, list) or len(value) != 4 or not all(isinstance(item, int) for item in value):
        raise ValueError(f"{field} must contain four integers")
    left, top, right, bottom = value
    if left < 0 or top < 0 or right <= left or bottom <= top:
        raise ValueError(f"{field} must be a non-empty rectangle")
    return value


def require_layout_contract(layout: object, size: str, field: str) -> dict[str, Any]:
    if not isinstance(layout, dict):
        raise ValueError(f"{field} must be an object")
    width, height = (int(value) for value in size.lower().split("x", 1))
    hard_regions = layout.get("hard_regions")
    if not isinstance(hard_regions, list):
        raise ValueError(f"{field}.hard_regions must be an array")
    seen_ids: set[str] = set()
    for index, region in enumerate(hard_regions):
        if not isinstance(region, dict):
            raise ValueError(f"{field}.hard_regions[{index}] must be an object")
        region_id = str(region.get("id") or "").strip()
        if not region_id or region_id in seen_ids:
            raise ValueError(f"{field}.hard_regions ids must be non-empty and unique")
        seen_ids.add(region_id)
        rect = require_rect(
            [region.get(key) for key in ("x1", "y1", "x2", "y2")],
            f"{field}.hard_regions[{index}]",
        )
        if rect[2] > width or rect[3] > height:
            raise ValueError(f"{field}.hard_regions[{index}] exceeds {size}")
    top_end = layout.get("top_key_content_exclusion_end")
    bottom_start = layout.get("bottom_key_content_exclusion_start")
    if not isinstance(top_end, int) or isinstance(top_end, bool) or not 0 <= top_end <= height:
        raise ValueError(f"{field}.top_key_content_exclusion_end must be within {size}")
    if not isinstance(bottom_start, int) or isinstance(bottom_start, bool) or not 0 <= bottom_start <= height:
        raise ValueError(f"{field}.bottom_key_content_exclusion_start must be within {size}")
    return layout


def component_active(component: dict[str, Any], qr_mode: str) -> bool:
    return component.get("enabled") is True and (component.get("kind") != "qr" or qr_mode != "none")


def validate_composition(composition: dict[str, Any]) -> tuple[list[dict[str, Any]], str]:
    if composition.get("schema_version") != 2:
        raise ValueError("prime_composition.schema_version must be 2")
    qr_mode = str(composition.get("qr_mode") or "").lower()
    if qr_mode not in SUPPORTED_QR_MODES:
        raise ValueError("prime_composition.qr_mode must be none, static, or dynamic")
    components = composition.get("components")
    if not isinstance(components, list) or not components:
        raise ValueError("prime_composition.components must be a non-empty array")

    seen_ids: set[str] = set()
    active_qr_count = 0
    for index, component in enumerate(components):
        if not isinstance(component, dict):
            raise ValueError(f"prime_composition.components[{index}] must be an object")
        component_id = str(component.get("id") or "").strip()
        kind = str(component.get("kind") or "").strip()
        if not component_id or component_id in seen_ids:
            raise ValueError(f"component id must be non-empty and unique: {component_id!r}")
        seen_ids.add(component_id)
        if kind not in SUPPORTED_KINDS:
            raise ValueError(f"component {component_id!r} kind must be image, text, or qr")
        if not component_active(component, qr_mode):
            continue
        if kind == "image" and not str(component.get("source_role") or "").strip():
            raise ValueError(f"image component {component_id!r} requires source_role")
        if kind == "text" and not str(component.get("content") or "").strip():
            raise ValueError(f"text component {component_id!r} requires content")
        if kind == "qr":
            active_qr_count += 1
            if qr_mode == "static" and not str(component.get("source_role") or "").strip():
                raise ValueError(f"static QR component {component_id!r} requires source_role")
    if active_qr_count > 1:
        raise ValueError("prime_composition can enable at most one QR component")
    if qr_mode in {"static", "dynamic"} and active_qr_count != 1:
        raise ValueError(f"{qr_mode} QR mode requires exactly one enabled QR component")
    return components, qr_mode


def compile_layout_contract(
    components: list[dict[str, Any]], layout: dict[str, Any], qr_mode: str, canvas_size: tuple[int, int]
) -> dict[str, Any]:
    hard_regions: list[dict[str, Any]] = []
    top_end = 0
    bottom_start = canvas_size[1]
    placements = layout.get("components") or {}
    for component in components:
        if not component_active(component, qr_mode):
            continue
        destination = require_rect(
            (placements.get(component["id"]) or {}).get("destination_rect"),
            f"{component['id']}.destination_rect",
        )
        region = {
            "id": component["id"],
            "kind": component["kind"],
            "x1": destination[0],
            "y1": destination[1],
            "x2": destination[2],
            "y2": destination[3],
        }
        backdrop_rule = str(component.get("backdrop_rule") or "").strip()
        if backdrop_rule:
            region["backdrop_rule"] = backdrop_rule
        hard_regions.append(region)
        if destination[1] < canvas_size[1] // 2:
            top_end = max(top_end, destination[3])
        if destination[3] > canvas_size[1] // 2:
            bottom_start = min(bottom_start, destination[1])
    return {
        "hard_regions": hard_regions,
        "top_key_content_exclusion_end": top_end,
        "bottom_key_content_exclusion_start": bottom_start,
    }


def component_backdrop_rule(component: dict[str, Any], placement: dict[str, Any]) -> str:
    return str(placement.get("backdrop_rule") or component.get("backdrop_rule") or "").strip().lower()


def component_declares_backdrop(component: dict[str, Any], placement: dict[str, Any]) -> bool:
    return component_backdrop_rule(component, placement) not in {"", "none", "transparent"}


def prepare_fixed_prime_backdrop(
    canvas: Image.Image, destination: list[int], kind: str, rule: str
) -> dict[str, Any] | None:
    """Quiet the fixed Prime footprint before compositing the frozen component.

    The coordinates still come only from the market pack. This step prevents
    model-generated text, logos or busy texture inside a fixed Prime slot from
    bleeding through transparent logo/store/terms assets.
    """
    left, top, right, bottom = destination
    width = right - left
    height = bottom - top
    if width < 1 or height < 1:
        return None

    normalized_rule = rule or "none"
    if normalized_rule in {"none", "transparent"}:
        return None
    crop = canvas.crop((left, top, right, bottom)).convert("RGBA")
    blur_radius = max(2, min(width, height) // 8)
    softened = crop.filter(ImageFilter.GaussianBlur(radius=blur_radius))

    if kind == "qr" or "light" in normalized_rule:
        overlay_alpha = 238 if kind == "qr" else 196
        mode = "fixed_light_backdrop"
    elif kind == "text" or "quiet" in normalized_rule or "low_texture" in normalized_rule:
        overlay_alpha = 174
        mode = "fixed_quiet_backdrop"
    else:
        overlay_alpha = 108
        mode = "fixed_soft_backdrop"

    repaired = Image.alpha_composite(softened, Image.new("RGBA", (width, height), (255, 255, 255, overlay_alpha)))
    canvas.paste(repaired, (left, top))
    return {
        "mode": mode,
        "rule": normalized_rule,
        "rect": destination,
        "blur_radius": blur_radius,
        "overlay": {"color": "#ffffff", "alpha": overlay_alpha},
    }


def prime_slot_clearance(image: Image.Image, destination: list[int]) -> dict[str, Any]:
    """Record whether the generated base leaves a fixed Prime slot visually quiet."""
    left, top, right, bottom = destination
    crop = image.crop((left, top, right, bottom)).convert("RGB")
    array = np.asarray(crop)
    if array.size == 0 or crop.width < 8 or crop.height < 8:
        return {"status": "passed", "edge_density": 0.0, "threshold": 0.055}
    gray = cv2.cvtColor(array, cv2.COLOR_RGB2GRAY)
    blurred = cv2.GaussianBlur(gray, (3, 3), 0)
    edges = cv2.Canny(blurred, 56, 144)
    edge_density = float(np.count_nonzero(edges) / edges.size)
    rgb_std = float(np.mean(np.std(array.astype(np.float32), axis=(0, 1))))
    edge = max(1, min(6, crop.width // 20, crop.height // 20))
    edge_pixels = np.concatenate((
        array[:edge, :, :].reshape(-1, 3),
        array[-edge:, :, :].reshape(-1, 3),
        array[:, :edge, :].reshape(-1, 3),
        array[:, -edge:, :].reshape(-1, 3),
    ))
    background_rgb = np.median(edge_pixels, axis=0)
    distance = np.linalg.norm(array.astype(np.float32) - background_rgb.astype(np.float32), axis=2)
    foreground_ratio = float(np.count_nonzero(distance > 72.0) / distance.size)
    edge_threshold = 0.055
    foreground_threshold = 0.075
    passed = edge_density <= edge_threshold and foreground_ratio <= foreground_threshold
    return {
        "status": "passed" if passed else "warning",
        "edge_density": round(edge_density, 6),
        "rgb_std": round(rgb_std, 3),
        "foreground_ratio": round(foreground_ratio, 6),
        "background_rgb": [int(round(value)) for value in background_rgb.tolist()],
        "edge_threshold": edge_threshold,
        "foreground_threshold": foreground_threshold,
    }


def transparentize_edge_background(source: Image.Image) -> tuple[Image.Image, dict[str, Any] | None]:
    """Remove a flat component background that is connected to the source edge.

    Market-pack components sometimes arrive as logo/footer/terms strips with an
    opaque rectangular background. The layout rectangle remains fixed; this
    only converts edge-connected background pixels to alpha so the normalized
    Prime backdrop shows through instead of a visible sticker block.
    """
    rgba = source.convert("RGBA")
    array = np.asarray(rgba).copy()
    height, width = array.shape[:2]
    if width < 4 or height < 4:
        return rgba, None

    alpha = array[:, :, 3]
    visible = alpha > 0
    if not np.any(visible):
        return rgba, None
    transparent_ratio = float(np.count_nonzero(alpha == 0) / alpha.size)
    if transparent_ratio > 0.02:
        return rgba, None

    edge = max(1, min(8, width // 20, height // 20))
    edge_mask = np.zeros((height, width), dtype=bool)
    edge_mask[:edge, :] = True
    edge_mask[-edge:, :] = True
    edge_mask[:, :edge] = True
    edge_mask[:, -edge:] = True
    edge_pixels = array[:, :, :3][edge_mask & visible]
    if len(edge_pixels) < 8:
        return rgba, None

    background_rgb = np.median(edge_pixels, axis=0)
    distance = np.linalg.norm(array[:, :, :3].astype(np.float32) - background_rgb.astype(np.float32), axis=2)
    threshold = 52.0
    similar = (distance <= threshold) & visible
    component_count, labels = cv2.connectedComponents(similar.astype(np.uint8), 8)
    if component_count <= 1:
        return rgba, None

    border_labels = np.unique(labels[edge_mask])
    border_labels = border_labels[border_labels != 0]
    if len(border_labels) == 0:
        return rgba, None

    removable = np.isin(labels, border_labels)
    removable_ratio = float(np.count_nonzero(removable) / np.count_nonzero(visible))
    if removable_ratio < 0.04 or removable_ratio > 0.985:
        return rgba, None

    array[:, :, 3] = np.where(removable, 0, alpha)
    return Image.fromarray(array, "RGBA"), {
        "mode": "edge_background_to_alpha",
        "background_rgb": [int(round(value)) for value in background_rgb.tolist()],
        "threshold": threshold,
        "transparent_pixel_ratio": round(removable_ratio, 4),
    }


def contain_layer(source: Image.Image, width: int, height: int, *, qr: bool = False) -> tuple[Image.Image, list[int]]:
    if source.width < 1 or source.height < 1:
        raise ValueError("component source image is empty")
    scale = min(width / source.width, height / source.height)
    target_width = max(1, min(width, round(source.width * scale)))
    target_height = max(1, min(height, round(source.height * scale)))
    resample = Image.Resampling.NEAREST if qr else Image.Resampling.LANCZOS
    rgba = source.convert("RGBA")
    resized = rgba.resize((target_width, target_height), resample)
    if qr:
        opaque_resized = Image.new("RGBA", resized.size, "white")
        opaque_resized.alpha_composite(resized)
        resized = opaque_resized
        max_inset = min(8, width - 1, height - 1)
        for inset in range(max_inset + 1):
            candidate_scale = min((width - inset) / source.width, (height - inset) / source.height)
            candidate_width = max(1, min(width, round(source.width * candidate_scale)))
            candidate_height = max(1, min(height, round(source.height * candidate_scale)))
            candidate = rgba.resize((candidate_width, candidate_height), Image.Resampling.NEAREST)
            opaque = Image.new("RGBA", candidate.size, "white")
            opaque.alpha_composite(candidate)
            rgb = np.asarray(opaque.convert("RGB"))
            if decode_qr(rgb)["decoded"] or decode_qr(rgb, scale=2)["decoded"]:
                resized = opaque
                target_width, target_height = candidate_width, candidate_height
                break
    x = (width - target_width) // 2
    y = (height - target_height) // 2
    return resized, [x, y, x + target_width, y + target_height]


def load_font(size: int) -> ImageFont.ImageFont:
    for font_name in ("DejaVuSans.ttf", "Arial.ttf"):
        try:
            return ImageFont.truetype(font_name, size=size)
        except OSError:
            continue
    return ImageFont.load_default()


def wrap_text(draw: ImageDraw.ImageDraw, content: str, font: ImageFont.ImageFont, max_width: int) -> str:
    def fits(value: str) -> bool:
        box = draw.textbbox((0, 0), value or " ", font=font)
        return box[2] - box[0] <= max_width

    wrapped: list[str] = []
    for paragraph in content.splitlines() or [""]:
        if not paragraph:
            wrapped.append("")
            continue
        words = paragraph.split(" ")
        current = ""
        for word in words:
            candidate = word if not current else f"{current} {word}"
            if fits(candidate):
                current = candidate
                continue
            if current:
                wrapped.append(current)
                current = ""
            if fits(word):
                current = word
                continue
            fragment = ""
            for character in word:
                candidate = f"{fragment}{character}"
                if fragment and not fits(candidate):
                    wrapped.append(fragment)
                    fragment = character
                else:
                    fragment = candidate
            current = fragment
        wrapped.append(current)
    return "\n".join(wrapped)


def render_text_component(content: str, width: int, height: int, style: object) -> tuple[Image.Image, dict[str, Any]]:
    values = style if isinstance(style, dict) else {}
    padding = values.get("padding", 0)
    if not isinstance(padding, int) or padding < 0:
        raise ValueError("text style.padding must be a non-negative integer")
    max_width = width - 2 * padding
    max_height = height - 2 * padding
    if max_width < 1 or max_height < 1:
        raise ValueError("text padding leaves no renderable area")
    requested_size = values.get("font_size")
    if requested_size is not None and (not isinstance(requested_size, int) or requested_size < 6):
        raise ValueError("text style.font_size must be an integer of at least 6")
    start_size = requested_size or min(48, max(12, height // 3))
    spacing = values.get("line_spacing", 4)
    if not isinstance(spacing, int) or spacing < 0:
        raise ValueError("text style.line_spacing must be a non-negative integer")
    align = str(values.get("align") or "center").lower()
    valign = str(values.get("valign") or "middle").lower()
    if align not in {"left", "center", "right"}:
        raise ValueError("text style.align must be left, center, or right")
    if valign not in {"top", "middle", "bottom"}:
        raise ValueError("text style.valign must be top, middle, or bottom")
    try:
        color = ImageColor.getcolor(str(values.get("color") or "#111111"), "RGBA")
    except ValueError as error:
        raise ValueError("text style.color must be a valid Pillow color") from error

    measure_layer = Image.new("RGBA", (width, height), (0, 0, 0, 0))
    measure = ImageDraw.Draw(measure_layer)
    selected: tuple[ImageFont.ImageFont, str, tuple[int, int, int, int], int] | None = None
    for font_size in range(start_size, 5, -1):
        font = load_font(font_size)
        wrapped = wrap_text(measure, content, font, max_width)
        bbox = measure.multiline_textbbox((0, 0), wrapped, font=font, spacing=spacing, align=align)
        if bbox[2] - bbox[0] <= max_width and bbox[3] - bbox[1] <= max_height:
            selected = (font, wrapped, bbox, font_size)
            break
    if selected is None:
        raise ValueError("text content cannot fit destination_rect")

    font, wrapped, bbox, font_size = selected
    text_width = bbox[2] - bbox[0]
    text_height = bbox[3] - bbox[1]
    x = padding if align == "left" else width - padding - text_width if align == "right" else (width - text_width) // 2
    y = padding if valign == "top" else height - padding - text_height if valign == "bottom" else (height - text_height) // 2
    layer = Image.new("RGBA", (width, height), (0, 0, 0, 0))
    draw = ImageDraw.Draw(layer)
    draw.multiline_text((x - bbox[0], y - bbox[1]), wrapped, font=font, fill=color, spacing=spacing, align=align)
    return layer, {
        "rendered_rect": [x, y, x + text_width, y + text_height],
        "font_size": font_size,
        "align": align,
        "valign": valign,
        "color": str(values.get("color") or "#111111"),
    }


def compose_v2(
    input_path: Path,
    output_path: Path,
    composition: dict[str, Any],
    sources: dict[str, Path],
    payload: str,
) -> dict[str, Any]:
    components, qr_mode = validate_composition(composition)
    if qr_mode in {"static", "dynamic"} and not payload:
        raise ValueError(f"{qr_mode} QR mode requires qr_validation.approved_payload")

    with Image.open(input_path) as input_image:
        body = input_image.convert("RGBA")
    size = f"{body.width}x{body.height}"
    layout = (composition.get("layouts") or {}).get(size)
    if not isinstance(layout, dict):
        raise ValueError(f"prime_composition does not define the {size} layout")
    placements = layout.get("components")
    if not isinstance(placements, dict):
        raise ValueError(f"prime_composition {size} layout components must be an object")

    canvas = body.copy()
    component_evidence: list[dict[str, Any]] = []
    qr_box: list[int] | None = None
    opened_sources: dict[str, Image.Image] = {}
    try:
        for component in components:
            if not component_active(component, qr_mode):
                continue
            component_id = str(component["id"])
            kind = str(component["kind"])
            placement = placements.get(component_id)
            if not isinstance(placement, dict):
                raise ValueError(f"{size} layout is missing enabled component {component_id!r}")
            destination = require_rect(placement.get("destination_rect"), f"{component_id}.destination_rect")
            if destination[2] > body.width or destination[3] > body.height:
                raise ValueError(f"{component_id}.destination_rect exceeds {size}")
            width = destination[2] - destination[0]
            height = destination[3] - destination[1]
            evidence: dict[str, Any] = {
                "id": component_id,
                "kind": kind,
                "destination_rect": destination,
            }
            clearance = prime_slot_clearance(body, destination)
            evidence["body_clearance"] = clearance
            backdrop_rule = component_backdrop_rule(component, placement)
            if component_declares_backdrop(component, placement):
                backdrop = prepare_fixed_prime_backdrop(canvas, destination, kind, backdrop_rule)
                if backdrop is not None:
                    evidence["fixed_backdrop"] = backdrop

            if kind == "text":
                content = str(component.get("content") or "")
                layer, text_evidence = render_text_component(content, width, height, placement.get("style"))
                canvas.alpha_composite(layer, (destination[0], destination[1]))
                local_rect = text_evidence.pop("rendered_rect")
                evidence.update(text_evidence)
                evidence["content"] = content
                evidence["rendered_rect"] = [
                    destination[0] + local_rect[0],
                    destination[1] + local_rect[1],
                    destination[0] + local_rect[2],
                    destination[1] + local_rect[3],
                ]
                component_evidence.append(evidence)
                continue

            if kind == "qr" and qr_mode == "dynamic":
                layer = render_qr_for_rect(payload, width, height)
                local_rect = [
                    (width - layer.width) // 2,
                    (height - layer.height) // 2,
                    (width + layer.width) // 2,
                    (height + layer.height) // 2,
                ]
                canvas.alpha_composite(layer, (destination[0] + local_rect[0], destination[1] + local_rect[1]))
                qr_box = [
                    destination[0] + local_rect[0],
                    destination[1] + local_rect[1],
                    destination[0] + local_rect[2],
                    destination[1] + local_rect[3],
                ]
                evidence.update({"source_role": "", "rendered_rect": qr_box})
                component_evidence.append(evidence)
                continue

            source_role = str(component.get("source_role") or "").strip()
            source_path = sources.get(source_role)
            if source_path is None:
                raise ValueError(f"component {component_id!r} requires source role {source_role!r}")
            if source_role not in opened_sources:
                opened_sources[source_role] = Image.open(source_path).convert("RGBA")
            source_image = opened_sources[source_role]
            is_static_qr = kind == "qr" and qr_mode == "static"
            component_source = source_image
            alpha_key_evidence = None
            if not is_static_qr:
                component_source, alpha_key_evidence = transparentize_edge_background(source_image)
            layer, local_rect = contain_layer(component_source, width, height, qr=is_static_qr)
            rendered_rect = [
                destination[0] + local_rect[0],
                destination[1] + local_rect[1],
                destination[0] + local_rect[2],
                destination[1] + local_rect[3],
            ]
            canvas.alpha_composite(layer, (rendered_rect[0], rendered_rect[1]))
            evidence.update({
                "source_role": source_role,
                "source_size": {"width": source_image.width, "height": source_image.height},
                "fit": "contain",
                "rendered_rect": rendered_rect,
            })
            if alpha_key_evidence is not None:
                evidence["alpha_key"] = alpha_key_evidence
            component_evidence.append(evidence)
            if is_static_qr:
                qr_box = rendered_rect
    finally:
        for source_image in opened_sources.values():
            source_image.close()

    output_path.parent.mkdir(parents=True, exist_ok=True)
    canvas.convert("RGB").save(output_path, "PNG", optimize=True)
    result: dict[str, Any] = {
        "output": str(output_path),
        "canvas": {"width": body.width, "height": body.height},
        "template_kind": "prime_components_v2",
        "qr_mode": qr_mode,
        "qr_payload": payload if qr_mode in {"static", "dynamic"} else "",
        "qr": {"rendered_rect": qr_box},
        "components": component_evidence,
        "layout_contract": compile_layout_contract(components, layout, qr_mode, body.size),
        "decoded": "",
        "passed": True,
        "detected_points": None,
        "successful_attempt": None,
        "decode_attempts": {},
    }
    if qr_mode in {"static", "dynamic"}:
        if qr_box is None:
            raise ValueError(f"{qr_mode} QR component did not produce a rendered rectangle")
        final = np.asarray(Image.open(output_path).convert("RGB"))
        evidence = qr_decode_evidence(final, payload, qr_box)
        result.update(evidence)
        if not result["passed"]:
            raise RuntimeError(json.dumps(result, ensure_ascii=False))
    return result


def resolve_manifest_path(base_dir: Path, value: str) -> Path:
    path = Path(value)
    return path if path.is_absolute() else base_dir / path


def resolve_sources(base_dir: Path, manifest: dict[str, Any]) -> dict[str, Path]:
    raw_sources = manifest.get("sources") or {}
    if not isinstance(raw_sources, dict):
        raise ValueError("manifest sources must map resource roles to files")
    return {
        str(role): resolve_manifest_path(base_dir, str(value))
        for role, value in raw_sources.items()
        if str(role).strip() and str(value).strip()
    }


def validated_qr_payload(composition: dict[str, Any], manifest: dict[str, Any], job: dict[str, Any]) -> str:
    qr_mode = str(composition.get("qr_mode") or "").lower()
    validation = manifest.get("qr_validation") or {}
    if not isinstance(validation, dict):
        raise ValueError("manifest qr_validation must be an object")
    if validation.get("status") != "passed":
        raise ValueError("qr_validation.status must be passed")
    validation_mode = str(validation.get("mode") or "").lower()
    if validation_mode != qr_mode:
        raise ValueError("qr_validation.mode must match prime_composition.qr_mode")
    payload = str(validation.get("approved_payload") or "")
    if qr_mode == "none":
        if payload or str(job.get("qr_payload") or ""):
            raise ValueError("none QR mode cannot declare an approved payload")
        return ""
    if not payload:
        raise ValueError(f"{qr_mode} QR mode requires qr_validation.approved_payload")
    job_payload = str(job.get("qr_payload") or "")
    if job_payload and job_payload != payload:
        raise ValueError("job qr_payload must match qr_validation.approved_payload")
    return payload


def validate_package_manifest(manifest: dict[str, Any]) -> tuple[list[dict[str, Any]], dict[str, Any]]:
    if manifest.get("package_contract_version") != PACKAGE_CONTRACT_VERSION:
        raise ValueError(f"package_contract_version must be {PACKAGE_CONTRACT_VERSION}")
    variant_id = str(manifest.get("variant_id") or "").strip()
    variant_key = str(manifest.get("variant_key") or "").strip()
    revision = manifest.get("revision")
    if not variant_id or not variant_key:
        raise ValueError("manifest variant_id and variant_key are required")
    if not isinstance(revision, int) or isinstance(revision, bool) or revision < 1:
        raise ValueError("manifest revision must be a positive integer")

    expected_sizes = manifest.get("expected_sizes")
    if not isinstance(expected_sizes, list) or not 1 <= len(expected_sizes) <= 3:
        raise ValueError("manifest expected_sizes must contain between 1 and 3 sizes")
    if any(not isinstance(size, str) or not SIZE_PATTERN.fullmatch(size) for size in expected_sizes):
        raise ValueError("manifest expected_sizes must use WIDTHxHEIGHT strings")
    if len(set(expected_sizes)) != len(expected_sizes):
        raise ValueError("manifest expected_sizes must be unique")

    composition = manifest.get("prime_composition")
    if not isinstance(composition, dict):
        raise ValueError("schema-v2 prime_composition is required; full-template legacy mode is unsupported")
    components, qr_mode = validate_composition(composition)
    contract = manifest.get("prime_layout_contract")
    layouts = contract.get("layouts") if isinstance(contract, dict) else None
    if not isinstance(layouts, dict):
        raise ValueError("manifest prime_layout_contract.layouts is required")

    jobs = manifest.get("jobs")
    if not isinstance(jobs, list) or len(jobs) != len(expected_sizes):
        raise ValueError("manifest jobs must contain exactly one job per expected size")
    seen_ids: set[str] = set()
    seen_sizes: set[str] = set()
    seen_outputs: set[str] = set()
    for index, job in enumerate(jobs):
        field = f"manifest jobs[{index}]"
        if not isinstance(job, dict):
            raise ValueError(f"{field} must be an object")
        job_id = str(job.get("id") or "").strip()
        size = str(job.get("size") or "").strip()
        if not job_id or job_id in seen_ids:
            raise ValueError("manifest job ids must be non-empty and unique")
        if size not in expected_sizes or size in seen_sizes:
            raise ValueError("manifest jobs must cover each expected size exactly once")
        if job.get("variant_id") != variant_id or job.get("variant_key") != variant_key:
            raise ValueError(f"{field} variant identity must match the manifest")
        if job.get("revision") != revision:
            raise ValueError(f"{field}.revision must match the manifest")
        if not str(job.get("input") or "").strip() or not str(job.get("output") or "").strip():
            raise ValueError(f"{field} input and output are required")
        output = str(job["output"])
        if output in seen_outputs:
            raise ValueError("manifest job output paths must be unique")

        composition_layout = (composition.get("layouts") or {}).get(size)
        if not isinstance(composition_layout, dict):
            raise ValueError(f"prime_composition does not define the {size} layout")
        width, height = (int(value) for value in size.lower().split("x", 1))
        compiled_layout = compile_layout_contract(components, composition_layout, qr_mode, (width, height))
        published_layout = require_layout_contract(layouts.get(size), size, f"prime_layout_contract.layouts.{size}")
        job_layout = job.get("layout_contract")
        if job_layout is not None:
            job_layout = require_layout_contract(job_layout, size, f"{field}.layout_contract")
        if published_layout != compiled_layout or (job_layout is not None and job_layout != compiled_layout):
            raise ValueError(f"{field} layout contract must equal the compiled published contract for {size}")

        seen_ids.add(job_id)
        seen_sizes.add(size)
        seen_outputs.add(output)
    if seen_sizes != set(expected_sizes):
        raise ValueError("manifest jobs do not match expected_sizes")
    return jobs, composition


def compose_manifest(manifest_path: Path) -> dict[str, Any]:
    manifest = json.loads(manifest_path.read_text(encoding="utf-8-sig"))
    try:
        jobs, composition = validate_package_manifest(manifest)
    except Exception as error:
        raw_jobs = manifest.get("jobs")
        return {
            "package_contract_version": manifest.get("package_contract_version"),
            "package_contract_valid": False,
            "variant_id": manifest.get("variant_id"),
            "variant_key": manifest.get("variant_key"),
            "revision": manifest.get("revision"),
            "expected_sizes": manifest.get("expected_sizes"),
            "succeeded": 0,
            "failed": max(1, len(raw_jobs) if isinstance(raw_jobs, list) else 1),
            "package_error": str(error),
            "results": [],
        }
    base_dir = manifest_path.resolve().parent
    results = []
    for job in jobs:
        job_id = str(job["id"])
        output_path = resolve_manifest_path(base_dir, str(job["output"])).resolve()
        try:
            if job.get("prime_composition") is not None or job.get("sources") is not None:
                raise ValueError("prime_composition and sources must be shared at manifest level")
            result = compose_v2(
                resolve_manifest_path(base_dir, str(job["input"])),
                output_path,
                composition,
                resolve_sources(base_dir, manifest),
                validated_qr_payload(composition, manifest, job),
            )
            expected_size = str(job["size"])
            actual_size = f"{result['canvas']['width']}x{result['canvas']['height']}"
            if actual_size != expected_size:
                raise ValueError(f"job {job_id!r} input canvas {actual_size} does not match declared size {expected_size}")
            published_layout = manifest["prime_layout_contract"]["layouts"][expected_size]
            if result["layout_contract"] != published_layout:
                raise ValueError(f"job {job_id!r} compose layout contract does not match its manifest contract")
            results.append({
                "id": job_id,
                "variant_id": manifest["variant_id"],
                "variant_key": manifest["variant_key"],
                "size": expected_size,
                "revision": manifest["revision"],
                "input": str(job["input"]),
                "status": "succeeded",
                **result,
            })
        except Exception as error:
            results.append({
                "id": job_id,
                "variant_id": manifest["variant_id"],
                "variant_key": manifest["variant_key"],
                "size": job["size"],
                "revision": manifest["revision"],
                "status": "failed",
                "error": str(error),
            })
    failed = sum(result["status"] == "failed" for result in results)
    return {
        "package_contract_version": PACKAGE_CONTRACT_VERSION,
        "package_contract_valid": True,
        "variant_id": manifest["variant_id"],
        "variant_key": manifest["variant_key"],
        "revision": manifest["revision"],
        "expected_sizes": manifest["expected_sizes"],
        "succeeded": len(results) - failed,
        "failed": failed,
        "results": results,
    }


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--manifest", required=True)
    args = parser.parse_args()
    result = compose_manifest(Path(args.manifest))
    print(json.dumps(result, ensure_ascii=False))
    if result["failed"]:
        raise SystemExit(1)


if __name__ == "__main__":
    main()
