# Load Test Scripts

This directory contains load-test tooling for MMOthello.

## Purpose

The bot runner simulates many websocket clients placing stones at controlled rates, then reports placement acks, error codes, and p50/p95/p99 ack latency.

## Tools

- `bots.mjs`: Node 22+ websocket bot runner.
- `run.sh`: wrapper that accepts HTTP or websocket targets and forwards settings to `bots.mjs`.

## Usage

```bash
./scripts/loadtest/run.sh --help
./scripts/loadtest/run.sh --base http://localhost:8080 --clients 100 --duration 60
./scripts/loadtest/run.sh --url ws://localhost:8080/ws --clients 1000 --duration 60 --cooldown-ms 5000
```
