# Load Test Scripts

This directory contains load-test tooling placeholders for MMOthello.

## Purpose

Planned use (Phase 2) is to simulate many websocket clients placing stones at controlled rates, then report latency/error/fanout metrics.

## Current Stub

- `run.sh`: executable placeholder script that validates arguments and prints the intended run configuration.

## Usage

```bash
./scripts/loadtest/run.sh --help
./scripts/loadtest/run.sh --url ws://localhost:8080/ws --clients 100 --duration 60
```

The script currently does not open websocket connections yet; it is a scaffold for future implementation.
