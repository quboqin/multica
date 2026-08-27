import json
import sys
from pathlib import Path

import pytest

from validate_copy_snapshot import (
    MAX_PROMPT_CHARS,
    approved_text,
    financial_tokens,
    find_item,
    main,
    validate_concise_prompt,
    validate_prime_prompt_guard,
    validate_snapshot,
)


def approved_snapshot() -> dict:
    return {
        "schema_version": 3,
        "id": "recipe-num",
        "library_id": "library-1",
        "library_version": 3,
        "recipe_id": "recipe-num",
        "recipe_key": "num",
        "creative_type": "num",
        "headline": "Pinjaman fleksibel",
        "benefit": "Limit hingga Rp80.000.000",
        "fragments": [{"id": "fragment-benefit", "key": "benefit", "role": "benefit", "text": "Limit hingga Rp80.000.000"}],
        "repayment_plan_entries": [],
        "status": "approved",
    }


def test_financial_tokens_normalize_ranges_and_keyword_amounts() -> None:
    assert financial_tokens("Limit 99.000.000, tenor 3-12 Bulan, bunga 0,03%") == {
        "financial_number:99000000",
        "financial_number:3",
        "term:3-12bulan",
        "percent:0.03",
    }

    assert financial_tokens("Rp80000000 / Rp.80.000.000 / IDR 80.000.000") == {
        "currency:80000000",
        "financial_number:80000000",
    }


def test_validate_snapshot_rejects_legacy_or_incomplete_approved_copy() -> None:
    with pytest.raises(ValueError, match="schema_version 3"):
        validate_snapshot({"headline": "legacy"}, "candidate-1")
    snapshot = approved_snapshot()
    snapshot["recipe_id"] = ""
    with pytest.raises(ValueError, match="incomplete composition provenance"):
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


def test_frozen_numeric_layout_authorizes_its_first_party_calculated_values(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    snapshot = approved_snapshot()
    snapshot["pre_adaptation"] = {
        "numeric_layouts": [{
            "id": "table-1",
            "render_instruction": "Jumlah Pinjaman Rp30.000.000, Periode Cicilan 3 Bulan, Cicilan per Bulan Rp10.270.000.",
        }],
        "production_prompt": "Use the two-column table with first-party values only.",
    }
    assert "Rp30.000.000" in approved_text(snapshot)
    materials = tmp_path / "materials.json"
    materials.write_text(json.dumps({"items": [{"candidate_id": "candidate-1", "copy_snapshot": snapshot}]}), encoding="utf-8")
    monkeypatch.setattr(sys, "argv", [
        "validate_copy_snapshot.py",
        "--materials-json", str(materials),
        "--candidate-id", "candidate-1",
        "--prompt-text", "Jumlah Pinjaman Rp30.000.000, Periode Cicilan 3 Bulan, Cicilan per Bulan Rp10.270.000.",
    ])

    assert main() == 0


def test_repayment_plan_and_calculation_values_authorize_financial_tokens(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    snapshot = approved_snapshot()
    snapshot["repayment_plan_entries"] = [{
        "key": "p5000-t12",
        "principal": 5_000_000,
        "tenor_months": 12,
        "monthly_installment": 461_667,
        "total_interest": 540_004,
        "total_repayment": 5_540_004,
        "source": "approved calculator",
    }]
    snapshot["pre_adaptation"] = {
        "text_replacements": [{
            "block_id": "daily-interest",
            "location": "rate card",
            "replacement_text": "Bunga per hari",
            "calculation": {
                "formula": "principal × daily_rate",
                "inputs": ["principal Rp5.000.000", "daily_rate 0,03%"],
                "result": "Rp1.500",
            },
        }],
        "repayment_plan_selections": [{
            "id": "scenario-12",
            "principal": 5_000_000,
            "tenor_months": 12,
            "values": {
                "principal": "Rp5.000.000",
                "tenor": "12 Bulan",
                "monthly_installment": "Rp461.667",
                "total_interest": "Rp540.004",
                "total_repayment": "Rp5.540.004",
            },
        }],
    }
    materials = tmp_path / "materials.json"
    materials.write_text(json.dumps({"items": [{"candidate_id": "candidate-1", "copy_snapshot": snapshot}]}), encoding="utf-8")
    monkeypatch.setattr(sys, "argv", [
        "validate_copy_snapshot.py",
        "--materials-json", str(materials),
        "--candidate-id", "candidate-1",
        "--prompt-text", "Render Rp5.000.000 / 12 Bulan with Rp461.667 monthly, Rp540.004 interest, Rp5.540.004 total, Rp1.500 daily interest, and 0,03%.",
    ])

    assert main() == 0


def test_editing_the_production_prompt_cannot_self_approve_a_new_financial_value(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    snapshot = approved_snapshot()
    snapshot["pre_adaptation"] = {
        "numeric_layouts": [{"id": "table-1", "render_instruction": "Jumlah Pinjaman Rp30.000.000"}],
        "production_prompt": "Do not add Rp99.000.000.",
    }
    materials = tmp_path / "materials.json"
    materials.write_text(json.dumps({"items": [{"candidate_id": "candidate-1", "copy_snapshot": snapshot}]}), encoding="utf-8")
    monkeypatch.setattr(sys, "argv", [
        "validate_copy_snapshot.py",
        "--materials-json", str(materials),
        "--candidate-id", "candidate-1",
        "--prompt-text", "Do not add Rp99.000.000.",
    ])

    assert main() == 2


def test_forbidden_direction_financial_token_is_not_treated_as_visible_copy(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    materials = tmp_path / "materials.json"
    materials.write_text(json.dumps({"items": [{"candidate_id": "candidate-1", "copy_snapshot": approved_snapshot()}]}), encoding="utf-8")
    prompt = """IMMUTABLE USER DIRECTION:
Do not retain the source 9 Bulan term.

APPROVED TEXT:
Pinjaman fleksibel
Limit hingga Rp80.000.000

TABLE:
No table.

FORBIDDEN:
No unapproved readable number.
"""
    monkeypatch.setattr(sys, "argv", [
        "validate_copy_snapshot.py",
        "--materials-json", str(materials),
        "--candidate-id", "candidate-1",
        "--prompt-text", prompt,
    ])

    assert main() == 0


def test_forbidden_source_text_section_is_not_treated_as_visible_copy(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    materials = tmp_path / "materials.json"
    materials.write_text(json.dumps({"items": [{"candidate_id": "candidate-1", "copy_snapshot": approved_snapshot()}]}), encoding="utf-8")
    prompt = """APPROVED TEXT:
Pinjaman fleksibel
Limit hingga Rp80.000.000

FORBIDDEN SOURCE TEXT:
Rp1.000.000 sampai Rp25.000.000, 14%, 91-120 hari

TABLE:
No table.

FORBIDDEN:
No unapproved readable number.
"""
    monkeypatch.setattr(sys, "argv", [
        "validate_copy_snapshot.py",
        "--materials-json", str(materials),
        "--candidate-id", "candidate-1",
        "--prompt-text", prompt,
    ])

    assert main() == 0


def test_visible_prompt_financial_token_remains_blocked(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    materials = tmp_path / "materials.json"
    materials.write_text(json.dumps({"items": [{"candidate_id": "candidate-1", "copy_snapshot": approved_snapshot()}]}), encoding="utf-8")
    prompt = """IMMUTABLE USER DIRECTION:
Do not retain the source 9 Bulan term.

APPROVED TEXT:
Pinjaman fleksibel 9 Bulan

TABLE:
No table.
"""
    monkeypatch.setattr(sys, "argv", [
        "validate_copy_snapshot.py",
        "--materials-json", str(materials),
        "--candidate-id", "candidate-1",
        "--prompt-text", prompt,
    ])

    assert main() == 2


def test_find_item_uses_order_item_or_variant_before_candidate_fallback() -> None:
    first_snapshot = approved_snapshot()
    second_snapshot = approved_snapshot()
    second_snapshot["headline"] = "Pinjaman kedua"
    payload = {
        "items": [
            {"id": "item-1", "candidate_id": "candidate-1", "variants": [{"id": "variant-1"}], "copy_snapshot": first_snapshot},
            {"id": "item-2", "candidate_id": "candidate-1", "variants": [{"id": "variant-2"}], "copy_snapshot": second_snapshot},
        ],
    }

    assert find_item(payload, "candidate-1", order_item_id="item-2")["id"] == "item-2"
    assert find_item(payload, "candidate-1", variant_id="variant-2")["id"] == "item-2"


def test_main_validates_the_context_order_item_snapshot(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    first_snapshot = approved_snapshot()
    second_snapshot = approved_snapshot()
    first_snapshot["benefit"] = "Limit hingga Rp10.000.000"
    second_snapshot["benefit"] = "Limit hingga Rp20.000.000"
    materials = tmp_path / "materials.json"
    materials.write_text(json.dumps({
        "items": [
            {"id": "item-1", "candidate_id": "candidate-1", "copy_snapshot": first_snapshot},
            {"id": "item-2", "candidate_id": "candidate-1", "copy_snapshot": second_snapshot},
        ],
    }), encoding="utf-8")
    monkeypatch.setattr(sys, "argv", [
        "validate_copy_snapshot.py",
        "--materials-json", str(materials),
        "--candidate-id", "candidate-1",
        "--order-item-id", "item-2",
        "--prompt-text", "Gunakan limit Rp20.000.000",
    ])

    assert main() == 0


def prime_layout() -> dict:
    return {
        "hard_regions": [
            {"id": "logo", "x1": 30, "y1": 28, "x2": 314, "y2": 100},
            {"id": "terms", "x1": 777, "y1": 32, "x2": 982, "y2": 95},
            {"id": "qr", "x1": 983, "y1": 29, "x2": 1053, "y2": 99},
            {"id": "regulatory", "x1": 378, "y1": 1010, "x2": 913, "y2": 1053},
        ],
        "safe_content_frame": [50, 100, 1030, 934],
        "top_key_content_exclusion_end": 100,
        "bottom_key_content_exclusion_start": 984,
    }


def structured_prime_prompt(
    *,
    include_safe_frame: bool = True,
    include_business_canvas_grammar: bool = True,
    include_bottom_clearance: bool = True,
    include_redesign: bool = True,
    whole_image_inset: bool = False,
) -> str:
    safe_frame = (
        "safe_content_frame=(50,100)-(1030,934). CONTENT_RECT=(50,100)-(1030,934). The safe content frame controls only readable content, key business content, text and CTA. "
        if include_safe_frame else ""
    )
    business_canvas_grammar = "COORDINATE GRAMMAR: BUSINESS_CANVAS=(50,100)-(1030,934). Top, upper, headline zone, header, bottom, lower, and footer mean BUSINESS_CANVAS, never the physical canvas. Prime coordinates are no-draw constraints, never visible bands or panels. " if include_business_canvas_grammar else ""
    bottom_clearance = (
        "Active business groups remain above the bottom template boundary y=984 with fully visible clearance. "
        if include_bottom_clearance else ""
    )
    redesign = "Reference is structure only; remove all source identity and change high-salience visual identity through redesign." if include_redesign else ""
    inset = "Scale down the whole image into the safe content frame." if whole_image_inset else ""
    return f"""
TASK:
Create an unbranded base image only. Prime overlay assets will be composited later.
INPUTS:
Input 1 is reference structure only. Input 2 is a neutral monochrome geometry map for avoidance only; do not draw input 2, do not copy its neutral gray marks, and do not render guide blocks.
PRIME RESERVED AREAS:
Do not render any logo, QR, legal footer, regulatory text, app store badge, OJK, AFPI, or Pindai mark.
Treat every hard region as natural low-detail background only: logo [30,28,314,100]; terms [777,32,982,95]; qr [983,29,1053,99]; regulatory [378,1010,913,1053].
Bottom exclusion boundary y>=984 must stay as continuous low texture background.
CONTENT LAYOUT:
{business_canvas_grammar}{safe_frame}Active business groups stay inside BUSINESS_CANVAS. {bottom_clearance}Keep background, color fields, shadows and decorative surfaces full-bleed edge-to-edge across the full canvas with no inset and no border. {inset}
APPROVED TEXT:
Pinjaman fleksibel. Limit hingga Rp80.000.000.
TABLE:
No table.
STYLE:
New Indonesian financial ad base. {redesign}
FORBIDDEN:
No extra claims, no extra numbers, no source brand, no official Prime template content.
FINAL:
Clean base only, all business content readable and outside Prime reserved areas.
"""


def test_prime_guard_blocks_prompt_that_would_generate_double_prime_assets() -> None:
    prompt = "Create a green, white, and gold AdaKami repayment-cost infographic with a footer and QR."

    missing = validate_prime_prompt_guard(prompt, [prime_layout()])

    assert "unbranded_base" in missing
    assert "region_id:logo" in missing
    assert "region_rect:30,28,314,100" in missing


def test_prime_guard_accepts_unbranded_prompt_with_exact_hard_region_contract() -> None:
    prompt = structured_prime_prompt()

    assert validate_prime_prompt_guard(prompt, [{"__canvas_width": 1080, "__canvas_height": 1080, **prime_layout()}]) == []


def test_prime_guard_requires_bottom_business_avoidance_when_bottom_boundary_exists() -> None:
    prompt = """
    Create an unbranded base image only. Prime overlay assets will be composited later.
    Input 2 is a reserved-area guide for avoidance only; do not draw input 2.
    Do not render any logo, QR, legal footer, regulatory text, app store badge, OJK, AFPI, or Pindai mark.
    Treat every hard region as natural low-detail background only:
    logo [30,28,314,100]; terms [777,32,982,95]; qr [983,29,1053,99]; regulatory [378,1010,913,1053].
    Top exclusion boundary y<=100 and bottom exclusion boundary y>=984 must stay as continuous low texture background.
    """

    assert "bottom_business_avoidance" in validate_prime_prompt_guard(prompt, [prime_layout()])


def test_prime_guard_requires_safe_content_frame_from_layout_contract() -> None:
    prompt = structured_prime_prompt(include_safe_frame=False, include_business_canvas_grammar=False, include_bottom_clearance=False)

    missing = validate_prime_prompt_guard(prompt, [{"__canvas_width": 1080, "__canvas_height": 1080, **prime_layout()}])

    assert "safe_content_frame_label" in missing
    assert "safe_content_frame_rect:50,100,1030,934" in missing
    assert "business_canvas_grammar:100:934" in missing
    assert "bottom_group_prime_clearance:934:984" in missing


def test_prime_guard_requires_lower_group_clearance_from_bottom_prime_band() -> None:
    prompt = structured_prime_prompt(include_bottom_clearance=False)

    missing = validate_prime_prompt_guard(prompt, [{"__canvas_width": 1080, "__canvas_height": 1080, **prime_layout()}])

    assert "bottom_group_prime_clearance:934:984" in missing


def test_prime_guard_rejects_whole_image_inset_into_safe_frame() -> None:
    prompt = structured_prime_prompt(whole_image_inset=True)

    missing = validate_prime_prompt_guard(prompt, [{"__canvas_width": 1080, "__canvas_height": 1080, **prime_layout()}])

    assert "whole_image_inset_forbidden" in missing


def test_main_requires_redesign_guard_when_requested(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    materials = tmp_path / "materials.json"
    layout = tmp_path / "layout-1080x1080.json"
    materials.write_text(json.dumps({"items": [{"candidate_id": "candidate-1", "copy_snapshot": approved_snapshot()}]}), encoding="utf-8")
    layout.write_text(json.dumps(prime_layout()), encoding="utf-8")
    prompt = structured_prime_prompt()
    monkeypatch.setattr(sys, "argv", [
        "validate_copy_snapshot.py",
        "--materials-json", str(materials),
        "--candidate-id", "candidate-1",
        "--prompt-text", prompt,
        "--prime-layout-file", str(layout),
        "--require-prime-guard",
        "--require-redesign-guard",
    ])

    assert main() == 0


def test_main_requires_prime_guard_when_layout_file_is_supplied(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    materials = tmp_path / "materials.json"
    layout = tmp_path / "layout-1080x1080.json"
    materials.write_text(json.dumps({"items": [{"candidate_id": "candidate-1", "copy_snapshot": approved_snapshot()}]}), encoding="utf-8")
    layout.write_text(json.dumps(prime_layout()), encoding="utf-8")
    monkeypatch.setattr(sys, "argv", [
        "validate_copy_snapshot.py",
        "--materials-json", str(materials),
        "--candidate-id", "candidate-1",
        "--prompt-text", "Create a branded ad with a QR footer.",
        "--prime-layout-file", str(layout),
        "--require-prime-guard",
    ])

    assert main() == 2


def test_concise_prompt_blocks_repeated_long_instruction() -> None:
    repeated = "Keep the repayment table compact with one principal column and three tenor columns"
    prompt = f"{repeated}. Render a new scene. {repeated}."

    debt = validate_concise_prompt(prompt)

    assert any(item.startswith("duplicate_segment:") for item in debt)


def test_concise_prompt_enforces_current_character_hard_limit() -> None:
    assert not any(item.startswith("prompt_too_long:") for item in validate_concise_prompt("x" * MAX_PROMPT_CHARS))
    assert f"prompt_too_long:{MAX_PROMPT_CHARS + 1}>{MAX_PROMPT_CHARS}" in validate_concise_prompt("x" * (MAX_PROMPT_CHARS + 1))


def test_main_explain_outputs_actionable_repair_guidance(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
    capsys: pytest.CaptureFixture[str],
) -> None:
    materials = tmp_path / "materials.json"
    materials.write_text(json.dumps({"items": [{"candidate_id": "candidate-1", "copy_snapshot": approved_snapshot()}]}), encoding="utf-8")
    monkeypatch.setattr(sys, "argv", [
        "validate_copy_snapshot.py",
        "--materials-json", str(materials),
        "--candidate-id", "candidate-1",
        "--prompt-text", "APPROVED TEXT:\nGunakan limit Rp99.000.000\nTABLE:\nNo table.\n",
        "--explain",
    ])

    assert main() == 2
    payload = json.loads(capsys.readouterr().out)

    assert payload["prompt_policy"] == {
        "target_chars": "900-1800",
        "complex_soft_max_chars": 2800,
        "hard_max_chars": 3600,
    }
    guidance = payload["repair_guidance"]
    assert any(
        item["rule"] == "unapproved_financial_token"
        and item["token"] == "currency:99000000"
        and item["line"] == 2
        and "financial_number:80000000" in item["approved_financial_tokens"]
        for item in guidance
    )
    assert any("unapproved_financial_token" in item for item in payload["repair_summary"])


def test_main_requires_concise_prompt_when_requested(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    materials = tmp_path / "materials.json"
    materials.write_text(json.dumps({"items": [{"candidate_id": "candidate-1", "copy_snapshot": approved_snapshot()}]}), encoding="utf-8")
    repeated = "Keep the approved business visual hierarchy while redesigning the scene as a new unbranded base"
    monkeypatch.setattr(sys, "argv", [
        "validate_copy_snapshot.py",
        "--materials-json", str(materials),
        "--candidate-id", "candidate-1",
        "--prompt-text", f"{repeated}. {repeated}.",
        "--require-concise-prompt",
    ])

    assert main() == 2


def compiled_visual_prompt(extra: str = "") -> str:
    return f"""TASK
Create one unbranded 1080x1080 locked 1:1 square ad base and redesign the source identity.

INPUT ROLES
Input 1 is used only for business structure and reading order. Input 2 is the current-size official Prime visual context.

LOCKED DESIGN DNA
Keep the same DesignDNA subject system, palette roles, material, lighting, motif, and information hierarchy.

EDITABLE LAYOUT
Use a native square layout with every business content group clear of the future component areas.

APPROVED COPY
Render every approved string exactly once: Pinjaman fleksibel. Limit hingga Rp80.000.000. No table.

PRIME SUPPORT
Input 2 is the current-size official Prime visual context. Keep its future component areas clear of business content and use a calm
low-detail background with sufficient contrast. Official Prime is visual context only: never draw or copy its logo, QR, legal text,
regulatory mark, template wording, or component geometry.

ACCEPTANCE
All approved copy is legible. No competitor or source identity, extra claim, duplicate copy, or Prime element is present. {extra}
"""


def test_compiled_visual_prompt_contract_passes_semantic_guards() -> None:
    prompt = compiled_visual_prompt()

    assert validate_prime_prompt_guard(prompt, [prime_layout()]) == []
    assert validate_concise_prompt(prompt) == []


def test_compiled_visual_prompt_rejects_transaction_protocol() -> None:
    debt = validate_concise_prompt(compiled_visual_prompt("Retry asset-put with request_id from JSON after upload timeout."))

    assert "non_visual_protocol_term:request_or_hash" in debt
    assert "non_visual_protocol_term:workflow_command" in debt
    assert "non_visual_protocol_term:workflow_control" in debt
