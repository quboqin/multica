"""Contract tests for the collector's native pre-analysis task dispatch."""

from __future__ import annotations

import importlib.util
from pathlib import Path
from unittest.mock import Mock, patch

import pytest


MODULE_PATH = Path(__file__).with_name("delegate_preanalysis.py")
SPEC = importlib.util.spec_from_file_location("delegate_preanalysis", MODULE_PATH)
assert SPEC and SPEC.loader
delegate_preanalysis = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(delegate_preanalysis)


def test_resolve_cli_prefers_explicit_path_over_task_runtime_env(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("MULTICA_CLI", r"C:\\runtime\\multica.exe")

    assert delegate_preanalysis.resolve_cli(r"C:\\task\\multica.exe") == r"C:\\task\\multica.exe"
    assert delegate_preanalysis.resolve_cli("") == r"C:\\runtime\\multica.exe"


def test_resolve_cli_uses_path_default_when_task_runtime_does_not_inject_one(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.delenv("MULTICA_CLI", raising=False)
    monkeypatch.setattr(delegate_preanalysis.os, "name", "posix")

    assert delegate_preanalysis.resolve_cli("") == "multica"


def test_resolve_cli_prefers_current_windows_command_shim(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.delenv("MULTICA_CLI", raising=False)
    monkeypatch.setattr(delegate_preanalysis.os, "name", "nt")
    monkeypatch.setattr(delegate_preanalysis.shutil, "which", lambda name: r"C:\\runtime\\multica.com" if name == "multica.com" else None)

    assert delegate_preanalysis.resolve_cli("") == r"C:\\runtime\\multica.com"


def test_missing_task_fanout_fails_with_cli_upgrade_action() -> None:
    completed = Mock(returncode=1, stdout="", stderr='unknown command "task" for "multica"')

    with patch.object(delegate_preanalysis.subprocess, "run", return_value=completed):
        with pytest.raises(RuntimeError, match="multica task fanout unavailable; upgrade CLI"):
            delegate_preanalysis.ensure_task_fanout_available(["C:\\old-runtime\\multica.exe"])


def test_available_task_fanout_accepts_task_runtime_cli() -> None:
    completed = Mock(returncode=0, stdout="fanout help", stderr="")

    with patch.object(delegate_preanalysis.subprocess, "run", return_value=completed) as run:
        delegate_preanalysis.ensure_task_fanout_available(["C:\\task-runtime\\multica.exe", "--profile", "direct-image2"])

    assert run.call_args.args[0] == ["C:\\task-runtime\\multica.exe", "--profile", "direct-image2", "task", "fanout", "--help"]
