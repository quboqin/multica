#!/usr/bin/env python3
"""Prepare an image operation from current order state without guessing retries."""

import argparse
import copy
import json
import os
import tempfile


def prepare_operation(order, draft):
    order = order.get("order", order)
    variants = [v for item in order.get("items", []) for v in item.get("variants", [])]
    variant = next((v for v in variants if v.get("id") == draft.get("variant_id")), None)
    if variant is None or variant.get("revision") != draft.get("revision"):
        raise ValueError("variant is missing or its current revision changed; read the order again")
    if order.get("status") == "cancelled" or variant.get("candidate_state") in {"rejected", "reserve"} or variant.get("status") == "cancelled":
        raise ValueError("variant is not eligible for production")
    matches = [op for op in variant.get("image_operations", [])
               if op.get("revision") == draft.get("revision")
               and op.get("size_key") == draft.get("size_key")
               and op.get("operation_kind") == draft.get("operation_kind")]
    if len(matches) > 1:
        raise ValueError("multiple operations match these coordinates; reconcile before invoking")
    if not matches:
        result = copy.deepcopy(draft)
        result["attempt"] = 1
        result["status"] = "running"
        for key in ("prompt_sha256", "provider_request_id", "result_receipt", "output_attachment_id"):
            result.pop(key, None)
        return result
    op = matches[0]
    if op.get("status") not in {"failed", "queued"}:
        raise ValueError("existing operation is %s; reuse its completed receipt or reconcile it, never invoke again" % op.get("status"))
    attempts = [a.get("attempt") for a in op.get("attempts", [])]
    if not attempts or any(isinstance(a, bool) or not isinstance(a, int) or a < 1 for a in attempts):
        raise ValueError("operation attempt history is missing or invalid; read the order again")
    result = {key: copy.deepcopy(op[key]) for key in (
        "size_key", "revision", "operation_kind", "idempotency_key", "model", "input_snapshot")}
    result["variant_id"] = variant["id"]
    result["attempt"] = max(attempts) + (1 if op["status"] == "failed" else 0)
    result["status"] = "running"
    if op.get("prompt_sha256"):
        result["prompt_sha256"] = op["prompt_sha256"]
    return result


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--order-json", required=True)
    parser.add_argument("--input-file", required=True)
    parser.add_argument("--output-file", required=True)
    args = parser.parse_args()
    try:
        with open(args.order_json, encoding="utf-8") as source:
            order = json.load(source)
        with open(args.input_file, encoding="utf-8") as source:
            draft = json.load(source)
        result = prepare_operation(order, draft)
        directory = os.path.dirname(os.path.abspath(args.output_file))
        with tempfile.NamedTemporaryFile(mode="w", encoding="utf-8", dir=directory, delete=False) as out:
            json.dump(result, out, ensure_ascii=False, indent=2)
            temporary = out.name
        os.replace(temporary, args.output_file)
        print(json.dumps({"prepared": True, "attempt": result["attempt"], "size_key": result["size_key"]}))
    except (OSError, ValueError, KeyError, TypeError) as err:
        parser.exit(1, "cannot prepare image operation: %s\n" % err)


if __name__ == "__main__":
    main()
