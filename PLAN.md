# MMOthello V1 Roadmap

This is the living implementation roadmap for the single-server v1. The old scaffold has been collapsed into current status, active work, and next milestones so the file reflects the repository as it exists now.

## V1 Contract

- Board: persistent `1000x1000` shared Othello grid.
- Teams: anonymous players assigned to Black or White, balanced by players who placed within the last minute.
- Cadence: one server-authoritative placement every `5s` by default, configurable with `MMOTHELLO_COOLDOWN_MS`.
- Rule: a placement must flip at least one opposing stone.
- Runtime: one Go server serving the Vite-built client and raw binary WebSocket traffic.
- Persistence: full-board snapshots plus WAL records for committed placements.
- Deployment target: one VPS behind HTTPS/WSS with local persistent disk.

## Completed

- Packed board storage, chunk mapping, seeding, scoring, and flip calculation.
- Server-side placement validation, cooldown, chunk locking, and concurrent placement tests.
- Binary protocol encoder/decoder for client and server opcodes.
- Cookie-backed anonymous sessions plus query-token bot fallback.
- WebSocket hub, chunk subscriptions, snapshots, deltas, score broadcast, and slow-consumer flags.
- Snapshot/WAL persistence with millisecond metadata, team-bearing WAL records, replay, and compaction.
- Canvas client with pan/zoom, chunk cache, mini-map, score/team/ping/cooldown UI, hover preview, optimistic placement, and touch pinch zoom.
- Node bot load tester and snapshot/WAL inspector scripts.
- CI for server tests, server race tests, client tests, and client build.
- Demo polish: responsive HUD layout, placement feedback states, recent activity text, and dependency-free browser smoke script.

## Current Polish

- Run the local demo path regularly: server, Vite client, browser at `http://127.0.0.1:5173/`, then `make smoke`.
- Keep the browser smoke check focused on local demo confidence: session, websocket, first snapshot, score, help toggle, placement feedback, and mobile HUD overlap.
- Keep protocol docs, README, and implementation synchronized whenever a frame shape or public endpoint changes.
- Keep advanced features out of v1 unless they unblock the demo: accounts, captcha, chat, leaderboards, attribution heatmaps, public PNG export, and replay/timelapse.

## Acceptance Tests

- `cd server && go test ./...`
- `cd server && go test -race ./...`
- `cd client && npm test`
- `cd client && npm run build`
- `make smoke`
- `./scripts/loadtest/run.sh --base http://localhost:8080 --clients 1000 --duration 60`

V1 is ready when a single server survives the load test without crashing, reports bounded latency in the bot output, preserves board state across restart, and remains playable from desktop and mobile browsers.

## Later

- More detailed observability for placement latency, broadcast queue depth, and slow-consumer recovery.
- Optional deploy packaging with a systemd unit and a sample Caddyfile or nginx config.
- Full load-test gate on a clean single-VPS environment.
