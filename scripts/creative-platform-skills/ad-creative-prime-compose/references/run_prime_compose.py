#!/usr/bin/env python3
"""Invoke the backend Prime composition CLI and validate its JSON result."""

from __future__ import annotations

import argparse
import json
import os
import subprocess
import sys
from collections.abc import Mapping, Sequence
from typing import Any


COMPLETED_STATUSES = frozenset({"completed", "complete", "success", "succeeded"})


def build_command(order_id: str, variant_id: str, output: str) -> list[str]:
    return [
        "multica",
        "creative",
        "order",
        "prime-compose",
        order_id,
        "--variant",
        variant_id,
        "--output",
        output,
    ]


def has_completion_signal(value: Any) -> bool:
    if not isinstance(value, Mapping):
        return False

    if value.get("success") is True or value.get("completed") is True:
        return True

    status = value.get("status")
    if isinstance(status, str) and status.strip().lower() in COMPLETED_STATUSES:
        return True

    result = value.get("result")
    return has_completion_signal(result)


def decode_completed_result(stdout: str) -> Mapping[str, Any]:
    try:
        payload = json.loads(stdout)
    except json.JSONDecodeError as exc:
        raise ValueError("prime-compose returned invalid JSON") from exc

    if not isinstance(payload, Mapping):
        raise ValueError("prime-compose JSON result must be an object")
    if not has_completion_signal(payload):
        raise ValueError("prime-compose JSON result is not successful or completed")
    return payload


def run_prime_compose(order_id: str, variant_id: str, output: str) -> Mapping[str, Any]:
    env = os.environ.copy()
    env.setdefault("MULTICA_HTTP_TIMEOUT", "5m")
    completed = subprocess.run(
        build_command(order_id, variant_id, output),
        check=False,
        capture_output=True,
        text=True,
        encoding="utf-8",
        errors="replace",
        env=env,
    )
    if completed.returncode != 0:
        detail = completed.stderr.strip() or "no stderr output"
        raise RuntimeError(
            f"prime-compose command failed with exit code {completed.returncode}: {detail}"
        )
    return decode_completed_result(completed.stdout)


def parse_args(argv: Sequence[str] | None = None) -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Invoke backend-owned Prime composition for one creative variant."
    )
    parser.add_argument("--order-id", required=True)
    parser.add_argument("--variant", required=True, dest="variant_id")
    parser.add_argument("--output", choices=("json",), default="json")
    return parser.parse_args(argv)


def main(argv: Sequence[str] | None = None) -> int:
    args = parse_args(argv)
    try:
        result = run_prime_compose(args.order_id, args.variant_id, args.output)
    except (OSError, RuntimeError, ValueError) as exc:
        print(str(exc), file=sys.stderr)
        return 1

    print(json.dumps(result, ensure_ascii=False, sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
