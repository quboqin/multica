import copy
import unittest

from prepare_image_operation import prepare_operation


class PrepareOperationTest(unittest.TestCase):
    def fixture(self, status="failed"):
        op = {"revision": 1, "size_key": "1200x628", "operation_kind": "generation",
              "idempotency_key": "frozen-key", "model": "gpt-image-2", "status": status,
              "input_snapshot": {"input_asset_fingerprints": {"prime": "original"}},
              "attempts": [{"attempt": 1}, {"attempt": 5}], "prompt_sha256": "original-prompt"}
        variant = {"id": "variant", "revision": 1, "candidate_state": "selected", "image_operations": [op]}
        draft = {"variant_id": "variant", "revision": 1, "size_key": "1200x628", "operation_kind": "generation",
                 "attempt": 1, "model": "wrong-model", "input_snapshot": {"replacement": True}}
        return {"items": [{"variants": [variant]}]}, draft

    def test_retry_uses_next_attempt_and_frozen_inputs(self):
        order, draft = self.fixture()
        before = copy.deepcopy(order)
        prepared = prepare_operation(order, draft)
        self.assertEqual(prepared["attempt"], 6)
        self.assertEqual(prepared["model"], "gpt-image-2")
        self.assertEqual(prepared["idempotency_key"], "frozen-key")
        self.assertEqual(prepared["input_snapshot"], order["items"][0]["variants"][0]["image_operations"][0]["input_snapshot"])
        self.assertEqual(order, before)

    def test_unsettled_and_successful_operations_cannot_be_reinvoked(self):
        for status in ("running", "unknown", "completed", "cancelled"):
            with self.subTest(status=status):
                with self.assertRaises(ValueError):
                    prepare_operation(*self.fixture(status))

    def test_retry_preserves_frozen_image_model_and_quality(self):
        order, draft = self.fixture()
        op = order["items"][0]["variants"][0]["image_operations"][0]
        op["model"] = "gpt-image-2.5-sunburst"
        op["input_snapshot"]["image_generation"] = {"model": op["model"], "quality": "xhigh"}
        prepared = prepare_operation(order, draft)
        self.assertEqual(prepared["model"], "gpt-image-2.5-sunburst")
        self.assertEqual(prepared["input_snapshot"]["image_generation"]["quality"], "xhigh")

    def test_new_operation_always_starts_at_one(self):
        order, draft = self.fixture()
        order["items"][0]["variants"][0]["image_operations"] = []
        draft["attempt"] = 6
        self.assertEqual(prepare_operation(order, draft)["attempt"], 1)

    def test_changed_revision_stops_before_any_request(self):
        order, draft = self.fixture()
        draft["revision"] = 2
        with self.assertRaises(ValueError):
            prepare_operation(order, draft)


if __name__ == "__main__":
    unittest.main()
