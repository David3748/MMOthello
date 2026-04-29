#!/usr/bin/env bash
set -euo pipefail

show_help() {
  cat <<'EOF'
MMOthello browser smoke check

Usage:
  run.sh [--url URL] [--timeout MS]
  run.sh --help

Options:
  --url       App URL to test (default: http://127.0.0.1:5173/)
  --timeout   Wait time for app readiness in ms (default: 15000)

Set BROWSER=/path/to/chrome if Chrome/Chromium is not on a standard path.
EOF
}

url="http://127.0.0.1:5173/"
timeout="15000"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --url)
      url="${2:-}"
      shift 2
      ;;
    --timeout)
      timeout="${2:-}"
      shift 2
      ;;
    --help|-h)
      show_help
      exit 0
      ;;
    *)
      echo "Unknown option: $1" >&2
      show_help
      exit 1
      ;;
  esac
done

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
node "${script_dir}/browser.mjs" --url "${url}" --timeout "${timeout}"
