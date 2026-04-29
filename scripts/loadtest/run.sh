#!/usr/bin/env bash
set -euo pipefail

show_help() {
  cat <<'EOF'
MMOthello load-test placeholder

Usage:
  run.sh [--url WS_URL] [--clients N] [--duration SECONDS]
  run.sh --help

Options:
  --url       WebSocket endpoint (default: ws://localhost:8080/ws)
  --clients   Number of simulated clients (default: 100)
  --duration  Test duration in seconds (default: 60)
EOF
}

url="ws://localhost:8080/ws"
clients="100"
duration="60"

while [[ $# -gt 0 ]]; do
  case "$1" in
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

echo "[stub] Load test not implemented yet."
echo "[stub] Target URL: ${url}"
echo "[stub] Simulated clients: ${clients}"
echo "[stub] Duration (s): ${duration}"
echo "[stub] Next step: implement websocket workers and metrics collection."
