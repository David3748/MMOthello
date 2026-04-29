#!/usr/bin/env bash
set -euo pipefail

show_help() {
  cat <<'EOF'
MMOthello load test

Usage:
  run.sh [--base HTTP_BASE] [--url WS_URL] [--clients N] [--duration SECONDS]
         [--cooldown-ms MS] [--place-rate RATE]
  run.sh --help

Options:
  --base         HTTP server base URL (default: http://localhost:8080)
  --url          WebSocket endpoint; converted to an HTTP base when --base is omitted
  --clients      Number of simulated clients (default: 100)
  --duration     Test duration in seconds (default: 60)
  --cooldown-ms  Bot placement cadence in ms (default: MMOTHELLO_COOLDOWN_MS or 5000)
  --place-rate   Export MMOTHELLO_PLACE_RATE for matching server runs
EOF
}

base="http://localhost:8080"
url=""
clients="100"
duration="60"
cooldown_ms="${MMOTHELLO_COOLDOWN_MS:-5000}"
place_rate="${MMOTHELLO_PLACE_RATE:-}"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --base)
      base="${2:-}"
      shift 2
      ;;
    --url)
      url="${2:-}"
      shift 2
      ;;
    --clients)
      clients="${2:-}"
      shift 2
      ;;
    --duration)
      duration="${2:-}"
      shift 2
      ;;
    --cooldown-ms)
      cooldown_ms="${2:-}"
      shift 2
      ;;
    --place-rate)
      place_rate="${2:-}"
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

if [[ -n "${url}" ]]; then
  base="${url/ws:/http:}"
  base="${base/wss:/https:}"
  base="${base%/ws}"
fi

if [[ -n "${place_rate}" ]]; then
  export MMOTHELLO_PLACE_RATE="${place_rate}"
fi
export MMOTHELLO_COOLDOWN_MS="${cooldown_ms}"

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
node "${script_dir}/bots.mjs" \
  --base "${base}" \
  --clients "${clients}" \
  --duration "${duration}" \
  --cooldown-ms "${cooldown_ms}"
