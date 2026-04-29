# Inspect Scripts

This directory contains inspection/debug tooling for MMOthello snapshots and state artifacts.

## Purpose

The inspector reads snapshot binaries, WAL files, and snapshot metadata to print human-readable summaries for debugging recovery correctness.

## Tool

- `run.sh`: detects artifact type by filename or accepts `--kind snapshot|wal|meta`.

## Usage

```bash
./scripts/inspect/run.sh --help
./scripts/inspect/run.sh --file ./server/data/meta.json
./scripts/inspect/run.sh --file ./server/data/wal.log --kind wal
./scripts/inspect/run.sh --file ./server/data/snapshot-1700000000.bin --kind snapshot
```
