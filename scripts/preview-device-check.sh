#!/usr/bin/env bash
set -euo pipefail

RUNTIME_URL="${MULTICA_DEVICE_RUNTIME_URL:-}"
TOKEN="${MULTICA_DEVICE_RUNTIME_TOKEN:-}"

usage() {
  cat <<'EOF'
Usage: scripts/preview-device-check.sh --url http://127.0.0.1:18081 [--token TOKEN]

Checks the local Android Device Runtime and requires at least one ADB device in
the "device" state. Run this on the machine hosting the emulator or USB device.
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --url)
      [[ $# -ge 2 ]] || { usage >&2; exit 2; }
      RUNTIME_URL="$2"
      shift 2
      ;;
    --token)
      [[ $# -ge 2 ]] || { usage >&2; exit 2; }
      TOKEN="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      usage >&2
      exit 2
      ;;
  esac
done

[[ -n "$RUNTIME_URL" ]] || { echo "Device Runtime URL is required" >&2; exit 1; }
[[ "$RUNTIME_URL" =~ ^http://(127\.0\.0\.1|localhost|\[::1\])(:[0-9]+)?$ ]] \
  || { echo "Device Runtime URL must be a loopback HTTP URL" >&2; exit 1; }
[[ -n "$TOKEN" ]] || { echo "MULTICA_DEVICE_RUNTIME_TOKEN is required" >&2; exit 1; }

devices="$(curl --fail --silent --show-error --max-time 10 \
  -H "Authorization: Bearer ${TOKEN}" \
  "${RUNTIME_URL%/}/api/devices")" \
  || { echo "Device Runtime is unavailable" >&2; exit 1; }

if ! grep -Eq '"state"[[:space:]]*:[[:space:]]*"device"' <<<"$devices"; then
  echo "No authorized Android emulator or USB device is online: $devices" >&2
  exit 1
fi

echo "Preview Device Runtime check passed."
