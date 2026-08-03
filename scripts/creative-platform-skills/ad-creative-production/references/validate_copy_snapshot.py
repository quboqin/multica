#!/usr/bin/env python3
"""Reject financial tokens that are absent from the selected copy snapshot."""

from __future__ import annotations

import argparse
import json
import re
import sys
from pathlib import Path
from typing import Any


TOKEN_PATTERNS = (
    re.compile(r"\bRp\s*\d{1,3}(?:\.\d{3})*", re.IGNORECASE),
    re.compile(r"\b\d+(?:[.,]\d+)?\s*%"),
    re.compile(r"\b\d+(?:[.,]\d+)?\s*(?:JUTA|BULAN|HARI|TRILIUN)\b", re.IGNORECASE),
)


def normalize_token(value: str) -> str:
    return re.sub(r"\s+", "", value).upper()


def financial_tokens(value: str) -> set[str]:
    tokens: set[str] = set()
    for pattern in TOKEN_PATTERNS:
        tokens.update(normalize_token(match.group(0)) for match in pattern.finditer(value))
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
        snapshot.get("cta"),
        snapshot.get("legal_text"),
    ]
    metadata = snapshot.get("metadata") or {}
    values.extend((metadata.get("original_copy"), metadata.get("notes")))
    return "\n".join(str(value) for value in values if value)


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
        "copy_snapshot_version": snapshot.get("version"),
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
