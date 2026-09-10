import json
from pathlib import Path
import tempfile
import unittest

from register_process_assets import process_metadata


class PrimeContextMetadataTest(unittest.TestCase):
    def test_guide_evidence_travels_with_actual_context_attachment(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "prime-context-1080x1080.png"
            guide = {"size_key": "1080x1080", "prime_template_source_role": "square_dark", "template_alpha": 1.0}
            path.with_suffix(".json").write_text(json.dumps(guide))
            metadata = process_metadata("1080x1080", "Prime context", path)
            self.assertEqual(metadata["prime_template_source_role"], "square_dark")
            self.assertEqual(metadata["prime_context"], guide)
            self.assertNotIn("prime_template_source_role", process_metadata("1080x1080", "模型原图", path))

    def test_wrong_size_and_missing_role_stop_before_upload(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "context.png"
            for guide in ({"size_key": "1200x628", "prime_template_source_role": "landscape_dark"}, {"size_key": "1080x1080"}):
                path.with_suffix(".json").write_text(json.dumps(guide))
                with self.assertRaises(ValueError):
                    process_metadata("1080x1080", "Prime context", path)

    def test_legacy_process_image_without_guide_keeps_existing_metadata(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "context.png"
            self.assertEqual(process_metadata("1080x1080", "Prime context", path), {"process_stage": "Prime context", "source_filename": "context.png"})
