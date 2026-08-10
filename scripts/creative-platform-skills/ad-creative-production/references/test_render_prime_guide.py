import json
from pathlib import Path

from PIL import Image

from render_prime_guide import main


def test_render_prime_guide_outputs_non_rendering_layout_map(tmp_path: Path, monkeypatch) -> None:
    layout_path = tmp_path / "layout-1080x1080.json"
    output_path = tmp_path / "prime-guide.jpg"
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
        assert image.getpixel((40, 40)) != image.getpixel((540, 540))
    evidence = json.loads(evidence_path.read_text(encoding="utf-8"))
    assert evidence["non_rendering_guide"] is True
    assert evidence["safe_content_frame"] == {"x1": 50, "y1": 100, "x2": 1030, "y2": 934}
    assert [region["id"] for region in evidence["hard_regions"]] == ["logo", "qr"]
