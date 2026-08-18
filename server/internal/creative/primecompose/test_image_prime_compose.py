import json
import tempfile
import unittest
from pathlib import Path

import qrcode
from PIL import Image, ImageDraw

from image_prime_compose import compose_manifest


SIZES = {
    "240x240": (240, 240),
    "300x150": (300, 150),
    "150x300": (150, 300),
}


class FullTemplateComposeTest(unittest.TestCase):
    def setUp(self) -> None:
        self.temp = tempfile.TemporaryDirectory()
        self.root = Path(self.temp.name)

    def tearDown(self) -> None:
        self.temp.cleanup()

    def write_template(self, path: Path, size: tuple[int, int], color: tuple[int, int, int, int], qr_payload: str = "") -> None:
        width, height = size
        template = Image.new("RGBA", size, (0, 0, 0, 0))
        draw = ImageDraw.Draw(template)
        band = max(4, height // 10)
        draw.rectangle((0, 0, width - 1, band - 1), fill=color)
        draw.rectangle((0, height - band, width - 1, height - 1), fill=color)
        if qr_payload:
            code = qrcode.QRCode(version=1, error_correction=qrcode.constants.ERROR_CORRECT_Q, box_size=2, border=1)
            code.add_data(qr_payload)
            code.make(fit=True)
            qr = code.make_image(fill_color="black", back_color="white").convert("RGBA")
            template.alpha_composite(qr, (width - qr.width - 4, max(0, (height - qr.height) // 2)))
        template.save(path)

    def write_family(self, family_id: str, color: tuple[int, int, int, int], high_resolution_square: bool = False, qr_payload: str = "") -> dict[str, Path]:
        paths: dict[str, Path] = {}
        for size, dimensions in SIZES.items():
            path = self.root / f"{family_id}-{size}.png"
            template_size = (480, 480) if high_resolution_square and size == "240x240" else dimensions
            self.write_template(path, template_size, color, qr_payload if size == "240x240" else "")
            paths[size] = path
        return paths

    def manifest(self, bodies: dict[str, Path], white: dict[str, Path], green: dict[str, Path]) -> dict:
        families = []
        sources: dict[str, str] = {}
        validations = []
        for family_id, label, paths in (("white_full", "White full", white), ("green_full", "Green full", green)):
            templates = {}
            validation_templates = {}
            for size, path in paths.items():
                role = f"{family_id}_{size.replace('x', '_')}"
                templates[size] = {"source_role": role}
                validation_templates[size] = {"source_role": role}
                sources[role] = path.name
            families.append({"id": family_id, "label": label, "description": label, "templates": templates})
            validations.append({"id": family_id, "label": label, "templates": validation_templates})
        layouts = {}
        jobs = []
        for size, (width, height) in SIZES.items():
            band = max(4, height // 10)
            layouts[size] = {
                "hard_regions": [],
                "top_key_content_exclusion_end": band,
                "bottom_key_content_exclusion_start": height - band,
            }
            jobs.append({"id": f"V01-{size}", "size": size, "input": bodies[size].name, "output": f"final-{size}.png"})
        return {
            "package_contract_version": 6,
            "variant_id": "variant-1",
            "variant_key": "V01",
            "revision": 1,
            "expected_sizes": list(SIZES),
            "prime_template_set": {"schema_version": 2, "selection_mode": "automatic_family_contrast", "families": families},
            "prime_template_set_validation": {"status": "passed", "families": validations},
            "prime_layout_contract": {"layouts": layouts},
            "sources": sources,
            "jobs": jobs,
        }

    def write_bodies(self, color: tuple[int, int, int, int]) -> dict[str, Path]:
        paths: dict[str, Path] = {}
        for size, dimensions in SIZES.items():
            path = self.root / f"body-{size}.png"
            Image.new("RGBA", dimensions, color).save(path)
            paths[size] = path
        return paths

    def compose(self, bodies: dict[str, Path], white: dict[str, Path], green: dict[str, Path]) -> dict:
        manifest_path = self.root / "manifest.json"
        manifest_path.write_text(json.dumps(self.manifest(bodies, white, green)), encoding="utf-8")
        return compose_manifest(manifest_path)

    def test_complete_templates_select_one_family_for_all_delivery_sizes(self) -> None:
        bodies = self.write_bodies((15, 30, 50, 255))
        result = self.compose(bodies, self.write_family("white", (255, 255, 255, 255)), self.write_family("green", (0, 130, 70, 255)))

        self.assertEqual(result["failed"], 0)
        selected_families = {item["template"]["family_id"] for item in result["results"]}
        self.assertEqual(selected_families, {"white_full"})
        self.assertEqual({item["template_selection"]["selected_family_id"] for item in result["results"]}, {"white_full"})
        self.assertEqual(result["results"][0]["template_selection"]["selection_reason"], "highest_minimum_key_band_contrast_across_all_delivery_sizes")
        self.assertTrue(all(item["compose"]["template_application"] == "unchanged_full_canvas_alpha_composite" for item in result["results"]))
        with Image.open(self.root / "final-240x240.png") as final:
            self.assertEqual(final.getpixel((120, 120)), (15, 30, 50, 255))

    def test_family_selection_uses_the_worst_readability_across_all_sizes(self) -> None:
        bodies = self.write_bodies((248, 248, 248, 255))
        result = self.compose(bodies, self.write_family("white", (255, 255, 255, 255)), self.write_family("green", (0, 130, 70, 255)))

        self.assertEqual(result["failed"], 0)
        self.assertEqual({item["template"]["family_id"] for item in result["results"]}, {"green_full"})
        selection = result["results"][0]["template_selection"]
        self.assertEqual(len(selection["candidates"]), 2)
        self.assertEqual(selection["selected_family_id"], "green_full")

    def test_high_resolution_template_is_applied_to_the_generated_canvas(self) -> None:
        bodies = self.write_bodies((15, 30, 50, 255))
        result = self.compose(bodies, self.write_family("white", (255, 255, 255, 255), high_resolution_square=True), self.write_family("green", (0, 130, 70, 255)))

        self.assertEqual(result["failed"], 0)
        square = next(item for item in result["results"] if item["size"] == "240x240")
        resize = square["template"]["resize"]
        self.assertEqual(resize["source_canvas"], [480, 480])
        self.assertEqual(resize["applied_canvas"], [240, 240])
        self.assertEqual(resize["resize_method"], "lanczos")

    def test_template_qr_is_decoded_as_optional_evidence(self) -> None:
        payload = "https://example.com/qr"
        bodies = self.write_bodies((15, 30, 50, 255))
        result = self.compose(
            bodies,
            self.write_family("white", (255, 255, 255, 255), qr_payload=payload),
            self.write_family("green", (0, 130, 70, 255)),
        )

        self.assertEqual(result["failed"], 0)
        square = next(item for item in result["results"] if item["size"] == "240x240")
        self.assertTrue(square["qr"]["detected"])
        self.assertEqual(square["qr"]["decoded"], payload)
        self.assertEqual(square["qr"]["validation_basis"], "optional_template_qr_decode")

    def test_published_full_template_set_supports_a_frozen_size_subset(self) -> None:
        bodies = self.write_bodies((15, 30, 50, 255))
        white = self.write_family("white", (255, 255, 255, 255))
        green = self.write_family("green", (0, 130, 70, 255))
        manifest = self.manifest(bodies, white, green)
        manifest["expected_sizes"] = ["240x240", "300x150"]
        manifest["jobs"] = [job for job in manifest["jobs"] if job["size"] in manifest["expected_sizes"]]
        manifest_path = self.root / "subset.json"
        manifest_path.write_text(json.dumps(manifest), encoding="utf-8")

        result = compose_manifest(manifest_path)

        self.assertEqual(result["failed"], 0)
        self.assertEqual(result["succeeded"], 2)
        self.assertEqual(result["expected_sizes"], ["240x240", "300x150"])
        self.assertEqual({item["size"] for item in result["results"]}, {"240x240", "300x150"})

    def test_package_without_complete_families_is_rejected(self) -> None:
        path = self.root / "incomplete.json"
        path.write_text(json.dumps({"package_contract_version": 6, "expected_sizes": ["240x240"]}), encoding="utf-8")

        with self.assertRaisesRegex(ValueError, "prime_template_set"):
            compose_manifest(path)


if __name__ == "__main__":
    unittest.main()
