#!/usr/bin/env python3
"""Transfer GitHub App settings and update Compose dotenv without logging values."""

import json
import os
from pathlib import Path
import re
import sys


KEYS = (
    "GITHUB_APP_SLUG",
    "GITHUB_WEBHOOK_SECRET",
    "GITHUB_APP_CLIENT_ID",
    "GITHUB_APP_CLIENT_SECRET",
    "GITHUB_APP_ID",
    "GITHUB_APP_PRIVATE_KEY",
)
# Match whole assignments, including quoted multiline PEM values, so rotations
# remove old credentials without changing unrelated dotenv bytes.
ASSIGNMENT = re.compile(
    r"^(?:export[ \t]+)?(?P<key>[A-Za-z_][A-Za-z0-9_]*)[ \t]*=[ \t]*"
    r"(?:'(?:\\.|[^'\\])*'|\"(?:\\.|[^\"\\])*\"|[^\r\n]*)[^\r\n]*(?:\r?\n|$)",
    re.MULTILINE,
)


def main():
    mode, *paths = sys.argv[1:]
    if mode == "capture" and len(paths) == 1:
        values = {key: os.environ.get(key, "") for key in KEYS}
    elif mode == "merge" and len(paths) == 3:
        values = json.loads(Path(paths[1]).read_text())
    else:
        raise ValueError("Invalid GitHub App configuration command")
    if not isinstance(values, dict) or set(values) != set(KEYS):
        raise ValueError("GitHub App configuration must contain all six settings")
    for key in KEYS:
        if not isinstance(values[key], str) or not values[key].strip() or "\0" in values[key]:
            raise ValueError(f"Missing or invalid GitHub App setting: {key}")
    if mode == "capture":
        output = json.dumps(values)
        destination = paths[0]
    else:
        original = Path(paths[0]).read_bytes().decode()
        output = ASSIGNMENT.sub(lambda match: "" if match["key"] in KEYS else match[0], original)
        if output and not output.endswith("\n"):
            output += "\n"
        for key in KEYS:
            # JSON quoting preserves PEM newlines and backslashes. Compose's
            # dollar escape prevents interpolation of credential contents.
            value = json.dumps(values[key], ensure_ascii=False).replace("$", "$$")
            output += f"{key}={value}\n"
        destination = paths[2]
    with open(destination, "w", opener=lambda path, flags: os.open(path, flags, 0o600)) as stream:
        stream.write(output)


if __name__ == "__main__":
    try:
        main()
    except (ValueError, OSError):
        # Malformed JSON and I/O errors can contain secret data or file contents.
        sys.exit("Could not prepare GitHub App configuration; check all six production settings.")
