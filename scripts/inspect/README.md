# Inspect Scripts

This directory contains inspection/debug tooling placeholders for MMOthello snapshots and state artifacts.

## Purpose

Planned use (Phase 3) is to inspect snapshot binaries/WAL metadata and print human-readable summaries for debugging recovery correctness.

## Current Stub

- `run.sh`: executable placeholder script that accepts a path and reports what would be inspected.

## Usage

```bash
./scripts/inspect/run.sh --help
./scripts/inspect/run.sh --file ./snapshot-latest.bin
```

The script currently does not parse binary artifacts yet; it is a scaffold for future implementation.
