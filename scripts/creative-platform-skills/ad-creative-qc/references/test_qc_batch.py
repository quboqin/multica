import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

import cv2
import numpy as np
from PIL import Image

from qc_batch import (
    compose_item_succeeded,
    decode_qr_evidence,
    index_compose_results,
    package_contract_failures,
    qr_check_passed,
    resolve_layout_contract,
    resolve_job_contract,
)


LAYOUT = {
    "hard_regions": [{"id": "qr", "kind": "qr", "x1": 700, "y1": 20, "x2": 760, "y2": 80}],
    "top_key_content_exclusion_end": 80,
    "bottom_key_content_exclusion_start": 1000,
}


class QCBatchContractTest(unittest.TestCase):
    def test_rejects_archived_compose_result_without_explicit_pass_evidence(self) -> None:
        item = {
            "size_key": "1080x1080",
            "output": "1080x1080/final.png",
            "canvas": {"width": 1080, "height": 1080},
            "decoded": "https://example.test/terms",
        }
        compose = {"succeeded": 1, "failed": 0, "results": [item]}

        self.assertIs(index_compose_results(compose)["1080x1080"], item)
        self.assertFalse(compose_item_succeeded(compose, item))

    def test_does_not_relax_explicit_failed_compose_result(self) -> None:
        item = {
            "id": "1080x1080",
            "status": "failed",
            "passed": False,
            "output": "final.png",
            "canvas": {"width": 1080, "height": 1080},
        }
        compose = {"succeeded": 1, "failed": 0, "results": [item]}

        self.assertFalse(compose_item_succeeded(compose, item))

    def test_reads_size_from_variant_prefixed_manifest_job_id(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            contract = resolve_job_contract(
                {"id": "V02-1200x628", "output": "final.png", "qr_payload": "https://example.test/terms"},
                {"canvas": {"width": 1200, "height": 628}},
                {"revision": 1, "variant_key": "V02"},
                Path(directory),
            )

        self.assertEqual(contract["size"], "1200x628")
        self.assertEqual(contract["variant"], "V02")
        self.assertEqual(contract["qr_mode"], "dynamic")

    def test_resolves_layout_contract_from_compose_result(self) -> None:
        layout = resolve_layout_contract({}, {}, {"layout_contract": LAYOUT}, "800x1000")

        self.assertEqual(layout, LAYOUT)

    def test_rejects_malformed_compose_layout_even_when_manifest_has_a_valid_fallback(self) -> None:
        layout = resolve_layout_contract(
            {"prime_layout_contract": {"layouts": {"800x1000": LAYOUT}}},
            {},
            {"layout_contract": {"hard_regions": []}},
            "800x1000",
        )

        self.assertIsNone(layout)

    def test_compatible_legacy_package_uses_compose_layout_and_id_suffix(self) -> None:
        manifest = {
            "variant_key": "V03",
            "revision": 1,
            "jobs": [{"id": "V03-800x1000", "output": "V03_800x1000_r1.png"}],
        }
        compose = {
            "succeeded": 1,
            "failed": 0,
            "results": [{
                "id": "V03-800x1000",
                "status": "succeeded",
                "passed": True,
                "output": "V03_800x1000_r1.png",
                "canvas": {"width": 800, "height": 1000},
                "layout_contract": LAYOUT,
                "decoded": "",
            }],
        }
        with tempfile.TemporaryDirectory() as directory:
            failures = package_contract_failures(manifest, compose, Path(directory))

        self.assertEqual(failures, [])

    def test_package_without_any_layout_contract_is_not_reusable(self) -> None:
        manifest = {"revision": 1, "jobs": [{"id": "V03-800x1000", "output": "final.png"}]}
        compose = {
            "succeeded": 1,
            "failed": 0,
            "results": [{
                "id": "V03-800x1000",
                "status": "succeeded",
                "passed": True,
                "output": "final.png",
                "canvas": {"width": 800, "height": 1000},
            }],
        }
        with tempfile.TemporaryDirectory() as directory:
            failures = package_contract_failures(manifest, compose, Path(directory))

        self.assertIn("V03-800x1000:layout_contract_missing_or_conflicting", failures)

    def test_independent_qr_decode_retries_padded_hard_region_crop_at_2x(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "final.png"
            Image.new("RGB", (800, 1000), "white").save(path)

            class CropOnlyDetector:
                def detectAndDecode(self, image: np.ndarray):
                    if image.shape[:2] == (168, 168):
                        return "https://example.test/terms", None, None
                    return "", None, None

            with patch.object(cv2, "QRCodeDetector", return_value=CropOnlyDetector()):
                evidence = decode_qr_evidence(path, LAYOUT)

        self.assertEqual(evidence["decoded"], "https://example.test/terms")
        self.assertEqual(evidence["successful_attempt"], "hard_region_crop_2x_nearest")

    def test_no_qr_mode_requires_no_payload_and_no_decoded_qr(self) -> None:
        self.assertTrue(qr_check_passed("none", "", "", {"qr_mode": "none"}))
        self.assertFalse(qr_check_passed("none", "", "https://unexpected.test", {"qr_mode": "none"}))


if __name__ == "__main__":
    unittest.main()
