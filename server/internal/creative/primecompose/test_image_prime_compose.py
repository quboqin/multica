import json
import tempfile
import unittest
from pathlib import Path

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

    def write_template(
        self,
        path: Path,
        size: tuple[int, int],
        color: tuple[int, int, int, int],
        solid_bands: bool = False,
    ) -> None:
        width, height = size
        template = Image.new("RGBA", size, (0, 0, 0, 0))
        draw = ImageDraw.Draw(template)
        band = max(4, height // 10)
        if solid_bands:
            draw.rectangle((0, 0, width - 1, band - 1), fill=color)
            draw.rectangle((0, height - band, width - 1, height - 1), fill=color)
        else:
            stroke = max(2, band // 3)
            draw.rectangle((width // 20, stroke, width * 7 // 20, min(band - 1, stroke * 2)), fill=color)
            draw.rectangle((width * 11 // 20, height - band + stroke, width * 19 // 20, min(height - 1, height - band + stroke * 2)), fill=color)
        template.save(path)

    def write_family(
        self,
        family_id: str,
        color: tuple[int, int, int, int],
        high_resolution_square: bool = False,
        solid_bands: bool = False,
    ) -> dict[str, Path]:
        paths: dict[str, Path] = {}
        for size, dimensions in SIZES.items():
            path = self.root / f"{family_id}-{size}.png"
            template_size = (480, 480) if high_resolution_square and size == "240x240" else dimensions
            self.write_template(path, template_size, color, solid_bands)
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
        return self.write_bodies_by_size({size: color for size in SIZES})

    def write_bodies_by_size(self, colors: dict[str, tuple[int, int, int, int]]) -> dict[str, Path]:
        paths: dict[str, Path] = {}
        for size, dimensions in SIZES.items():
            path = self.root / f"body-{size}.png"
            Image.new("RGBA", dimensions, colors[size]).save(path)
            paths[size] = path
        return paths

    def compose(self, bodies: dict[str, Path], white: dict[str, Path], green: dict[str, Path]) -> dict:
        manifest_path = self.root / "manifest.json"
        manifest_path.write_text(json.dumps(self.manifest(bodies, white, green)), encoding="utf-8")
        return compose_manifest(manifest_path)

    def test_dominant_bright_patch_reselects_an_approved_readable_family(self) -> None:
        bodies = self.write_bodies((15, 30, 50, 255))
        result = self.compose(
            bodies,
            self.write_family("white", (255, 255, 255, 255), solid_bands=True),
            self.write_family("green", (230, 245, 235, 255)),
        )

        self.assertEqual(result["failed"], 0)
        selected_families = {item["template"]["family_id"] for item in result["results"]}
        self.assertEqual(selected_families, {"green_full"})
        selection = result["results"][0]["template_selection"]
        self.assertEqual({item["template_selection"]["selected_family_id"] for item in result["results"]}, {"green_full"})
        self.assertEqual(selection["selection_reason"], "visual_adequacy_reselected_from_highest_contrast_family")
        self.assertEqual(selection["visual_adequacy"]["status"], "reselected")
        self.assertEqual(selection["visual_adequacy"]["contrast_preferred_family_id"], "white_full")
        self.assertTrue(selection["visual_adequacy"]["reselected_without_regenerating_base"])
        self.assertTrue(all(item["compose"]["template_application"] == "unchanged_full_canvas_alpha_composite" for item in result["results"]))
        with Image.open(self.root / "final-240x240.png") as final:
            self.assertEqual(final.getpixel((120, 120)), (15, 30, 50, 255))

    def test_fad6_v03_selects_each_size_by_its_actual_glyph_polarity(self) -> None:
        # Observed legacy RGB scores were dark/light: square 93.57/229.67,
        # landscape 245.08/44.01, portrait 57.05/204.00. The synthetic
        # fixture preserves that polarity split without depending on RGB
        # distance as the production gate.
        bodies = self.write_bodies_by_size({
            "240x240": (235, 248, 242, 255),
            "300x150": (15, 30, 50, 255),
            "150x300": (225, 245, 238, 255),
        })
        result = self.compose(bodies, self.write_family("white", (255, 255, 255, 255)), self.write_family("green", (0, 130, 70, 255)))

        self.assertEqual(result["failed"], 0)
        selected = {item["size"]: item["template_selection"]["selected_family_id"] for item in result["results"]}
        self.assertEqual(selected, {
            "240x240": "green_full",
            "300x150": "white_full",
            "150x300": "green_full",
        })
        self.assertTrue(all(item["template_selection"]["selection_scope"] == "delivery_size" for item in result["results"]))
        self.assertEqual(
            {item["size"]: item["template_selection"]["foreground_polarity"] for item in result["results"]},
            {"240x240": "dark", "300x150": "light", "150x300": "dark"},
        )
        self.assertEqual(
            {item["size"]: item["template_selection"]["support_requirement"] for item in result["results"]},
            {"240x240": "light_low_texture", "300x150": "dark_low_texture", "150x300": "light_low_texture"},
        )

    def test_one_size_without_an_adequate_template_fails_the_whole_package_closed(self) -> None:
        bodies = self.write_bodies_by_size({
            "240x240": (245, 245, 245, 255),
            "300x150": (15, 30, 50, 255),
            "150x300": (160, 160, 160, 255),
        })
        Image.new("RGBA", SIZES["240x240"], (255, 0, 0, 255)).save(self.root / "final-240x240.png")
        result = self.compose(
            bodies,
            self.write_family("white", (255, 255, 255, 255)),
            self.write_family("green", (0, 130, 70, 255)),
        )

        self.assertEqual(result["succeeded"], 0)
        self.assertEqual(result["failed"], 1)
        self.assertEqual(result["results"], [])
        self.assertFalse(any((self.root / f"final-{size}.png").exists() for size in SIZES))
        failed = result["failures"][0]
        self.assertEqual(failed["size"], "150x300")
        self.assertEqual(failed["error_code"], "prime_no_adequate_template_for_size")
        self.assertEqual(failed["template_selection"]["visual_adequacy"]["status"], "failed_no_adequate_template")
        selection_by_size = {item["size"]: item["template_selection"] for item in result["size_selections"]}
        self.assertEqual(selection_by_size["240x240"]["selected_family_id"], "green_full")
        self.assertEqual(selection_by_size["300x150"]["selected_family_id"], "white_full")
        self.assertEqual(selection_by_size["150x300"]["selected_family_id"], "")

    def test_legacy_validation_snapshot_without_polarity_metadata_remains_compatible(self) -> None:
        bodies = self.write_bodies((15, 30, 50, 255))
        white = self.write_family("white", (255, 255, 255, 255))
        green = self.write_family("green", (0, 130, 70, 255))
        manifest = self.manifest(bodies, white, green)
        for family in manifest["prime_template_set_validation"]["families"]:
            for validation in family["templates"].values():
                self.assertEqual(set(validation), {"source_role"})
        manifest_path = self.root / "legacy-validation.json"
        manifest_path.write_text(json.dumps(manifest), encoding="utf-8")

        result = compose_manifest(manifest_path)

        self.assertEqual(result["failed"], 0)
        self.assertEqual(result["succeeded"], 3)
        self.assertTrue(all(item["visibility_audit"]["visual_adequacy"]["adequate"] for item in result["results"]))

    def test_high_texture_under_the_visible_glyph_mask_is_rejected(self) -> None:
        bodies = self.write_bodies((250, 250, 250, 255))
        for size, path in bodies.items():
            width, height = SIZES[size]
            image = Image.new("RGBA", (width, height), (250, 250, 250, 255))
            draw = ImageDraw.Draw(image)
            for x in range(0, width, 2):
                draw.rectangle((x, 0, x, height - 1), fill=(25, 25, 25, 255))
            image.save(path)
        result = self.compose(
            bodies,
            self.write_family("white", (255, 255, 255, 255)),
            self.write_family("green", (0, 80, 40, 255)),
        )

        self.assertGreater(result["failed"], 0)
        codes = {
            code
            for failure in result["failures"]
            for candidate in failure["template_selection"]["candidates"]
            for code in candidate["visual_adequacy"]["inadequacy_codes"]
        }
        self.assertIn("prime_background_too_textured", codes)

    def test_high_resolution_template_is_applied_to_the_generated_canvas(self) -> None:
        bodies = self.write_bodies((15, 30, 50, 255))
        result = self.compose(bodies, self.write_family("white", (255, 255, 255, 255), high_resolution_square=True), self.write_family("green", (0, 80, 40, 255)))

        self.assertEqual(result["failed"], 0)
        square = next(item for item in result["results"] if item["size"] == "240x240")
        resize = square["template"]["resize"]
        self.assertEqual(resize["source_canvas"], [480, 480])
        self.assertEqual(resize["applied_canvas"], [240, 240])
        self.assertEqual(resize["resize_method"], "lanczos")

    def test_template_qr_decode_is_skipped(self) -> None:
        bodies = self.write_bodies((15, 30, 50, 255))
        result = self.compose(
            bodies,
            self.write_family("white", (255, 255, 255, 255)),
            self.write_family("green", (0, 130, 70, 255)),
        )

        self.assertEqual(result["failed"], 0)
        square = next(item for item in result["results"] if item["size"] == "240x240")
        self.assertFalse(square["qr"]["detected"])
        self.assertEqual(square["qr"]["decoded"], "")
        self.assertEqual(square["qr"]["validation_basis"], "template_owned_qr_decode_disabled")
        self.assertTrue(square["qr"]["skipped"])

    def test_published_full_template_set_supports_a_frozen_size_subset(self) -> None:
        bodies = self.write_bodies((245, 245, 245, 255))
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
