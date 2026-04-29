# MMOthello

MMOthello is a persistent, massively multiplayer Othello board played on a single shared 1000x1000 grid. Players are assigned to Black or White, place one stone every 5 seconds, and all placement legality is enforced server-side using strict Othello flip rules.

## Architecture

The project is designed as a server-authoritative realtime system:

- `server/` (Go): board state, Othello validation, cooldown enforcement, websocket hub, snapshots/WAL, and session handling.
- `client/` (TypeScript + Canvas): pan/zoom renderer, chunk cache, websocket protocol client, and placement UI.
- `proto/`: shared wire protocol and binary packing details so server/client stay consistent.
- `scripts/`: operational tooling including load testing and snapshot/state inspection.

Core design decisions from `PLAN.md`:

- Board size: `1000x1000`, persistent, no game-end.
- Teams: exactly two (`Black`, `White`), assigned on join.
- Placement cadence: one move per player every `5s`.
- Placement legality: must flip at least one opposing stone.
- Initial seeding: standard 2x2 Othello clusters every 50 cells.
- Wire format: binary websocket frames, little-endian fields.

## Quickstart

```bash
# Terminal 1 — server (Go 1.22+):
make server-run                # listens on :8080, data in ./data
# or:
( cd server && go run ./cmd/mmothello )

# Terminal 2 — client dev server (Vite proxies /ws and /session to :8080):
make client-install
make client-dev                # http://localhost:5173

# Terminal 3 — bot loadtest (Node 22+ for built-in WebSocket):
make loadtest                  # 50 bots × 30 s
```

Server tests + race detector: `make server-test` (`cd server && go test -race ./...`).
Client tests: `make client-test`.

## Phased Roadmap

The delivery plan follows `PLAN.md`:

1. **Phase 1 - MVP (100x100)**  
   Build board rules, websocket protocol path, and minimal playable client.
2. **Phase 2 - Scale to 1000x1000**  
   Add chunked subscriptions, pan/zoom rendering, snapshot chunk encoding, and load testing.
3. **Phase 3 - Persistence**  
   Implement periodic snapshots, WAL append/replay, and crash recovery verification.
4. **Phase 4 - Anti-abuse + Accounts**  
   Add per-IP limits, connection caps, and optional human-verification gates.
5. **Phase 5 - Polish**  
   Improve UX/mobile support and add optional advanced features (mini-map, attribution, leaderboards).

For full implementation details, milestones, and performance targets, see `PLAN.md`.
