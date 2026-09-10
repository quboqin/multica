import json
from pathlib import Path

from PIL import Image

from render_prime_guide import main


def test_render_prime_guide_outputs_non_rendering_layout_map(tmp_path: Path, monkeypatch) -> None:
    layout_path = tmp_path / "layout-1080x1080.json"
    output_path = tmp_path / "prime-guide.png"
    evidence_path = tmp_path / "prime-guide.json"
    layout_path.write_text(json.dumps({
        "hard_regions": [
            {"id": "logo", "x1": 30, "y1": 28, "x2": 314, "y2": 100},
            {"id": "qr", "x1": 983, "y1": 29, "x2": 1053, "y2": 99},
        ],
        "safe_content_frame": [50, 100, 1030, 934],
        "top_key_content_exclusion_end": 100,
        "bottom_key_content_exclusion_start": 984,
    }), encoding="utf-8")
    monkeypatch.setattr("sys.argv", [
        "render_prime_guide.py",
        "--layout-file", str(layout_path),
        "--width", "1080",
        "--height", "1080",
        "--output", str(output_path),
        "--evidence", str(evidence_path),
    ])

    assert main() == 0
    with Image.open(output_path) as image:
        assert image.size == (1080, 1080)
        assert image.mode == "RGBA"
        assert image.getpixel((30, 28))[3] > 0
        assert image.getpixel((540, 540))[3] == 0
    evidence = json.loads(evidence_path.read_text(encoding="utf-8"))
    assert evidence["non_rendering_guide"] is True
    assert evidence["render_style"] == "transparent_neutral_outlines"
    assert evidence["source_canvas"] == {"width": 1080, "height": 1080}
    assert evidence["safe_content_frame"] == {"x1": 50, "y1": 100, "x2": 1030, "y2": 934}
    assert [region["id"] for region in evidence["hard_regions"]] == ["logo", "qr"]


def test_render_prime_guide_scales_frozen_delivery_coordinates_to_model_canvas(tmp_path: Path, monkeypatch) -> None:
    layout_path = tmp_path / "layout-1200x628.json"
    output_path = tmp_path / "prime-guide.png"
    evidence_path = tmp_path / "prime-guide.json"
    layout_path.write_text(json.dumps({
        "hard_regions": [{"id": "green_full:header", "x1": 0, "y1": 0, "x2": 1200, "y2": 70}],
        "top_key_content_exclusion_end": 70,
        "bottom_key_content_exclusion_start": 560,
    }), encoding="utf-8")
    monkeypatch.setattr("sys.argv", [
        "render_prime_guide.py", "--layout-file", str(layout_path), "--width", "1200", "--height", "624",
        "--output", str(output_path), "--evidence", str(evidence_path),
    ])

    assert main() == 0
    evidence = json.loads(evidence_path.read_text(encoding="utf-8"))
    assert evidence["source_canvas"] == {"width": 1200, "height": 628}
    assert evidence["hard_regions"] == [{"id": "green_full:header", "rect": [0, 0, 1200, 70]}]


def test_render_prime_context_preserves_official_visual_cues_only_in_hard_regions(tmp_path: Path, monkeypatch) -> None:
    layout_path = tmp_path / "layout-4x4.json"
    template_path = tmp_path / "official-prime.png"
    output_path = tmp_path / "prime-context.png"
    evidence_path = tmp_path / "prime-context.json"
    layout_path.write_text(json.dumps({
        "hard_regions": [{"id": "footer", "x1": 0, "y1": 2, "x2": 4, "y2": 4}],
    }), encoding="utf-8")
    Image.new("RGBA", (4, 4), (180, 20, 30, 255)).save(template_path)
    monkeypatch.setattr("sys.argv", [
        "render_prime_guide.py", "--layout-file", str(layout_path), "--width", "4", "--height", "4",
        "--template-image", str(template_path), "--output", str(output_path), "--evidence", str(evidence_path),
    ])

    assert main() == 0
    with Image.open(output_path) as image:
        assert image.mode == "RGBA"
        assert image.getpixel((1, 3))[:3] == (180, 20, 30)
        assert image.getpixel((1, 3))[3] == 255
        assert image.getpixel((1, 1))[3] == 0
    evidence = json.loads(evidence_path.read_text(encoding="utf-8"))
    assert evidence["non_rendering_context"] is True
    assert evidence["render_style"] == "official_prime_visual_context"
    assert evidence["template_alpha"] == 1.0


def test_render_prime_context_emits_conservative_reflow_guide(tmp_path: Path, monkeypatch) -> None:
    layout_path = tmp_path / "layout-1080x1080.json"
    template_path = tmp_path / "official-prime.png"
    output_path = tmp_path / "prime-context.png"
    envelope_path = tmp_path / "prime-envelope.png"
    evidence_path = tmp_path / "prime-context.json"
    layout_path.write_text(json.dumps({
        "hard_regions": [
            {"id": "header", "x1": 0, "y1": 0, "x2": 1080, "y2": 100},
            {"id": "footer", "x1": 0, "y1": 988, "x2": 1080, "y2": 1080},
        ],
        "safe_content_frame": [33, 149, 1047, 938],
    }), encoding="utf-8")
    Image.new("RGBA", (1080, 1080), (180, 20, 30, 255)).save(template_path)
    monkeypatch.setattr("sys.argv", [
        "render_prime_guide.py", "--layout-file", str(layout_path), "--width", "1080", "--height", "1080",
        "--template-image", str(template_path), "--output", str(output_path),
        "--content-envelope-output", str(envelope_path), "--evidence", str(evidence_path),
    ])

    assert main() == 0
    with Image.open(envelope_path) as image:
        assert image.mode == "RGBA"
        assert image.getpixel((540, 300))[3] == 0
        assert image.getpixel((540, 500))[3] == 88
    evidence = json.loads(evidence_path.read_text(encoding="utf-8"))
    assert evidence["content_envelope"] == {"title": [33, 216, 1047, 430], "table": [33, 620, 1047, 860]}
    assert evidence["content_envelope_render_style"] == "official_prime_reflow_window_context"
