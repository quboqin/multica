import json
import sys
from pathlib import Path

import pytest

from validate_copy_snapshot import financial_tokens, main, validate_snapshot


def approved_snapshot() -> dict:
    return {
        "schema_version": 2,
        "id": "recipe-num",
        "library_id": "library-1",
        "library_version": 3,
        "recipe_id": "recipe-num",
        "recipe_key": "num",
        "creative_type": "num",
        "headline": "Pinjaman fleksibel",
        "benefit": "Limit hingga Rp80.000.000",
        "fragments": [{"id": "fragment-benefit", "key": "benefit", "role": "benefit", "text": "Limit hingga Rp80.000.000"}],
        "product_facts": [{"key": "limit", "value": "80000000", "copy_text": "Rp80.000.000"}],
        "status": "approved",
    }


def test_financial_tokens_normalize_ranges_and_keyword_amounts() -> None:
    assert financial_tokens("Limit 99.000.000, tenor 3-12 Bulan, bunga 0,03%") == {
        "financial_number:99000000",
        "financial_number:3",
        "financial_number:0",
        "term:3-12bulan",
        "percent:0.03",
    }

    assert financial_tokens("Rp80000000 / Rp.80.000.000 / IDR 80.000.000") == {
        "currency:80000000",
        "financial_number:80000000",
    }


def test_validate_snapshot_rejects_legacy_or_incomplete_approved_copy() -> None:
    with pytest.raises(ValueError, match="schema_version 2"):
        validate_snapshot({"headline": "legacy"}, "candidate-1")
    snapshot = approved_snapshot()
    snapshot["recipe_id"] = ""
    with pytest.raises(ValueError, match="incomplete recipe provenance"):
        validate_snapshot(snapshot, "candidate-1")


def test_main_blocks_unapproved_keyword_amount_before_generation(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    materials = tmp_path / "materials.json"
    materials.write_text(json.dumps({"items": [{"candidate_id": "candidate-1", "copy_snapshot": approved_snapshot()}]}), encoding="utf-8")
    monkeypatch.setattr(sys, "argv", [
        "validate_copy_snapshot.py",
        "--materials-json", str(materials),
        "--candidate-id", "candidate-1",
        "--prompt-text", "Gunakan limit 99.000.000",
    ])

    assert main() == 2


def test_metadata_cannot_expand_approved_financial_copy(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    snapshot = approved_snapshot()
    snapshot["metadata"] = {"notes": "Rp99.000.000"}
    materials = tmp_path / "materials.json"
    materials.write_text(json.dumps({"items": [{"candidate_id": "candidate-1", "copy_snapshot": snapshot}]}), encoding="utf-8")
    monkeypatch.setattr(sys, "argv", [
        "validate_copy_snapshot.py",
        "--materials-json", str(materials),
        "--candidate-id", "candidate-1",
        "--prompt-text", "Gunakan Rp99.000.000",
    ])

    assert main() == 2
