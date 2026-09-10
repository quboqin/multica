import copy
import tempfile
import unittest
from pathlib import Path

from qc_batch import package_contract_failures, resolve_layout_contract, skipped_qr_evidence


LAYOUT = {
    "hard_regions": [{"id": "white_full:header", "kind": "template", "x1": 0, "y1": 0, "x2": 800, "y2": 80}],
    "top_key_content_exclusion_end": 80,
    "bottom_key_content_exclusion_start": 920,
}


def valid_selection(family_id: str = "white_full", source_role: str = "portrait_white") -> dict:
    details = {
        "source_role": source_role,
        "foreground_polarity": "light",
        "visible_component_mask": {
            "source": "template_alpha_and_dominant_foreground_polarity",
            "alpha_threshold": 0.08,
            "visible_pixels": 240,
            "all_visible_template_pixels": 280,
            "coverage": 0.003,
            "foreground_polarity": "light",
            "foreground_relative_luminance_median": 0.92,
        },
        "background_support": {
            "polarity": "dark",
            "relative_luminance_contrast": {
                "minimum_local_p10": 3.8,
                "threshold": 2.2,
                "basis": "alpha_composited_template_over_generated_body",
            },
            "polarity_match": {"minimum_local_ratio": 0.93, "threshold": 0.85},
            "texture": {
                "maximum_local_p90": 0.02,
                "threshold": 0.08,
                "metric": "relative_luminance_neighbor_difference",
            },
        },
        "visual_adequacy": {"adequate": True, "inadequacy_codes": []},
    }
    return {
        "size": "800x1000",
        "selection_scope": "delivery_size",
        "selection_mode": "automatic_family_contrast",
        "selected_family_id": family_id,
        "selected_source_role": source_role,
        "foreground_polarity": "light",
        "background_polarity": "dark",
        "minimum_relative_luminance_contrast": 3.8,
        "visual_adequacy": {"adequate": True, "inadequacy_codes": []},
        "candidates": [{"family_id": family_id, **details, "sizes": {"800x1000": copy.deepcopy(details)}}],
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
    selection = selection or valid_selection()
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
        selection = valid_selection("green_full", "portrait_green")
        manifest, compose = package(selection)
        with tempfile.TemporaryDirectory() as directory:
            failures = package_contract_failures(manifest, compose, Path(directory))
        self.assertIn("V01-800x1000:template_selection_invalid", failures)

    def test_contract_v6_rejects_legacy_selection_without_structured_adequacy(self) -> None:
        legacy = {
            "selection_mode": "automatic_family_contrast",
            "selected_family_id": "white_full",
            "selected_source_role": "portrait_white",
            "candidates": [{"family_id": "white_full", "sizes": {"800x1000": {"source_role": "portrait_white"}}}],
        }
        manifest, compose = package(legacy)
        with tempfile.TemporaryDirectory() as directory:
            failures = package_contract_failures(manifest, compose, Path(directory))
        self.assertIn("V01-800x1000:template_selection_scope_invalid", failures)
        self.assertIn("V01-800x1000:template_visual_adequacy_not_passed", failures)
        self.assertIn("V01-800x1000:template_foreground_polarity_invalid", failures)
        self.assertIn("V01-800x1000:template_visible_component_mask_invalid", failures)
        self.assertIn("V01-800x1000:template_background_polarity_invalid", failures)

    def test_contract_v6_rejects_invalid_selected_candidate_metrics(self) -> None:
        cases = [
            ("contrast basis", ("background_support", "relative_luminance_contrast", "basis"), "background_only_rgb_distance", "template_relative_luminance_contrast_invalid"),
            ("contrast threshold", ("background_support", "relative_luminance_contrast", "minimum_local_p10"), 1.2, "template_relative_luminance_contrast_invalid"),
            ("polarity evidence", ("background_support", "polarity_match", "minimum_local_ratio"), 0.4, "template_background_polarity_evidence_invalid"),
            ("texture threshold", ("background_support", "texture", "maximum_local_p90"), 0.2, "template_background_texture_invalid"),
        ]
        for label, path, value, expected in cases:
            with self.subTest(label=label):
                selection = valid_selection()
                candidate = selection["candidates"][0]
                size_evidence = candidate["sizes"]["800x1000"]
                target = size_evidence
                for key in path[:-1]:
                    target = target[key]
                target[path[-1]] = value
                candidate[path[0]] = copy.deepcopy(size_evidence[path[0]])
                manifest, compose = package(selection)
                with tempfile.TemporaryDirectory() as directory:
                    failures = package_contract_failures(manifest, compose, Path(directory))
                self.assertIn(f"V01-800x1000:{expected}", failures)

    def test_qr_decode_is_skipped_evidence(self) -> None:
        evidence = skipped_qr_evidence()
        self.assertTrue(evidence["skipped"])
        self.assertEqual(evidence["reason"], "qr_decode_disabled")
        self.assertFalse(evidence["detected"])
        self.assertEqual(evidence["decoded"], "")

if __name__ == "__main__":
    unittest.main()
