#!/usr/bin/env bash
set -euo pipefail

show_help() {
  cat <<'EOF'
MMOthello snapshot/state inspector placeholder

Usage:
  run.sh [--file PATH]
  run.sh --help

Options:
  --file   Path to snapshot/WAL/meta artifact (default: ./snapshot-latest.bin)
EOF
}

file="./snapshot-latest.bin"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --file)
      file="${2:-}"
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

echo "[stub] Inspector not implemented yet."
echo "[stub] Target file: ${file}"
if [[ -e "${file}" ]]; then
  echo "[stub] File exists; parser scaffold ready."
else
  echo "[stub] File does not exist; pass --file with a valid path."
fi
echo "[stub] Next step: decode packed board/chunk metadata and WAL records."
