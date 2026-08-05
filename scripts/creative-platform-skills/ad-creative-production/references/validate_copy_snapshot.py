#!/usr/bin/env python3
"""Reject financial tokens that are absent from the selected copy snapshot."""

from __future__ import annotations

import argparse
import json
import re
import sys
from pathlib import Path
from typing import Any


CREATIVE_TYPES = {"num", "repayment_plan"}
CURRENCY_PATTERN = re.compile(r"\b(?:Rp\.?|IDR)\s*(\d+(?:[.,]\d+)*)", re.IGNORECASE)
PERCENT_PATTERN = re.compile(r"\b(\d+(?:[.,]\d+)?)\s*%")
TERM_PATTERN = re.compile(r"\b(\d+(?:\s*-\s*\d+)?)\s*(bulan|hari|tahun)\b", re.IGNORECASE)
FINANCIAL_NUMBER_PATTERN = re.compile(
    r"\b(?:limit|pinjaman|dana|jumlah|cicilan|angsuran|tenor|bunga|interest|biaya|fee)"
    r"\D{0,24}(\d{1,3}(?:[.,]\d{3})+|\d+)(?:\s*(?:juta|ribu|miliar))?",
    re.IGNORECASE,
)


def digits(value: str) -> str:
    return re.sub(r"\D", "", value)


def financial_tokens(value: str) -> set[str]:
    tokens: set[str] = set()
    for match in CURRENCY_PATTERN.finditer(value):
        normalized = digits(match.group(1))
        tokens.update((f"currency:{normalized}", f"financial_number:{normalized}"))
    for match in PERCENT_PATTERN.finditer(value):
        tokens.add(f"percent:{match.group(1).replace(',', '.')}")
    for match in TERM_PATTERN.finditer(value):
        term = re.sub(r"\s+", "", match.group(1))
        tokens.add(f"term:{term}{match.group(2).lower()}")
    for match in FINANCIAL_NUMBER_PATTERN.finditer(value):
        tokens.add(f"financial_number:{digits(match.group(1))}")
    return tokens


def read_json(path: str) -> dict[str, Any]:
    if path == "-":
        return json.load(sys.stdin)
    with Path(path).open("r", encoding="utf-8-sig") as handle:
        return json.load(handle)


def find_item(payload: dict[str, Any], candidate_id: str) -> dict[str, Any]:
    for item in payload.get("items", []):
        if str(item.get("candidate_id")) == candidate_id:
            return item
    raise ValueError(f"candidate {candidate_id} has no selected copy snapshot")


def approved_text(snapshot: dict[str, Any]) -> str:
    values = [
        snapshot.get("headline"),
        snapshot.get("subheadline"),
        snapshot.get("benefit"),
        snapshot.get("supporting"),
        snapshot.get("cta"),
        snapshot.get("legal_text"),
    ]
    return "\n".join(str(value) for value in values if value)


def validate_snapshot(snapshot: dict[str, Any], candidate_id: str) -> None:
    if snapshot.get("schema_version") != 2:
        raise ValueError(f"candidate {candidate_id} copy snapshot must use schema_version 2")
    if snapshot.get("creative_type") not in CREATIVE_TYPES:
        raise ValueError(f"candidate {candidate_id} copy snapshot has invalid creative_type")
    status = snapshot.get("status")
    if status not in {"approved", "user_custom"}:
        raise ValueError(f"candidate {candidate_id} copy snapshot has invalid status")
    if status == "approved":
        required = ("library_id", "library_version", "recipe_id", "recipe_key")
        if any(not snapshot.get(field) for field in required):
            raise ValueError(f"candidate {candidate_id} approved copy snapshot has incomplete recipe provenance")
        if not isinstance(snapshot.get("fragments"), list) or not isinstance(snapshot.get("product_facts"), list):
            raise ValueError(f"candidate {candidate_id} approved copy snapshot has invalid evidence")


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--materials-json", required=True)
    parser.add_argument("--candidate-id", required=True)
    parser.add_argument("--prompt-file", action="append", default=[])
    parser.add_argument("--prompt-text", action="append", default=[])
    parser.add_argument("--evidence")
    args = parser.parse_args()

    payload = read_json(args.materials_json)
    item = find_item(payload, args.candidate_id)
    snapshot = item.get("copy_snapshot") or {}
    validate_snapshot(snapshot, args.candidate_id)
    approved = financial_tokens(approved_text(snapshot))

    prompts: list[tuple[str, str]] = []
    for raw_path in args.prompt_file:
        path = Path(raw_path)
        prompts.append((str(path), path.read_text(encoding="utf-8-sig")))
    prompts.extend((f"inline:{index + 1}", value) for index, value in enumerate(args.prompt_text))
    if not prompts:
        raise ValueError("at least one --prompt-file or --prompt-text is required")

    checks = []
    passed = True
    for source, prompt in prompts:
        observed = financial_tokens(prompt)
        unapproved = sorted(observed - approved)
        passed = passed and not unapproved
        checks.append(
            {
                "source": source,
                "observed_financial_tokens": sorted(observed),
                "unapproved_financial_tokens": unapproved,
                "passed": not unapproved,
            }
        )

    evidence = {
        "candidate_id": args.candidate_id,
        "copy_snapshot_id": snapshot.get("id") or item.get("copy_entry_id"),
        "copy_snapshot_version": snapshot.get("library_version") or snapshot.get("version"),
        "approved_financial_tokens": sorted(approved),
        "prompt_checks": checks,
        "passed": passed,
    }
    encoded = json.dumps(evidence, ensure_ascii=False, indent=2)
    if args.evidence:
        Path(args.evidence).write_text(encoded + "\n", encoding="utf-8")
    print(encoded)
    return 0 if passed else 2


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except (OSError, ValueError, json.JSONDecodeError) as exc:
        print(json.dumps({"passed": False, "error": str(exc)}, ensure_ascii=False), file=sys.stderr)
        raise SystemExit(2)
