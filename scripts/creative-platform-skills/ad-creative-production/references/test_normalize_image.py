import json
from pathlib import Path

from PIL import Image

import normalize_image


def test_prime_safe_fit_preserves_edge_to_edge_canvas_for_legacy_callers(tmp_path: Path, monkeypatch) -> None:
    source = Image.new("RGB", (100, 100), "white")
    for y in range(86, 100):
        for x in range(20, 80):
            source.putpixel((x, y), (0, 0, 0))
    input_path = tmp_path / "input.png"
    output_path = tmp_path / "output.png"
    evidence_path = tmp_path / "evidence.json"
    layout_path = tmp_path / "layout.json"
    source.save(input_path)
    layout_path.write_text(json.dumps({
        "hard_regions": [],
        "top_key_content_exclusion_end": 10,
        "bottom_key_content_exclusion_start": 90,
    }), encoding="utf-8")

    monkeypatch.setattr("sys.argv", [
        "normalize_image.py",
        "--input", str(input_path),
        "--output", str(output_path),
        "--width", "100",
        "--height", "100",
        "--prime-layout-file", str(layout_path),
        "--prime-safe-fit",
        "--prime-safe-margin", "2",
        "--evidence", str(evidence_path),
    ])

    assert normalize_image.main() == 0
    evidence = json.loads(evidence_path.read_text(encoding="utf-8"))
    assert evidence["prime_safe_audit"] is True
    assert evidence["prime_safe_fit"] is False
    assert evidence["edge_to_edge_canvas_preserved"] is True
    assert evidence["safe_content_rect"] == {"x1": 2, "y1": 12, "x2": 98, "y2": 88}

    output = Image.open(output_path).convert("RGB")
    bottom_pixels = [output.getpixel((x, y)) for y in range(90, 100) for x in range(20, 80)]
    assert max(sum(pixel) for pixel in bottom_pixels) == 0


def test_prime_safe_fit_uses_separate_x_and_y_margins(tmp_path: Path, monkeypatch) -> None:
    source = Image.new("RGB", (100, 100), "white")
    input_path = tmp_path / "input.png"
    output_path = tmp_path / "output.png"
    evidence_path = tmp_path / "evidence.json"
    layout_path = tmp_path / "layout.json"
    source.save(input_path)
    layout_path.write_text(json.dumps({
        "hard_regions": [],
        "top_key_content_exclusion_end": 10,
        "bottom_key_content_exclusion_start": 90,
    }), encoding="utf-8")

    monkeypatch.setattr("sys.argv", [
        "normalize_image.py",
        "--input", str(input_path),
        "--output", str(output_path),
        "--width", "100",
        "--height", "100",
        "--prime-layout-file", str(layout_path),
        "--prime-safe-fit",
        "--prime-safe-x-margin", "4",
        "--prime-safe-y-margin", "8",
        "--evidence", str(evidence_path),
    ])

    assert normalize_image.main() == 0
    evidence = json.loads(evidence_path.read_text(encoding="utf-8"))
    assert evidence["prime_safe_audit"] is True
    assert evidence["prime_safe_fit"] is False
    assert evidence["safe_content_margin"] == {"x": 4, "y": 8}
    assert evidence["safe_content_rect"] == {"x1": 4, "y1": 18, "x2": 96, "y2": 82}


def test_prime_safe_fit_derives_default_frame_from_layout_contract(tmp_path: Path, monkeypatch) -> None:
    source = Image.new("RGB", (100, 100), "white")
    input_path = tmp_path / "input.png"
    output_path = tmp_path / "output.png"
    evidence_path = tmp_path / "evidence.json"
    layout_path = tmp_path / "layout.json"
    source.save(input_path)
    layout_path.write_text(json.dumps({
        "hard_regions": [],
        "top_key_content_exclusion_end": 10,
        "bottom_key_content_exclusion_start": 90,
    }), encoding="utf-8")

    monkeypatch.setattr("sys.argv", [
        "normalize_image.py",
        "--input", str(input_path),
        "--output", str(output_path),
        "--width", "100",
        "--height", "100",
        "--prime-layout-file", str(layout_path),
        "--prime-safe-fit",
        "--evidence", str(evidence_path),
    ])

    assert normalize_image.main() == 0
    evidence = json.loads(evidence_path.read_text(encoding="utf-8"))
    assert evidence["prime_safe_audit"] is True
    assert evidence["prime_safe_fit"] is False
    assert evidence["safe_content_rect_source"] == "derived_from_prime_layout_contract"
    assert evidence["safe_content_margin"] == {"x": 3, "y": 5}
    assert evidence["safe_content_rect"] == {"x1": 3, "y1": 15, "x2": 97, "y2": 85}
