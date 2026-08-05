import json
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

import cv2
import numpy as np
from PIL import Image, ImageDraw

from extract_prime_components import extract_manifest
from image_prime_compose import (
    compile_layout_contract,
    compose_manifest,
    compose_v2,
    contain_layer,
    qr_decode_evidence,
    render_qr,
)


PAYLOAD = "https://www.adakami.id/termsandconditions"


def composition(qr_mode: str, components: list[dict], layouts: dict) -> dict:
    return {
        "schema_version": 2,
        "qr_mode": qr_mode,
        "components": components,
        "layouts": layouts,
    }


class PrimeV2CompositionTest(unittest.TestCase):
    def setUp(self) -> None:
        self.temp = tempfile.TemporaryDirectory()
        self.root = Path(self.temp.name)
        self.body = self.root / "body.png"
        Image.new("RGB", (1080, 1080), (210, 40, 40)).save(self.body)

    def tearDown(self) -> None:
        self.temp.cleanup()

    def test_image_component_contains_standalone_source_without_cropping(self) -> None:
        logo = self.root / "logo.png"
        output = self.root / "image.png"
        Image.new("RGBA", (200, 100), (0, 120, 70, 255)).save(logo)
        config = composition(
            "none",
            [{"id": "logo", "label": "Logo", "kind": "image", "enabled": True, "source_role": "prime_logo"}],
            {"1080x1080": {"components": {"logo": {"destination_rect": [20, 20, 120, 120]}}}},
        )

        result = compose_v2(self.body, output, config, {"prime_logo": logo}, "")

        component = result["components"][0]
        self.assertEqual(component["rendered_rect"], [20, 45, 120, 95])
        self.assertEqual(component["fit"], "contain")
        self.assertNotIn("source_rect", component)
        with Image.open(output) as image:
            self.assertEqual(image.getpixel((50, 30)), (210, 40, 40))
            self.assertEqual(image.getpixel((50, 60)), (0, 120, 70))

    def test_text_component_renders_content_with_default_style(self) -> None:
        output = self.root / "text.png"
        config = composition(
            "none",
            [{"id": "terms", "label": "Terms", "kind": "text", "enabled": True, "content": "Terms apply"}],
            {"1080x1080": {"components": {"terms": {"destination_rect": [100, 100, 500, 220]}}}},
        )

        result = compose_v2(self.body, output, config, {}, "")

        component = result["components"][0]
        self.assertEqual(component["content"], "Terms apply")
        self.assertGreaterEqual(component["font_size"], 6)
        with Image.open(output) as image:
            colors = image.crop((100, 100, 500, 220)).getcolors(maxcolors=10000)
            self.assertGreater(len(colors or []), 1)

    def test_static_qr_reuses_one_standalone_resource_and_decodes_it(self) -> None:
        qr_path = self.root / "qr.png"
        output = self.root / "static.png"
        render_qr(PAYLOAD, box_size=5).save(qr_path)
        config = composition(
            "static",
            [{"id": "qr", "label": "QR", "kind": "qr", "enabled": True, "source_role": "prime_qr"}],
            {"1080x1080": {"components": {"qr": {"destination_rect": [700, 20, 1040, 360]}}}},
        )

        result = compose_v2(self.body, output, config, {"prime_qr": qr_path}, PAYLOAD)

        self.assertTrue(result["passed"])
        self.assertEqual(result["decoded"], PAYLOAD)
        self.assertEqual(result["components"][0]["source_role"], "prime_qr")

    def test_dynamic_qr_generates_and_decodes_approved_payload(self) -> None:
        output = self.root / "dynamic.png"
        config = composition(
            "dynamic",
            [{"id": "qr", "label": "QR", "kind": "qr", "enabled": True}],
            {"1080x1080": {"components": {"qr": {"destination_rect": [700, 20, 1040, 360]}}}},
        )

        result = compose_v2(self.body, output, config, {}, PAYLOAD)

        self.assertTrue(result["passed"])
        self.assertEqual(result["decoded"], PAYLOAD)
        self.assertEqual(result["components"][0]["source_role"], "")

    def test_static_qr_remains_decodable_in_all_delivery_rectangles(self) -> None:
        qr_path = self.root / "shared-qr.png"
        render_qr(PAYLOAD, box_size=9).save(qr_path)
        rectangles = {
            "1080x1080": [983, 29, 1053, 99],
            "1200x628": [1133, 20, 1185, 73],
            "800x1000": [724, 22, 779, 76],
        }
        config = composition(
            "static",
            [{"id": "qr", "label": "QR", "kind": "qr", "enabled": True, "source_role": "prime_qr"}],
            {size: {"components": {"qr": {"destination_rect": rect}}} for size, rect in rectangles.items()},
        )

        for size in rectangles:
            width, height = map(int, size.split("x"))
            body = self.root / f"qr-body-{size}.png"
            output = self.root / f"qr-output-{size}.png"
            Image.new("RGB", (width, height), "white").save(body)
            with self.subTest(size=size):
                result = compose_v2(body, output, config, {"prime_qr": qr_path}, PAYLOAD)
                self.assertTrue(result["passed"])
                if size == "800x1000":
                    self.assertEqual(result["components"][0]["rendered_rect"], [725, 23, 777, 75])
                    self.assertEqual(result["successful_attempt"], "slot_crop_2x_nearest")

    def test_small_qr_uses_enlarged_slot_crop_and_maps_points_to_full_frame(self) -> None:
        final = np.zeros((1000, 800, 3), dtype=np.uint8)
        rect = [724, 22, 778, 76]

        class CropOnlyDetector:
            def detectAndDecode(self, image: np.ndarray):
                if image.shape[:2] == (156, 156):
                    points = np.array([[[24, 28], [128, 28], [128, 132], [24, 132]]], dtype=np.float32)
                    return PAYLOAD, points, None
                return "", None, None

        with patch.object(cv2, "QRCodeDetector", return_value=CropOnlyDetector()):
            evidence = qr_decode_evidence(final, PAYLOAD, rect)

        self.assertTrue(evidence["passed"])
        self.assertEqual(evidence["successful_attempt"], "slot_crop_2x_nearest")
        self.assertEqual(
            evidence["detected_points"],
            [[724.0, 24.0], [776.0, 24.0], [776.0, 76.0], [724.0, 76.0]],
        )
        self.assertEqual(
            list(evidence["decode_attempts"]),
            ["full_frame_1x", "full_frame_2x_nearest", "slot_crop_1x", "slot_crop_2x_nearest"],
        )

    def test_contain_layer_uses_nearest_only_for_static_qr(self) -> None:
        source = Image.new("RGBA", (2, 2))
        source.putdata([
            (0, 0, 0, 255),
            (255, 255, 255, 255),
            (255, 255, 255, 255),
            (0, 0, 0, 255),
        ])

        qr_layer, qr_rect = contain_layer(source, 5, 5, qr=True)
        image_layer, image_rect = contain_layer(source, 5, 5)

        expected_qr = source.resize((5, 5), Image.Resampling.NEAREST)
        expected_image = source.resize((5, 5), Image.Resampling.LANCZOS)
        self.assertEqual(qr_rect, [0, 0, 5, 5])
        self.assertEqual(image_rect, [0, 0, 5, 5])
        self.assertEqual(np.asarray(qr_layer).tolist(), np.asarray(expected_qr).tolist())
        self.assertEqual(np.asarray(image_layer).tolist(), np.asarray(expected_image).tolist())

    def test_manifest_reuses_same_component_sources_across_three_sizes(self) -> None:
        logo = self.root / "logo.png"
        qr_path = self.root / "qr.png"
        Image.new("RGBA", (240, 80), (0, 120, 70, 255)).save(logo)
        render_qr(PAYLOAD, box_size=5).save(qr_path)
        sizes = [(1080, 1080), (1200, 628), (800, 1000)]
        layouts = {}
        jobs = []
        for width, height in sizes:
            size = f"{width}x{height}"
            body = self.root / f"body-{size}.png"
            Image.new("RGB", (width, height), "white").save(body)
            layouts[size] = {
                "components": {
                    "logo": {"destination_rect": [20, 20, 260, 100]},
                    "qr": {"destination_rect": [width - 260, 20, width - 20, 260]},
                }
            }
            jobs.append({
                "id": f"V01-{size}",
                "variant_id": "variant-1",
                "variant_key": "V01",
                "size": size,
                "revision": 2,
                "input": body.name,
                "output": f"out-{size}.png",
            })
        prime_composition = composition(
            "static",
            [
                {"id": "logo", "label": "Logo", "kind": "image", "enabled": True, "source_role": "prime_logo"},
                {"id": "qr", "label": "QR", "kind": "qr", "enabled": True, "source_role": "prime_qr"},
            ],
            layouts,
        )
        components = prime_composition["components"]
        contract_layouts = {
            size: compile_layout_contract(
                components,
                layouts[size],
                "static",
                tuple(int(value) for value in size.split("x")),
            )
            for size in layouts
        }
        manifest = {
            "package_contract_version": 1,
            "variant_id": "variant-1",
            "variant_key": "V01",
            "revision": 2,
            "expected_sizes": list(layouts),
            "prime_composition": prime_composition,
            "prime_layout_contract": {"layouts": contract_layouts},
            "qr_validation": {"status": "passed", "mode": "static", "approved_payload": PAYLOAD},
            "sources": {"prime_logo": logo.name, "prime_qr": qr_path.name},
            "jobs": jobs,
        }
        manifest_path = self.root / "manifest.json"
        manifest_path.write_text(json.dumps(manifest), encoding="utf-8")

        result = compose_manifest(manifest_path)

        self.assertEqual(result["succeeded"], 3)
        self.assertEqual(result["failed"], 0)
        self.assertTrue(result["package_contract_valid"])
        self.assertEqual(result["revision"], 2)
        for job in result["results"]:
            self.assertEqual(job["variant_id"], "variant-1")
            self.assertEqual(job["revision"], 2)
            self.assertIn(job["size"], manifest["expected_sizes"])
            self.assertEqual([item["source_role"] for item in job["components"]], ["prime_logo", "prime_qr"])
            self.assertEqual(job["decoded"], PAYLOAD)
            self.assertEqual(job["layout_contract"], contract_layouts[job["size"]])

    def test_manifest_rejects_legacy_full_template_mode(self) -> None:
        manifest_path = self.root / "legacy.json"
        manifest_path.write_text(json.dumps({
            "package_contract_version": 1,
            "variant_id": "variant-1",
            "variant_key": "V01",
            "revision": 1,
            "expected_sizes": ["1080x1080"],
            "jobs": [{
                "id": "V01-1080x1080",
                "variant_id": "variant-1",
                "variant_key": "V01",
                "size": "1080x1080",
                "revision": 1,
                "input": "a.png",
                "output": "b.png",
            }],
        }))

        result = compose_manifest(manifest_path)

        self.assertEqual(result["failed"], 1)
        self.assertFalse(result["package_contract_valid"])
        self.assertIn("legacy mode is unsupported", result["package_error"])

    def test_manifest_rejects_job_without_explicit_size_and_revision(self) -> None:
        manifest_path = self.root / "missing-job-contract.json"
        manifest_path.write_text(json.dumps({
            "package_contract_version": 1,
            "variant_id": "variant-1",
            "variant_key": "V01",
            "revision": 1,
            "expected_sizes": ["1080x1080"],
            "prime_composition": composition("none", [{
                "id": "logo", "kind": "image", "enabled": False, "source_role": "prime_logo",
            }], {"1080x1080": {"components": {}}}),
            "prime_layout_contract": {"layouts": {"1080x1080": {
                "hard_regions": [],
                "top_key_content_exclusion_end": 0,
                "bottom_key_content_exclusion_start": 1080,
            }}},
            "jobs": [{"id": "V01-1080x1080", "input": self.body.name, "output": "out.png"}],
        }))

        result = compose_manifest(manifest_path)

        self.assertFalse(result["package_contract_valid"])
        self.assertIn("cover each expected size", result["package_error"])

    def test_migration_extracts_transparent_asset_but_preserves_enclosed_white(self) -> None:
        sheet = self.root / "sheet.png"
        output = self.root / "logo.png"
        image = Image.new("RGB", (200, 100), "white")
        draw = ImageDraw.Draw(image)
        draw.rectangle((40, 20, 159, 79), fill=(0, 120, 70))
        draw.rectangle((80, 35, 119, 64), fill="white")
        image.save(sheet)
        manifest = self.root / "extract.json"
        manifest.write_text(json.dumps({
            "source": sheet.name,
            "authored_size": [200, 100],
            "components": [{
                "role": "prime_logo",
                "source_rect": [20, 10, 180, 90],
                "output": output.name,
                "background": "transparent",
            }],
        }), encoding="utf-8")

        result = extract_manifest(manifest)

        self.assertEqual(result["components"][0]["role"], "prime_logo")
        with Image.open(output) as extracted:
            self.assertEqual(extracted.getpixel((0, 0))[3], 0)
            self.assertEqual(extracted.getpixel((80, 40)), (255, 255, 255, 255))


if __name__ == "__main__":
    unittest.main()
