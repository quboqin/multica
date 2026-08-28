#!/usr/bin/env python3
"""Validate a first image-operation-put payload before it reaches the API."""

import argparse
import json
import sys


REQUIRED_FIELDS = {
    "variant_id",
    "size_key",
    "revision",
    "operation_kind",
    "idempotency_key",
    "status",
    "model",
    "input_snapshot",
    "attempt",
}


def fail(message: str) -> None:
    raise ValueError(message)


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--input-file", required=True)
    args = parser.parse_args()

    try:
        with open(args.input_file, encoding="utf-8") as source:
            payload = json.load(source)
        if not isinstance(payload, dict):
            fail("operation payload must be a JSON object")
        missing = sorted(REQUIRED_FIELDS - payload.keys())
        if missing:
            fail("missing required top-level fields: " + ", ".join(missing))
        misplaced = sorted(
            key
            for key in ("input_asset_fingerprints", "input_fingerprints", "input_roles", "target_size", "kind")
            if key in payload
        )
        if misplaced:
            fail("fields belong under input_snapshot or use operation_kind: " + ", ".join(misplaced))
        if payload["status"] not in {"queued", "running"}:
            fail("first operation status must be queued or running")
        if payload["attempt"] != 1:
            fail("first operation attempt must be 1")
        snapshot = payload["input_snapshot"]
        if not isinstance(snapshot, dict):
            fail("input_snapshot must be a JSON object")
        if not isinstance(snapshot.get("input_asset_fingerprints"), dict):
            fail("input_snapshot.input_asset_fingerprints must be a JSON object")
        if not isinstance(snapshot.get("input_roles"), list) or not snapshot["input_roles"]:
            fail("input_snapshot.input_roles must be a non-empty array")
        if snapshot.get("target_size") != payload["size_key"]:
            fail("input_snapshot.target_size must equal size_key")
    except (OSError, json.JSONDecodeError, ValueError) as err:
        print(f"invalid image operation: {err}", file=sys.stderr)
        return 1

    print(json.dumps({"valid": True, "size_key": payload["size_key"]}))
    return 0


if __name__ == "__main__":
    sys.exit(main())
