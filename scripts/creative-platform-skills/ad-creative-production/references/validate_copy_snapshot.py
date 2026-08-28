#!/usr/bin/env python3
"""Reject financial tokens that are absent from the selected copy snapshot."""

from __future__ import annotations

import argparse
import json
import re
import sys
from pathlib import Path
from typing import Any


CREATIVE_TYPES = {"num", "repayment_plan"}
CURRENCY_PATTERN = re.compile(r"\b(?:Rp\.?|IDR)\s*(\d+(?:[.,]\d+)*)", re.IGNORECASE)
PERCENT_PATTERN = re.compile(r"\b(\d+(?:[.,]\d+)?)\s*%")
TERM_PATTERN = re.compile(r"\b(\d+(?:\s*-\s*\d+)?)\s*(bulan|hari|tahun)\b", re.IGNORECASE)
FINANCIAL_NUMBER_PATTERN = re.compile(
    r"\b(?:limit|pinjaman|dana|jumlah|cicilan|angsuran|tenor|bunga|interest|biaya|fee)"
    r"\D{0,24}(\d{1,3}(?:[.,]\d{3})+|\d+)(?:\s*(?:juta|ribu|miliar))?",
    re.IGNORECASE,
)
PRIME_GUARD_TERMS = {
    "unbranded_base": ("unbranded", "brand-free", "no brand", "no brands", "no competitor material", "无品牌"),
    "prime_overlay": ("prime", "overlay", "贴片", "合成"),
    "prime_guide": ("input 2", "reserved-area guide", "layout guide", "avoidance guide", "占位参考"),
    "neutral_guide": ("neutral monochrome", "neutral gray", "neutral grey", "geometry map", "中性灰"),
    "guide_no_draw": ("do not draw input 2", "do not copy input 2", "do not render the guide", "no guide blocks", "不渲染占位"),
    "no_logo": ("logo", "标识"),
    "no_qr": ("qr", "二维码"),
    "no_footer": ("footer", "legal", "regulatory", "页脚", "法律", "监管"),
    "hard_region": ("hard region", "hard_regions", "overlay region", "fixed region", "硬区", "避让"),
    "natural_background": ("background", "low-detail", "low texture", "低纹理", "自然背景"),
}
SEMANTIC_PRIME_GUARD_TERMS = {
    "unbranded_base": ("unbranded base", "unbranded visual base", "unbranded"),
    "prime_overlay": ("deterministic official prime composition", "official overlay", "official prime overlay", "official prime"),
    "prime_guide": ("input 2 is the current-size official prime visual context",),
    "guide_no_draw": (
        "never draw prime",
        "do not copy any prime",
        "prime is visual context only",
        "official overlay remains readable",
    ),
    "no_logo": ("logo", "标识"),
    "no_qr": ("qr", "二维码"),
    "no_footer": ("footer", "legal text", "regulatory", "法律", "监管"),
    "protected_bands": ("protected prime bands", "protected top and bottom bands", "future component areas", "future prime component areas"),
    "business_avoidance": (
        "all business content in the middle content area between the protected prime bands",
        "no business content enters the protected bands",
        "keep every module and keep it out of the protected bands",
        "keep its future component areas clear of business content",
    ),
    "natural_background": ("background", "continuous background", "low-detail"),
}
SEMANTIC_MODEL_INTEGRATED_PRIME_GUARD_TERMS = {
    "integrated_template": ("model-integrated", "model integrated"),
    "prime_guide": ("input 2 is the current-size official prime visual context",),
    "qr_free_template": ("qr-free full official prime template", "qr free full official prime template"),
    "template_fidelity": ("visible official text", "official text, logo, color", "official text and logo"),
    "no_invented_official_component": ("do not invent any qr", "do not add official component", "no additional official component"),
    "business_avoidance": (
        "keep business content clear of the template areas",
        "keep business content clear of template areas",
        "keep every module and keep it out of the protected bands",
    ),
}
SEMANTIC_REDESIGN_GUARD_TERMS = {
    "reference_structure_only": (
        "use it only for business structure",
        "used only for business structure",
        "use it only for structure",
        "reference structure",
        "visual anchors",
    ),
    "remove_source_identity": (
        "do not copy any reference",
        "do not copy any prime",
        "competitor mark",
        "competitor wording",
        "no competitor",
        "source identity",
        "redesign the source identity",
    ),
    "change_high_salience_identity": (
        "reimagine",
        "original",
        "new visual identity",
        "redesign",
        "modern wedding planning table",
    ),
}
BOTTOM_BUSINESS_TERMS = ("active business", "business group", "business content", "approved business", "正文内容", "业务内容", "内容组")
BOTTOM_AVOID_TERMS = ("above", "outside", "safe", "clear", "上方", "之外", "安全区", "避让", "不进入")
BOTTOM_GROUP_TERMS = (
    "business group",
    "active business group",
    "lower business group",
    "bottom business group",
    "bottom content group",
    "lower content group",
    "benefit strip",
    "benefit labels",
    "three benefit labels",
    "cta group",
    "benefit_box",
    "cta_box",
    "content boxes",
    "底部内容组",
    "底部功能组",
    "下方内容组",
    "卖点条",
)
BOTTOM_PRIME_BAND_TERMS = (
    "bottom template boundary",
    "bottom template",
    "bottom overlay boundary",
    "bottom fixed coordinate",
    "底部模板边界",
    "底部覆盖边界",
    "底部固定坐标",
)
BOTTOM_CLEARANCE_TERMS = ("clearance", "gap", "buffer", "visible", "fully visible", "留白", "缓冲", "间距", "可见", "完整可见")
SAFE_CONTENT_FRAME_TERMS = ("safe content frame", "content safe frame", "safe_content_frame", "内容安全框", "安全内容框")
SAFE_CONTENT_SCOPE_TERMS = (
    "key business content",
    "readable content",
    "critical content",
    "text and cta",
    "safe content frame controls only",
    "full canvas",
    "full-bleed",
    "edge-to-edge",
    "no inset",
    "关键内容",
    "可读内容",
    "业务内容",
    "文字和按钮",
    "满版",
    "铺满",
    "不内缩",
    "不加边",
)
WHOLE_IMAGE_INSET_PATTERN = re.compile(
    r"(?:(?:whole|entire|full|完整|整张|整图)[^.;\n。]{0,80}"
    r"(?:fit|contain|scale down|safe content frame|safe_content_frame|收进|缩进|内缩|放入)"
    r"|(?:fit|contain|scale down|收进|缩进|内缩|放入)[^.;\n。]{0,80}"
    r"(?:whole|entire|full|完整|整张|整图))",
    re.IGNORECASE,
)
REDESIGN_GUARD_TERMS = {
    "reference_structure_only": ("structure only", "hierarchy only", "composition only", "not the reference identity", "只供结构", "仅供结构"),
    "remove_source_identity": ("remove all source", "do not retain source", "no competitor material", "not the reference identity", "移除竞品", "不保留竞品"),
    "change_high_salience_identity": ("high-salience", "new visual identity", "redesign", "重构", "高显著"),
}
REQUIRED_PROMPT_SECTIONS = (
    "TASK",
    "INPUTS",
    "PRIME RESERVED AREAS",
    "CONTENT LAYOUT",
    "APPROVED TEXT",
    "TABLE",
    "STYLE",
    "FORBIDDEN",
    "FINAL",
)
REQUIRED_PROMPT_TERMS = {
    "business_canvas": ("business_canvas", "business canvas"),
    "body_coordinate_semantics": ("coordinate grammar", "坐标语义"),
}
SEMANTIC_REQUIRED_PROMPT_SECTIONS = (
    "TASK",
    "INPUT ROLES",
    "LOCKED DESIGN DNA",
    "EDITABLE LAYOUT",
    "APPROVED COPY",
    "PRIME SUPPORT",
    "ACCEPTANCE",
)
SEMANTIC_REQUIRED_PROMPT_TERMS = {
    "input_roles": ("input 1", "input 2"),
    "design_dna": ("design dna", "designdna"),
    "approved_copy": ("render every approved string", "approved copy"),
    "prime_context": ("current-size official prime visual context",),
    "model_rendered_text": ("render every approved string", "render all approved copy"),
}
MODEL_PROMPT_PROTOCOL_TERMS = {
    "task_or_revision": ("task_id", "task id", "multica_task_id", "revision"),
    "attachment_or_lineage": ("attachment_id", "attachment id", "lineage"),
    "request_or_hash": ("request_id", "request id", "prompt_sha256", "sha256"),
    "workflow_command": ("multica ", "asset-put", "diagnostic-asset-put", "--result-file", "--output-file"),
    "workflow_control": ("retry", "timeout", "upload", "json"),
    "runtime_path": ("/app/", "<workdir>", "file path"),
}
PROMPT_SEGMENT_PATTERN = re.compile(r"[\n.;。]+")
SIZE_PATTERN = re.compile(r"(?P<width>[1-9]\d*)x(?P<height>[1-9]\d*)", re.IGNORECASE)
PROMPT_TARGET_MIN_CHARS = 900
PROMPT_TARGET_MAX_CHARS = 1800
PROMPT_COMPLEX_SOFT_MAX_CHARS = 2800
MAX_PROMPT_CHARS = 3600


def digits(value: str) -> str:
    return re.sub(r"\D", "", value)


def financial_tokens(value: str) -> set[str]:
    tokens: set[str] = set()
    for match in CURRENCY_PATTERN.finditer(value):
        normalized = digits(match.group(1))
        tokens.update((f"currency:{normalized}", f"financial_number:{normalized}"))
    for match in PERCENT_PATTERN.finditer(value):
        tokens.add(f"percent:{match.group(1).replace(',', '.')}")
    for match in TERM_PATTERN.finditer(value):
        term = re.sub(r"\s+", "", match.group(1))
        tokens.add(f"term:{term}{match.group(2).lower()}")
    for match in FINANCIAL_NUMBER_PATTERN.finditer(value):
        suffix = value[match.end(1):match.end(1) + 8]
        if re.match(r"(?:\s*(?:%|bulan|hari|tahun)\b|[.,]\d+\s*%)", suffix, re.IGNORECASE):
            continue
        tokens.add(f"financial_number:{digits(match.group(1))}")
    return tokens


def visible_prompt_financial_tokens(prompt: str) -> set[str]:
    """Extract financial facts only from sections the model may render."""
    sections = re.findall(
        r"(?:^|\n)(APPROVED TEXT|APPROVED COPY|TABLE):\n(.*?)(?=\n[A-Z][A-Z _-]+:?\n|\Z)",
        prompt,
        flags=re.DOTALL,
    )
    if not sections:
        return financial_tokens(prompt)
    return financial_tokens("\n".join(body for _, body in sections))


def read_json(path: str) -> dict[str, Any]:
    if path == "-":
        return json.load(sys.stdin)
    with Path(path).open("r", encoding="utf-8-sig") as handle:
        return json.load(handle)


def text_has_any(prompt: str, values: tuple[str, ...]) -> bool:
    normalized = prompt.casefold()
    return any(value.casefold() in normalized for value in values)


def normalize_region_id(value: object) -> str:
    return re.sub(r"[\s_-]+", " ", str(value or "").strip().casefold())


def infer_canvas_size(path: str, payload: dict[str, Any]) -> tuple[int, int] | None:
    for source in (Path(path).stem, str(payload.get("size") or ""), str(payload.get("canvas") or "")):
        match = SIZE_PATTERN.search(source)
        if match:
            return (int(match.group("width")), int(match.group("height")))
    canvas = payload.get("canvas")
    if isinstance(canvas, dict):
        width = canvas.get("width")
        height = canvas.get("height")
        if isinstance(width, int) and isinstance(height, int) and not isinstance(width, bool) and not isinstance(height, bool):
            return (width, height)
    return None


def with_canvas(layout: dict[str, Any], width: int | None, height: int | None, size_key: str | None) -> dict[str, Any]:
    value = dict(layout)
    if width is not None and height is not None:
        value["__canvas_width"] = width
        value["__canvas_height"] = height
    if size_key:
        value["__size_key"] = size_key
    return value


def load_prime_layout(path: str) -> list[dict[str, Any]]:
    payload = read_json(path)
    if isinstance(payload.get("hard_regions"), list):
        canvas_size = infer_canvas_size(path, payload)
        width, height = canvas_size if canvas_size else (None, None)
        return [with_canvas(payload, width, height, f"{width}x{height}" if width and height else None)]
    layouts = payload.get("layouts")
    if isinstance(layouts, dict) and layouts:
        loaded = []
        for size_key, layout in layouts.items():
            if not isinstance(layout, dict):
                continue
            match = SIZE_PATTERN.fullmatch(str(size_key))
            width = int(match.group("width")) if match else None
            height = int(match.group("height")) if match else None
            loaded.append(with_canvas(layout, width, height, str(size_key)))
        return loaded
    raise ValueError("prime layout file must contain hard_regions or layouts")


def region_coordinates(region: dict[str, Any]) -> list[str]:
    values = []
    for key in ("x1", "y1", "x2", "y2"):
        value = region.get(key)
        if isinstance(value, int) and not isinstance(value, bool):
            values.append(str(value))
    return values


def compact_text(value: str) -> str:
    return re.sub(r"[^a-z0-9]+", "", value.casefold())


def prompt_has_region_id(prompt: str, region_id: str) -> bool:
    normalized_prompt = prompt.casefold().replace("_", " ")
    compact_prompt = compact_text(prompt)
    compact_region = compact_text(region_id)
    if region_id in normalized_prompt or compact_region in compact_prompt:
        return True
    aliases = {
        "store badges": ("store badge", "store badges", "app store", "google play"),
        "pindai legal": ("pindai", "legal"),
        "afpi": ("afpi",),
        "regulatory": ("regulatory", "legal", "footer", "ojk"),
        "terms": ("terms", "legal", "regulatory"),
        "qr": ("qr", "qrcode", "二维码"),
        "logo": ("logo", "brand", "mark"),
    }
    return text_has_any(prompt, aliases.get(region_id, (region_id,)))


def prompt_covers_rect(prompt: str, coordinates: str, region: dict[str, Any]) -> bool:
    compact_prompt = re.sub(r"\s+", "", prompt)
    loose_coordinates = coordinates.replace(",", r"\D+")
    if coordinates in compact_prompt or re.search(loose_coordinates, prompt):
        return True
    x1, y1, x2, y2 = (region.get(key) for key in ("x1", "y1", "x2", "y2"))
    if all(isinstance(value, int) and not isinstance(value, bool) for value in (x1, y1, x2, y2)):
        x_then_y_patterns = (
            rf"x\s*{x1}\s*(?:-|–|—|to|~)\s*{x2}\D{{0,24}}y\s*{y1}\s*(?:-|–|—|to|~)\s*{y2}",
            rf"x\s*(?:from\s*)?{x1}\s*(?:to|-|–|—|~)\s*{x2}\D{{0,24}}y\s*(?:from\s*)?{y1}\s*(?:to|-|–|—|~)\s*{y2}",
        )
        if any(re.search(pattern, prompt, re.IGNORECASE) for pattern in x_then_y_patterns):
            return True
    bottom = region.get("bottom_key_content_exclusion_start")
    if isinstance(bottom, int) and not isinstance(bottom, bool):
        y1 = region.get("y1")
        if isinstance(y1, int) and y1 >= bottom:
            patterns = (
                rf"all\s+y\s*(?:>=|≥|from|above)\s*{bottom}",
                rf"y\s*(?:>=|≥)\s*{bottom}",
                rf"bottom[^.;\n]{{0,80}}{bottom}",
            )
            if any(re.search(pattern, prompt, re.IGNORECASE) for pattern in patterns):
                return True
    return False


def layout_canvas_size(layout: dict[str, Any]) -> tuple[int, int] | None:
    width = layout.get("__canvas_width")
    height = layout.get("__canvas_height")
    if isinstance(width, int) and isinstance(height, int) and not isinstance(width, bool) and not isinstance(height, bool):
        return (width, height)
    return None


def layout_safe_content_frame(layout: dict[str, Any]) -> dict[str, int] | None:
    for key in ("safe_content_frame", "content_safe_frame"):
        frame = layout.get(key)
        if isinstance(frame, dict):
            values = {name: frame.get(name) for name in ("x1", "y1", "x2", "y2")}
            if all(isinstance(value, int) and not isinstance(value, bool) for value in values.values()):
                if values["x2"] > values["x1"] and values["y2"] > values["y1"]:
                    return values
        if isinstance(frame, list) and len(frame) == 4:
            values = {name: value for name, value in zip(("x1", "y1", "x2", "y2"), frame)}
            if all(isinstance(value, int) and not isinstance(value, bool) for value in values.values()):
                if values["x2"] > values["x1"] and values["y2"] > values["y1"]:
                    return values
    canvas_size = layout_canvas_size(layout)
    if canvas_size is None:
        return None
    width, height = canvas_size
    top = layout.get("top_key_content_exclusion_end", 0)
    bottom = layout.get("bottom_key_content_exclusion_start", height)
    if not isinstance(top, int) or isinstance(top, bool):
        top = 0
    if not isinstance(bottom, int) or isinstance(bottom, bool):
        bottom = height
    vertical_reserve = round(max(top, height - bottom) / 2)
    horizontal_reserve = round(vertical_reserve * width / max(1, height) * 2 / 3)
    x1 = horizontal_reserve
    x2 = width - horizontal_reserve
    y1 = top + vertical_reserve
    y2 = bottom - vertical_reserve
    if x2 <= x1 or y2 <= y1:
        return None
    return {"x1": x1, "y1": y1, "x2": x2, "y2": y2}


def prompt_mentions_business_canvas_grammar(prompt: str, y1: int, y2: int) -> bool:
    normalized = prompt.casefold().replace("_", " ")
    return (
        "business canvas" in normalized
        and str(y1) in prompt
        and str(y2) in prompt
        and "physical canvas" in normalized
        and "headline zone" in normalized
    )


def prompt_mentions_bottom_group_clearance(prompt: str, y2: int, bottom_start: int) -> bool:
    if not text_has_any(prompt, BOTTOM_GROUP_TERMS):
        return False
    if not text_has_any(prompt, BOTTOM_PRIME_BAND_TERMS):
        return False
    if not text_has_any(prompt, BOTTOM_CLEARANCE_TERMS):
        return False
    if str(bottom_start) not in prompt:
        return False
    return str(y2) in prompt or text_has_any(prompt, SAFE_CONTENT_FRAME_TERMS)


def validate_prime_prompt_guard(prompt: str, layouts: list[dict[str, Any]]) -> list[str]:
    """Return missing Prime-safe guard requirements for a final image prompt."""

    if uses_model_integrated_prime_contract(prompt):
        return validate_model_integrated_prime_prompt_guard(prompt, layouts)
    if uses_coordinate_free_prime_contract(prompt):
        return validate_semantic_prime_prompt_guard(prompt, layouts)

    missing: list[str] = []
    for key, terms in PRIME_GUARD_TERMS.items():
        if not text_has_any(prompt, terms):
            missing.append(key)

    normalized_prompt = prompt.casefold().replace("_", " ")
    required_region_ids: set[str] = set()
    required_regions: list[dict[str, Any]] = []
    bottom_values: set[str] = set()
    for layout in layouts:
        hard_regions = layout.get("hard_regions")
        if not isinstance(hard_regions, list):
            missing.append("layout.hard_regions")
            continue
        bottom = layout.get("bottom_key_content_exclusion_start")
        for region in hard_regions:
            if not isinstance(region, dict):
                missing.append("layout.hard_regions.item")
                continue
            region_id = normalize_region_id(region.get("id"))
            if region_id:
                required_region_ids.add(region_id)
            coordinates = region_coordinates(region)
            if len(coordinates) == 4:
                region_with_bounds = dict(region)
                if isinstance(bottom, int) and not isinstance(bottom, bool):
                    region_with_bounds["bottom_key_content_exclusion_start"] = bottom
                required_regions.append({"id": region_id, "coordinates": ",".join(coordinates), **region_with_bounds})
        bottom_value = layout.get("bottom_key_content_exclusion_start")
        if isinstance(bottom_value, int) and not isinstance(bottom_value, bool):
            bottom_values.add(str(bottom_value))

    for region_id in sorted(required_region_ids):
        if not prompt_has_region_id(prompt, region_id):
            missing.append(f"region_id:{region_id}")
    for region in sorted(required_regions, key=lambda item: item["coordinates"]):
        coordinates = str(region["coordinates"])
        if not prompt_covers_rect(prompt, coordinates, region):
            missing.append(f"region_rect:{coordinates}")
    for value in sorted(bottom_values, key=int):
        if value not in prompt:
            missing.append(f"exclusion_boundary:{value}")
    if bottom_values and (not text_has_any(prompt, BOTTOM_BUSINESS_TERMS) or not text_has_any(prompt, BOTTOM_AVOID_TERMS)):
        missing.append("bottom_business_avoidance")
    for layout in layouts:
        frame = layout_safe_content_frame(layout)
        if frame is None:
            missing.append("safe_content_frame:unresolved_canvas")
            continue
        if not text_has_any(prompt, SAFE_CONTENT_FRAME_TERMS):
            missing.append("safe_content_frame_label")
        if not text_has_any(prompt, SAFE_CONTENT_SCOPE_TERMS):
            missing.append("safe_content_frame_scope")
        if WHOLE_IMAGE_INSET_PATTERN.search(prompt):
            missing.append("whole_image_inset_forbidden")
        frame_region = {"x1": frame["x1"], "y1": frame["y1"], "x2": frame["x2"], "y2": frame["y2"]}
        coordinates = f"{frame['x1']},{frame['y1']},{frame['x2']},{frame['y2']}"
        if not prompt_covers_rect(prompt, coordinates, frame_region):
            missing.append(f"safe_content_frame_rect:{coordinates}")
        if not prompt_mentions_business_canvas_grammar(prompt, frame["y1"], frame["y2"]):
            missing.append(f"business_canvas_grammar:{frame['y1']}:{frame['y2']}")
        bottom_start = layout.get("bottom_key_content_exclusion_start")
        if isinstance(bottom_start, int) and not isinstance(bottom_start, bool):
            if not prompt_mentions_bottom_group_clearance(prompt, frame["y2"], bottom_start):
                missing.append(f"bottom_group_prime_clearance:{frame['y2']}:{bottom_start}")
    return missing


def uses_coordinate_free_prime_contract(prompt: str) -> bool:
    normalized = re.sub(r"\s+", " ", prompt.casefold())
    return (
        "input roles" in normalized
        and "locked design dna" in normalized
        and "prime support" in normalized
        and "input 2 is the current-size official prime visual context" in normalized
    )


def uses_model_integrated_prime_contract(prompt: str) -> bool:
    normalized = re.sub(r"\s+", " ", prompt.casefold())
    return (
        "model-integrated" in normalized
        and "qr-free full official prime template" in normalized
        and "input 2 is the current-size official prime visual context" in normalized
    )


def validate_semantic_prime_prompt_guard(prompt: str, layouts: list[dict[str, Any]]) -> list[str]:
    """Validate the current coordinate-free Prime prompt contract.

    The model prompt intentionally describes protected bands in natural language;
    exact geometry remains in the layout evidence and deterministic composer.
    """

    missing = [key for key, terms in SEMANTIC_PRIME_GUARD_TERMS.items() if not text_has_any(prompt, terms)]
    for layout in layouts:
        if not isinstance(layout.get("hard_regions"), list):
            missing.append("layout.hard_regions")
        if not isinstance(layout.get("top_key_content_exclusion_end"), int):
            missing.append("layout.top_key_content_exclusion_end")
        if not isinstance(layout.get("bottom_key_content_exclusion_start"), int):
            missing.append("layout.bottom_key_content_exclusion_start")
    return missing


def validate_model_integrated_prime_prompt_guard(prompt: str, layouts: list[dict[str, Any]]) -> list[str]:
    missing = [key for key, terms in SEMANTIC_MODEL_INTEGRATED_PRIME_GUARD_TERMS.items() if not text_has_any(prompt, terms)]
    for layout in layouts:
        if not isinstance(layout.get("hard_regions"), list):
            missing.append("layout.hard_regions")
        if not isinstance(layout.get("top_key_content_exclusion_end"), int):
            missing.append("layout.top_key_content_exclusion_end")
        if not isinstance(layout.get("bottom_key_content_exclusion_start"), int):
            missing.append("layout.bottom_key_content_exclusion_start")
    return missing


def validate_redesign_prompt_guard(prompt: str) -> list[str]:
    if uses_model_integrated_prime_contract(prompt):
        return [key for key, terms in SEMANTIC_REDESIGN_GUARD_TERMS.items() if not text_has_any(prompt, terms)]
    if uses_coordinate_free_prime_contract(prompt):
        return [key for key, terms in SEMANTIC_REDESIGN_GUARD_TERMS.items() if not text_has_any(prompt, terms)]
    return [key for key, terms in REDESIGN_GUARD_TERMS.items() if not text_has_any(prompt, terms)]


def validate_concise_prompt(prompt: str) -> list[str]:
    debt: list[str] = []
    if len(prompt) > MAX_PROMPT_CHARS:
        debt.append(f"prompt_too_long:{len(prompt)}>{MAX_PROMPT_CHARS}")
    required_sections = (
        SEMANTIC_REQUIRED_PROMPT_SECTIONS if uses_coordinate_free_prime_contract(prompt) else REQUIRED_PROMPT_SECTIONS
    )
    required_terms = (
        SEMANTIC_REQUIRED_PROMPT_TERMS if uses_coordinate_free_prime_contract(prompt) else REQUIRED_PROMPT_TERMS
    )
    semantic_sections = uses_coordinate_free_prime_contract(prompt)
    for section in required_sections:
        suffix = r":?\s*$" if semantic_sections else r":"
        if not re.search(rf"(?im)^\s*{re.escape(section)}\s*{suffix}", prompt):
            debt.append(f"missing_section:{section}")
    for key, terms in required_terms.items():
        if not text_has_any(prompt, terms):
            debt.append(f"missing_term:{key}")
    for key, terms in MODEL_PROMPT_PROTOCOL_TERMS.items():
        if text_has_any(prompt, terms):
            debt.append(f"non_visual_protocol_term:{key}")

    normalized_counts: dict[str, int] = {}
    for raw_segment in PROMPT_SEGMENT_PATTERN.split(prompt):
        segment = re.sub(r"\s+", " ", raw_segment.strip().casefold())
        segment = re.sub(r"^(?:[-*]|\d+[.)])\s*", "", segment)
        if len(segment) < 64:
            continue
        normalized_counts[segment] = normalized_counts.get(segment, 0) + 1
    debt.extend(f"duplicate_segment:{segment[:96]}" for segment, count in sorted(normalized_counts.items()) if count > 1)
    return debt


def infer_size_key(source: str, prompt: str) -> str:
    for value in (source, prompt[:400]):
        match = SIZE_PATTERN.search(value)
        if match:
            return f"{match.group('width')}x{match.group('height')}"
    return ""


def line_info(prompt: str, needle: str) -> dict[str, Any]:
    if not needle:
        return {}
    normalized_needle = needle.casefold()
    for index, line in enumerate(prompt.splitlines(), start=1):
        if normalized_needle in line.casefold():
            return {"line": index, "excerpt": line.strip()[:220]}
    return {}


def line_info_for_financial_token(prompt: str, token: str) -> dict[str, Any]:
    if ":" not in token:
        return {}
    kind, value = token.split(":", 1)
    if kind == "percent":
        value = value.replace(".", ",")
    compact_value = digits(value)
    if not compact_value:
        return {}
    for index, line in enumerate(prompt.splitlines(), start=1):
        compact_line = digits(line)
        if compact_value in compact_line:
            return {"line": index, "excerpt": line.strip()[:220]}
    return {}


def missing_terms_for_guard(rule: str) -> tuple[str, ...]:
    if rule in SEMANTIC_MODEL_INTEGRATED_PRIME_GUARD_TERMS:
        return SEMANTIC_MODEL_INTEGRATED_PRIME_GUARD_TERMS[rule]
    if rule in SEMANTIC_PRIME_GUARD_TERMS:
        return SEMANTIC_PRIME_GUARD_TERMS[rule]
    if rule in PRIME_GUARD_TERMS:
        return PRIME_GUARD_TERMS[rule]
    if rule in SEMANTIC_REDESIGN_GUARD_TERMS:
        return SEMANTIC_REDESIGN_GUARD_TERMS[rule]
    if rule in REDESIGN_GUARD_TERMS:
        return REDESIGN_GUARD_TERMS[rule]
    if rule in SEMANTIC_REQUIRED_PROMPT_TERMS:
        return SEMANTIC_REQUIRED_PROMPT_TERMS[rule]
    if rule in REQUIRED_PROMPT_TERMS:
        return REQUIRED_PROMPT_TERMS[rule]
    return ()


def build_repair_guidance(
    *,
    source: str,
    prompt: str,
    unapproved: list[str],
    approved: set[str],
    missing_prime_guard: list[str],
    missing_redesign_guard: list[str],
    prompt_debt: list[str],
) -> list[dict[str, Any]]:
    size_key = infer_size_key(source, prompt)
    prefix = {"source": source, "size_key": size_key, "severity": "error"}
    guidance: list[dict[str, Any]] = []
    for token in unapproved:
        item = {
            **prefix,
            "rule": "unapproved_financial_token",
            "token": token,
            "message": "Prompt contains a visible financial token that is absent from the selected copy_snapshot.",
            "approved_financial_tokens": sorted(approved),
            "fix": "Remove this visible value or replace it with an approved value from copy_snapshot/approved_copy; do not self-approve it in prompt text.",
        }
        item.update(line_info_for_financial_token(prompt, token))
        guidance.append(item)
    for rule in missing_prime_guard:
        terms = missing_terms_for_guard(rule)
        guidance.append(
            {
                **prefix,
                "rule": "missing_prime_guard",
                "requirement": rule,
                "acceptable_terms": list(terms),
                "message": "Prompt is missing a Prime-safety guard required before image generation.",
                "fix": "Add one concise natural-language sentence that satisfies this requirement without adding coordinates, JSON, or repeated audit text.",
            }
        )
    for rule in missing_redesign_guard:
        terms = missing_terms_for_guard(rule)
        guidance.append(
            {
                **prefix,
                "rule": "missing_redesign_guard",
                "requirement": rule,
                "acceptable_terms": list(terms),
                "message": "Prompt does not explicitly remove source/competitor identity or require a redesigned visual identity.",
                "fix": "State that the reference is structure only, remove competitor/source identity, and redesign the high-salience visual identity.",
            }
        )
    for debt in prompt_debt:
        if debt.startswith("prompt_too_long:"):
            current = len(prompt)
            guidance.append(
                {
                    **prefix,
                    "rule": "prompt_too_long",
                    "current_chars": current,
                    "target_chars": f"{PROMPT_TARGET_MIN_CHARS}-{PROMPT_TARGET_MAX_CHARS}",
                    "complex_soft_max_chars": PROMPT_COMPLEX_SOFT_MAX_CHARS,
                    "max_chars": MAX_PROMPT_CHARS,
                    "message": f"Prompt is {current} characters; hard cap is {MAX_PROMPT_CHARS}.",
                    "fix": "Keep model-facing composition instructions and approved copy, but remove audit logs, JSON, hashes, duplicate wording, and non-visual commentary. Complex table/App-UI prompts may stay above the target range, but must remain under the hard cap.",
                }
            )
            continue
        if debt.startswith("missing_section:"):
            section = debt.split(":", 1)[1]
            guidance.append(
                {
                    **prefix,
                    "rule": "missing_section",
                    "required_section": section,
                    "message": f"Prompt must include the `{section}` section from the production template.",
                    "fix": "Add this template heading and fill it with only the size-specific model-facing instruction.",
                }
            )
            continue
        if debt.startswith("missing_term:"):
            requirement = debt.split(":", 1)[1]
            terms = missing_terms_for_guard(requirement)
            guidance.append(
                {
                    **prefix,
                    "rule": "missing_term",
                    "requirement": requirement,
                    "acceptable_terms": list(terms),
                    "message": "Prompt is missing a required semantic term from the production contract.",
                    "fix": "Add the missing semantic constraint once, using the production template wording when possible.",
                }
            )
            continue
        if debt.startswith("duplicate_segment:"):
            snippet = debt.split(":", 1)[1]
            item = {
                **prefix,
                "rule": "duplicate_segment",
                "snippet": snippet,
                "message": "Prompt repeats a long instruction segment.",
                "fix": "Keep the clearest occurrence and delete the duplicate segment; do not compensate by adding another paraphrase.",
            }
            item.update(line_info(prompt, snippet[:48]))
            guidance.append(item)
            continue
        if debt.startswith("non_visual_protocol_term:"):
            requirement = debt.split(":", 1)[1]
            guidance.append(
                {
                    **prefix,
                    "rule": "non_visual_protocol_term",
                    "requirement": requirement,
                    "message": "Model prompt contains workflow or transaction protocol that cannot change pixels.",
                    "fix": "Remove task, revision, file, hash, request, upload, retry, timeout, JSON, CLI, and lineage instructions from model-facing text.",
                }
            )
            continue
        guidance.append(
            {
                **prefix,
                "rule": "prompt_debt",
                "requirement": debt,
                "message": "Prompt failed a concise-prompt validation rule.",
                "fix": "Revise only this rule's cause and rerun validation before calling the image model.",
            }
        )
    return guidance


def summarize_repair_guidance(guidance: list[dict[str, Any]]) -> list[str]:
    if not guidance:
        return ["All prompt validation checks passed."]
    summary = []
    for item in guidance:
        where = item["source"]
        if item.get("size_key"):
            where += f" ({item['size_key']})"
        line = f": line {item['line']}" if item.get("line") else ""
        rule = item.get("rule") or "validation"
        fix = item.get("fix") or item.get("message") or "Revise this item and rerun validation."
        summary.append(f"{where}{line}: {rule} - {fix}")
    return summary


def item_has_variant(item: dict[str, Any], variant_id: str) -> bool:
    return any(str(variant.get("id")) == variant_id for variant in item.get("variants", []) if isinstance(variant, dict))


def find_item(payload: dict[str, Any], candidate_id: str, order_item_id: str = "", variant_id: str = "") -> dict[str, Any]:
    candidate_id = str(candidate_id).strip()
    order_item_id = str(order_item_id).strip()
    variant_id = str(variant_id).strip()
    if order_item_id or variant_id:
        for item in payload.get("items", []):
            if not isinstance(item, dict):
                continue
            if order_item_id and str(item.get("id")) != order_item_id:
                continue
            if variant_id and not item_has_variant(item, variant_id):
                continue
            if candidate_id and str(item.get("candidate_id")) != candidate_id:
                raise ValueError(
                    f"selected order item {item.get('id') or '<unknown>'} does not belong to candidate {candidate_id}"
                )
            return item
        target = f"order item {order_item_id}" if order_item_id else f"variant {variant_id}"
        raise ValueError(f"{target} has no selected copy snapshot")
    for item in payload.get("items", []):
        if isinstance(item, dict) and str(item.get("candidate_id")) == candidate_id:
            return item
    raise ValueError(f"candidate {candidate_id} has no selected copy snapshot")


def approved_text(snapshot: dict[str, Any]) -> str:
    values = [
        snapshot.get("headline"),
        snapshot.get("subheadline"),
        snapshot.get("benefit"),
        snapshot.get("supporting"),
        snapshot.get("cta"),
        snapshot.get("legal_text"),
    ]
    for entry in snapshot.get("repayment_plan_entries") or []:
        if isinstance(entry, dict):
            values.extend(
                entry.get(key)
                for key in (
                    "principal",
                    "tenor_months",
                    "monthly_installment",
                    "total_interest",
                    "total_repayment",
                    "source",
                )
            )
    adaptation = snapshot.get("pre_adaptation") or {}
    for replacement in adaptation.get("text_replacements") or []:
        if isinstance(replacement, dict):
            values.append(replacement.get("replacement_text"))
            calculation = replacement.get("calculation")
            if isinstance(calculation, dict):
                values.append(calculation.get("result"))
                values.extend(calculation.get("inputs") or [])
    for selection in adaptation.get("repayment_plan_selections") or []:
        if isinstance(selection, dict):
            values.extend(selection.get(key) for key in ("principal", "tenor_months"))
            selection_values = selection.get("values")
            if isinstance(selection_values, dict):
                values.extend(selection_values.values())
    for layout in adaptation.get("numeric_layouts") or []:
        if isinstance(layout, dict):
            values.append(layout.get("render_instruction"))
    return "\n".join(str(value) for value in values if value)


def validate_snapshot(snapshot: dict[str, Any], candidate_id: str) -> None:
    if snapshot.get("schema_version") != 3:
        raise ValueError(f"candidate {candidate_id} copy snapshot must use schema_version 3")
    if snapshot.get("creative_type") not in CREATIVE_TYPES:
        raise ValueError(f"candidate {candidate_id} copy snapshot has invalid creative_type")
    status = snapshot.get("status")
    if status not in {"approved", "model_pre_adapted", "user_custom"}:
        raise ValueError(f"candidate {candidate_id} copy snapshot has invalid status")
    if status in {"approved", "model_pre_adapted"}:
        required = ("library_id", "library_version")
        if any(not snapshot.get(field) for field in required):
            raise ValueError(f"candidate {candidate_id} approved copy snapshot has incomplete library provenance")
        has_composition = bool(snapshot.get("composition_id") and snapshot.get("composition_key"))
        has_legacy_recipe = bool(snapshot.get("recipe_id") and snapshot.get("recipe_key"))
        if status == "approved" and not has_composition and not has_legacy_recipe:
            raise ValueError(f"candidate {candidate_id} approved copy snapshot has incomplete composition provenance")
        if not isinstance(snapshot.get("fragments"), list) or not isinstance(snapshot.get("repayment_plan_entries"), list):
            raise ValueError(f"candidate {candidate_id} approved copy snapshot has invalid evidence")


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--materials-json", required=True)
    parser.add_argument("--candidate-id", required=True)
    parser.add_argument("--order-item-id", default="")
    parser.add_argument("--variant-id", default="")
    parser.add_argument("--prompt-file", action="append", default=[])
    parser.add_argument("--prompt-text", action="append", default=[])
    parser.add_argument("--prime-layout-file")
    parser.add_argument("--require-prime-guard", action="store_true")
    parser.add_argument("--require-redesign-guard", action="store_true")
    parser.add_argument("--require-concise-prompt", action="store_true")
    parser.add_argument("--explain", action="store_true", help="include a concise repair summary for failed prompt checks")
    parser.add_argument("--evidence")
    args = parser.parse_args()

    payload = read_json(args.materials_json)
    item = find_item(payload, args.candidate_id, args.order_item_id, args.variant_id)
    snapshot = item.get("copy_snapshot") or {}
    validate_snapshot(snapshot, args.candidate_id)
    approved = financial_tokens(approved_text(snapshot))

    prompts: list[tuple[str, str]] = []
    for raw_path in args.prompt_file:
        path = Path(raw_path)
        prompts.append((str(path), path.read_text(encoding="utf-8-sig")))
    prompts.extend((f"inline:{index + 1}", value) for index, value in enumerate(args.prompt_text))
    if not prompts:
        raise ValueError("at least one --prompt-file or --prompt-text is required")

    layouts = load_prime_layout(args.prime_layout_file) if args.prime_layout_file else []
    if args.require_prime_guard and not layouts:
        raise ValueError("--require-prime-guard requires --prime-layout-file")

    checks = []
    all_guidance: list[dict[str, Any]] = []
    passed = True
    for source, prompt in prompts:
        observed = visible_prompt_financial_tokens(prompt)
        unapproved = sorted(observed - approved)
        missing_prime_guard = validate_prime_prompt_guard(prompt, layouts) if args.require_prime_guard else []
        missing_redesign_guard = validate_redesign_prompt_guard(prompt) if args.require_redesign_guard else []
        prompt_debt = validate_concise_prompt(prompt) if args.require_concise_prompt else []
        repair_guidance = build_repair_guidance(
            source=source,
            prompt=prompt,
            unapproved=unapproved,
            approved=approved,
            missing_prime_guard=missing_prime_guard,
            missing_redesign_guard=missing_redesign_guard,
            prompt_debt=prompt_debt,
        )
        all_guidance.extend(repair_guidance)
        prompt_passed = not unapproved and not missing_prime_guard and not missing_redesign_guard and not prompt_debt
        passed = passed and prompt_passed
        checks.append(
            {
                "source": source,
                "size_key": infer_size_key(source, prompt),
                "prompt_chars": len(prompt),
                "observed_financial_tokens": sorted(observed),
                "unapproved_financial_tokens": unapproved,
                "missing_prime_guard": missing_prime_guard,
                "missing_redesign_guard": missing_redesign_guard,
                "prompt_debt": prompt_debt,
                "repair_guidance": repair_guidance,
                "passed": prompt_passed,
            }
        )

    evidence = {
        "candidate_id": args.candidate_id,
        "order_item_id": item.get("id") or args.order_item_id,
        "variant_id": args.variant_id,
        "copy_snapshot_id": snapshot.get("composition_id") or snapshot.get("id"),
        "copy_snapshot_version": snapshot.get("library_version") or snapshot.get("version"),
        "approved_financial_tokens": sorted(approved),
        "prompt_policy": {
            "target_chars": f"{PROMPT_TARGET_MIN_CHARS}-{PROMPT_TARGET_MAX_CHARS}",
            "complex_soft_max_chars": PROMPT_COMPLEX_SOFT_MAX_CHARS,
            "hard_max_chars": MAX_PROMPT_CHARS,
        },
        "prompt_checks": checks,
        "repair_guidance": all_guidance,
        "passed": passed,
    }
    if args.explain:
        evidence["repair_summary"] = summarize_repair_guidance(all_guidance)
    encoded = json.dumps(evidence, ensure_ascii=False, indent=2)
    if args.evidence:
        Path(args.evidence).write_text(encoded + "\n", encoding="utf-8")
    print(encoded)
    return 0 if passed else 2


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except (OSError, ValueError, json.JSONDecodeError) as exc:
        print(json.dumps({"passed": False, "error": str(exc)}, ensure_ascii=False), file=sys.stderr)
        raise SystemExit(2)
