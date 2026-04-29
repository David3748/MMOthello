#!/usr/bin/env bash
set -euo pipefail

show_help() {
  cat <<'EOF'
MMOthello snapshot/WAL inspector

Usage:
  run.sh [--file PATH] [--kind auto|snapshot|wal|meta]
  run.sh --help

Options:
  --file   Path to snapshot, WAL, or meta.json artifact (default: ./snapshot-latest.bin)
  --kind   Artifact kind (default: auto)
EOF
}

file="./snapshot-latest.bin"
kind="auto"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --file)
      file="${2:-}"
      shift 2
      ;;
    --kind)
      kind="${2:-}"
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

node - "${file}" "${kind}" <<'NODE'
const fs = require("node:fs");
const path = require("node:path");

const file = process.argv[2];
let kind = process.argv[3] || "auto";
if (!fs.existsSync(file)) {
  console.error(`file not found: ${file}`);
  process.exit(1);
}
if (kind === "auto") {
  const name = path.basename(file);
  if (name === "meta.json" || name.endsWith(".json")) kind = "meta";
  else if (name.endsWith(".log")) kind = "wal";
  else kind = "snapshot";
}

const data = fs.readFileSync(file);
if (kind === "meta") {
  const meta = JSON.parse(data.toString("utf8"));
  console.log(`meta timestamp_unix=${meta.timestamp_unix ?? "n/a"} timestamp_ms=${meta.timestamp_ms ?? "n/a"} snapshot_file=${meta.snapshot_file ?? "n/a"}`);
} else if (kind === "wal") {
  const recordSize = 21;
  const complete = Math.floor(data.length / recordSize);
  const partial = data.length % recordSize;
  const byTeam = { 1: 0, 2: 0, other: 0 };
  let minTs = null;
  let maxTs = null;
  for (let offset = 0; offset + recordSize <= data.length; offset += recordSize) {
    const team = data.readUInt8(offset + 12);
    if (team === 1 || team === 2) byTeam[team] += 1;
    else byTeam.other += 1;
    const ts = Number(data.readBigUInt64LE(offset + 13));
    minTs = minTs === null ? ts : Math.min(minTs, ts);
    maxTs = maxTs === null ? ts : Math.max(maxTs, ts);
  }
  console.log(`wal records=${complete} partial_tail_bytes=${partial} team_black=${byTeam[1]} team_white=${byTeam[2]} team_other=${byTeam.other} ts_min=${minTs ?? "n/a"} ts_max=${maxTs ?? "n/a"}`);
} else if (kind === "snapshot") {
  const expectedFull = 250000;
  const expectedChunk = 625;
  if (data.length !== expectedFull && data.length !== expectedChunk) {
    console.log(`snapshot bytes=${data.length} warning=unexpected_size`);
  } else {
    console.log(`snapshot bytes=${data.length} shape=${data.length === expectedFull ? "full-board" : "chunk"}`);
  }
  let empty = 0, black = 0, white = 0, reserved = 0;
  for (const byte of data) {
    for (const shift of [6, 4, 2, 0]) {
      const cell = (byte >> shift) & 0b11;
      if (cell === 0) empty += 1;
      else if (cell === 1) black += 1;
      else if (cell === 2) white += 1;
      else reserved += 1;
    }
  }
  console.log(`cells empty=${empty} black=${black} white=${white} reserved=${reserved}`);
} else {
  console.error(`unknown kind: ${kind}`);
  process.exit(1);
}
NODE
