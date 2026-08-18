import tempfile
import unittest
from pathlib import Path

import qrcode
from PIL import Image

from qc_batch import decode_qr_evidence, package_contract_failures, resolve_layout_contract


LAYOUT = {
    "hard_regions": [{"id": "white_full:header", "kind": "template", "x1": 0, "y1": 0, "x2": 800, "y2": 80}],
    "top_key_content_exclusion_end": 80,
    "bottom_key_content_exclusion_start": 920,
}


def package(selection: dict | None = None) -> tuple[dict, dict]:
    manifest = {
        "package_contract_version": 6,
        "variant_id": "variant-1",
        "variant_key": "V01",
        "revision": 1,
        "expected_sizes": ["800x1000"],
        "prime_template_set": {"schema_version": 2, "selection_mode": "automatic_family_contrast", "families": [
            {"id": "white_full", "label": "White full", "description": "White full", "templates": {"800x1000": {"source_role": "portrait_white"}}}
        ]},
        "prime_layout_contract": {"layouts": {"800x1000": LAYOUT}},
        "jobs": [{"id": "V01-800x1000", "variant_id": "variant-1", "variant_key": "V01", "revision": 1, "size": "800x1000", "output": "final.png"}],
    }
    selection = selection or {"selection_mode": "automatic_family_contrast", "selected_family_id": "white_full", "selected_source_role": "portrait_white", "candidates": [{"family_id": "white_full", "sizes": {"800x1000": {"source_role": "portrait_white"}}}]}
    compose = {
        "package_contract_version": 6,
        "package_contract_valid": True,
        "variant_id": "variant-1",
        "variant_key": "V01",
        "revision": 1,
        "expected_sizes": ["800x1000"],
        "succeeded": 1,
        "failed": 0,
        "results": [{"id": "V01-800x1000", "size": "800x1000", "revision": 1, "variant_key": "V01", "output": "final.png", "canvas": {"width": 800, "height": 1000}, "template_selection": selection, "compose": {"mode": "full_transparent_template", "template_application": "unchanged_full_canvas_alpha_composite"}}],
    }
    return manifest, compose


class QCBatchContractTest(unittest.TestCase):
    def test_resolves_published_template_layout(self) -> None:
        self.assertEqual(resolve_layout_contract({"prime_layout_contract": {"layouts": {"800x1000": LAYOUT}}}, {}, {}, "800x1000"), LAYOUT)

    def test_contract_v6_accepts_valid_template_selection_without_qr(self) -> None:
        manifest, compose = package()
        with tempfile.TemporaryDirectory() as directory:
            failures = package_contract_failures(manifest, compose, Path(directory))
        self.assertEqual(failures, [])

    def test_contract_v6_rejects_unpublished_selected_template(self) -> None:
        selection = {"selection_mode": "automatic_family_contrast", "selected_family_id": "green_full", "selected_source_role": "portrait_green", "candidates": [{"family_id": "green_full", "sizes": {"800x1000": {"source_role": "portrait_green"}}}]}
        manifest, compose = package(selection)
        with tempfile.TemporaryDirectory() as directory:
            failures = package_contract_failures(manifest, compose, Path(directory))
        self.assertIn("V01-800x1000:template_selection_invalid", failures)

    def test_qr_decode_is_optional_evidence(self) -> None:
        payload = "https://example.com/qr"
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "qr.png"
            code = qrcode.QRCode(version=1, error_correction=qrcode.constants.ERROR_CORRECT_Q, box_size=8, border=4)
            code.add_data(payload)
            code.make(fit=True)
            code.make_image(fill_color="black", back_color="white").save(path)
            evidence = decode_qr_evidence(path)
            self.assertTrue(evidence["detected"])
            self.assertEqual(evidence["decoded"], payload)

            Image.new("RGB", (160, 160), "white").save(path)
            empty = decode_qr_evidence(path)
            self.assertFalse(empty["detected"])
            self.assertEqual(empty["decoded"], "")

if __name__ == "__main__":
    unittest.main()
