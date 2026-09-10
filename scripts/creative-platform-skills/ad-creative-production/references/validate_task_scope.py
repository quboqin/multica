#!/usr/bin/env python3
"""Validate the exact creative variant selected from an order response."""

import argparse
import json
import sys


def fail(message: str) -> None:
    raise ValueError(message)


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--order-file", required=True)
    parser.add_argument("--order-item-id", required=True)
    parser.add_argument("--variant-id", required=True)
    parser.add_argument("--revision", required=True, type=int)
    parser.add_argument("--expected-size", action="append", required=True)
    args = parser.parse_args()

    try:
        with open(args.order_file, encoding="utf-8") as source:
            order = json.load(source)
        items = order.get("items")
        if not isinstance(items, list):
            fail("order.items must be an array")
        item = next((entry for entry in items if entry.get("id") == args.order_item_id), None)
        if not isinstance(item, dict):
            fail("order item was not found by its exact ID")
        variants = item.get("variants")
        if not isinstance(variants, list):
            fail("order item variants must be an array")
        variant = next((entry for entry in variants if entry.get("id") == args.variant_id), None)
        if not isinstance(variant, dict):
            fail("variant was not found by its exact ID")
        if variant.get("revision") != args.revision:
            fail("variant revision does not match task revision")
        candidate_state = variant.get("candidate_state")
        if candidate_state == "candidate":
            if len(args.expected_size) != 1:
                fail("candidate primary task must contain exactly one expected size")
            if variant.get("primary_size") != args.expected_size[0]:
                fail("exact candidate primary_size does not match task expected size")
    except (OSError, json.JSONDecodeError, ValueError) as err:
        print(f"invalid creative task scope: {err}", file=sys.stderr)
        return 1

    print(json.dumps({
        "valid": True,
        "candidate_state": candidate_state,
        "primary_size": variant.get("primary_size", ""),
        "expected_sizes": args.expected_size,
    }))
    return 0


if __name__ == "__main__":
    sys.exit(main())
