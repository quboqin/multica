import tempfile
import unittest
from pathlib import Path

from PIL import Image, ImageDraw

from expand_background import compose, pixel_hash, prepare


class BackgroundExpansionTest(unittest.TestCase):
    def test_model_changes_cannot_overwrite_protected_design(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            source = Image.new("RGB", (320, 320), "navy")
            ImageDraw.Draw(source).text((70, 70), "RM 1,000 / 3 Bulan", fill="white")
            source.save(root / "source.png")
            layout = {"__canvas_width": 320, "__canvas_height": 320,
                      "safe_content_frame": {"x1": 16, "y1": 40, "x2": 304, "y2": 280}}
            info = prepare(root / "source.png", layout, (320, 320), root / "job")
            Image.new("RGB", (352, 352), "red").save(root / "model.png")
            final = compose(root / "job", root / "model.png", root / "final.png")
            with Image.open(root / "final.png") as image:
                self.assertEqual(pixel_hash(image.crop(info["body_box"])), info["body_pixel_sha256"])
                self.assertEqual(image.getpixel((0, 0)), (255, 0, 0))
            with Image.open(root / "job" / "mask.png") as mask:
                self.assertEqual(mask.getpixel((0, 0))[3], 0)
                self.assertEqual(mask.getpixel((160, 160))[3], 255)
            self.assertTrue(final["body_preserved"])
            self.assertFalse(final["text_rendered_by_script"])

    def test_rejects_excessive_shrinking_and_changed_body(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            Image.new("RGB", (320, 320), "navy").save(root / "source.png")
            layout = {"__canvas_width": 320, "__canvas_height": 320,
                      "safe_content_frame": {"x1": 0, "y1": 100, "x2": 320, "y2": 200}}
            with self.assertRaises(ValueError):
                prepare(root / "source.png", layout, (320, 320), root / "job")


if __name__ == "__main__":
    unittest.main()
