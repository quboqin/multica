#!/usr/bin/env python3
"""Run one image-edit CLI command outside a short-lived agent tool call.

The Codex command bridge can return an empty result after roughly thirty
seconds even while a provider request is still alive.  This helper detaches a
single worker, persists its state atomically, and lets the agent poll without
starting a second provider request.
"""

from __future__ import annotations

import argparse
import json
import os
from pathlib import Path
import subprocess
import sys
import time
from typing import Any


def utc_now() -> str:
    return time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())


def read_state(path: Path) -> dict[str, Any]:
    try:
        value = json.loads(path.read_text(encoding="utf-8"))
    except FileNotFoundError:
        return {}
    except json.JSONDecodeError as exc:
        raise RuntimeError(f"invalid job state {path}: {exc}") from exc
    if not isinstance(value, dict):
        raise RuntimeError(f"invalid job state {path}: expected JSON object")
    return value


def write_state(path: Path, value: dict[str, Any]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    temporary = path.with_name(f".{path.name}.{os.getpid()}.tmp")
    temporary.write_text(json.dumps(value, ensure_ascii=True, sort_keys=True) + "\n", encoding="utf-8")
    os.replace(temporary, path)


def command_from_args(command: list[str]) -> list[str]:
    if command and command[0] == "--":
        command = command[1:]
    if len(command) < 3 or command[:3] != ["multica", "image", "edit"]:
        raise RuntimeError("job command must start with: multica image edit")
    required = {"--result-file", "--output-file", "--operation-id", "--operation-attempt"}
    missing = sorted(required.difference(command))
    if missing:
        raise RuntimeError(f"image edit command is missing required arguments: {', '.join(missing)}")
    return command


def worker(args: argparse.Namespace) -> int:
    state_path = Path(args.state)
    stdout_path = Path(args.stdout_file)
    stderr_path = Path(args.stderr_file)
    command = command_from_args(args.command)
    started = time.monotonic()
    state = read_state(state_path)
    state.update(
        {
            "schema_version": 1,
            "status": "running",
            "worker_pid": os.getpid(),
            "started_at": state.get("started_at") or utc_now(),
            "command": command,
            "stdout_file": str(stdout_path),
            "stderr_file": str(stderr_path),
            "timeout_seconds": args.timeout_seconds,
        }
    )
    write_state(state_path, state)

    stdout_path.parent.mkdir(parents=True, exist_ok=True)
    stderr_path.parent.mkdir(parents=True, exist_ok=True)
    with stdout_path.open("wb") as stdout, stderr_path.open("wb") as stderr:
        try:
            process = subprocess.Popen(command, stdout=stdout, stderr=stderr)
        except OSError as exc:
            state.update(
                {
                    "status": "completed",
                    "finished_at": utc_now(),
                    "duration_ms": int((time.monotonic() - started) * 1000),
                    "exit_code": 127,
                    "timed_out": False,
                    "launcher_error": str(exc),
                }
            )
            write_state(state_path, state)
            return 0
        state["command_pid"] = process.pid
        write_state(state_path, state)
        timed_out = False
        try:
            exit_code = process.wait(timeout=args.timeout_seconds)
        except subprocess.TimeoutExpired:
            timed_out = True
            process.terminate()
            try:
                exit_code = process.wait(timeout=20)
            except subprocess.TimeoutExpired:
                process.kill()
                exit_code = process.wait()

    state.update(
        {
            "status": "completed",
            "finished_at": utc_now(),
            "duration_ms": int((time.monotonic() - started) * 1000),
            "exit_code": exit_code,
            "timed_out": timed_out,
        }
    )
    write_state(state_path, state)
    return 0


def start(args: argparse.Namespace) -> int:
    state_path = Path(args.state)
    command = command_from_args(args.command)
    current = read_state(state_path)
    if current.get("status") in {"running", "completed"}:
        print(json.dumps({"disposition": current["status"], "state": current}, ensure_ascii=True))
        return 0

    initial = {
        "schema_version": 1,
        "status": "starting",
        "started_at": utc_now(),
        "command": command,
        "stdout_file": str(Path(args.stdout_file)),
        "stderr_file": str(Path(args.stderr_file)),
        "timeout_seconds": args.timeout_seconds,
    }
    write_state(state_path, initial)
    worker_command = [
        sys.executable,
        str(Path(__file__).resolve()),
        "worker",
        "--state",
        str(state_path),
        "--stdout-file",
        args.stdout_file,
        "--stderr-file",
        args.stderr_file,
        "--timeout-seconds",
        str(args.timeout_seconds),
        "--",
        *command,
    ]
    process = subprocess.Popen(
        worker_command,
        stdin=subprocess.DEVNULL,
        stdout=subprocess.DEVNULL,
        stderr=subprocess.DEVNULL,
        start_new_session=True,
        close_fds=True,
    )
    initial["worker_pid"] = process.pid
    # The worker may already have completed; never overwrite its final state.
    current = read_state(state_path)
    if current.get("status") == "starting":
        write_state(state_path, initial)
        current = initial
    print(json.dumps({"disposition": current.get("status", "running"), "state": current}, ensure_ascii=True))
    return 0


def wait_for_job(args: argparse.Namespace) -> int:
    state_path = Path(args.state)
    deadline = time.monotonic() + args.max_wait_seconds
    heartbeat = max(1, args.heartbeat_seconds)
    next_heartbeat = time.monotonic()
    while True:
        state = read_state(state_path)
        status = state.get("status")
        if status == "completed":
            print(json.dumps({"disposition": "completed", "state": state}, ensure_ascii=True))
            return 0
        if status not in {"starting", "running"}:
            raise RuntimeError(f"unsupported job state: {status!r}")
        now = time.monotonic()
        if now >= deadline:
            print(json.dumps({"disposition": "running", "state": state}, ensure_ascii=True))
            return 0
        if now >= next_heartbeat:
            print(json.dumps({"disposition": "waiting", "state": state}, ensure_ascii=True), flush=True)
            next_heartbeat = now + heartbeat
        time.sleep(min(1, max(0.05, deadline - now)))


def inspect(args: argparse.Namespace) -> int:
    print(json.dumps(read_state(Path(args.state)), ensure_ascii=True))
    return 0


def add_job_arguments(parser: argparse.ArgumentParser, command: bool = False) -> None:
    parser.add_argument("--state", required=True)
    if command:
        parser.add_argument("--stdout-file", required=True)
        parser.add_argument("--stderr-file", required=True)
        parser.add_argument("--timeout-seconds", type=int, default=1500)
        parser.add_argument("command", nargs=argparse.REMAINDER)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    subparsers = parser.add_subparsers(dest="mode", required=True)
    start_parser = subparsers.add_parser("start")
    add_job_arguments(start_parser, command=True)
    start_parser.set_defaults(handler=start)
    worker_parser = subparsers.add_parser("worker")
    add_job_arguments(worker_parser, command=True)
    worker_parser.set_defaults(handler=worker)
    wait_parser = subparsers.add_parser("wait")
    add_job_arguments(wait_parser)
    wait_parser.add_argument("--max-wait-seconds", type=int, default=20)
    wait_parser.add_argument("--heartbeat-seconds", type=int, default=5)
    wait_parser.set_defaults(handler=wait_for_job)
    inspect_parser = subparsers.add_parser("inspect")
    add_job_arguments(inspect_parser)
    inspect_parser.set_defaults(handler=inspect)
    args = parser.parse_args()
    if getattr(args, "timeout_seconds", 1) < 1:
        parser.error("--timeout-seconds must be positive")
    if getattr(args, "max_wait_seconds", 1) < 1:
        parser.error("--max-wait-seconds must be positive")
    try:
        return args.handler(args)
    except RuntimeError as exc:
        print(f"run_image_edit_job: {exc}", file=sys.stderr)
        return 2


if __name__ == "__main__":
    raise SystemExit(main())
