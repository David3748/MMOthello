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
- Teams: exactly two (`Black`, `White`), assigned on join using live players who placed within the last minute.
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
# or:
./scripts/loadtest/run.sh --base http://localhost:8080 --clients 1000 --duration 60
```

Server tests: `make server-test`. Race detector: `make server-race`.
Client tests: `make client-test`.

## Local Demo

1. Start the server: `make server-run`.
2. Start the client in another terminal: `make client-dev`.
3. Open `http://127.0.0.1:5173/`.
4. Confirm the top bar shows `status: connected`, a team, score percentages, and ping.
5. Use `?` for rules, drag/wheel or pinch to move around, then click the board to see placement feedback and cooldown.

Run the demo smoke check after both server and client are running:

```bash
make smoke
# or:
./scripts/smoke/run.sh --url http://127.0.0.1:5173/
```

The smoke check opens headless Chrome, verifies the session/websocket path,
waits for score and snapshot data, toggles help, clicks the board to observe
placement feedback, and checks mobile HUD controls for overlap. If port `8080`
is already in use, either stop the existing process or run the server on another
port and update the Vite proxy before starting the client.

## Operations

The server exposes:

- `GET /healthz` for process health checks.
- `GET /session` to issue the anonymous session cookie and return `sessionID`, `team`, and `cooldownMs`.
- `GET /stats` for operational/debug JSON: black, white, empty, connected clients, and live players by team.
- `GET /ws` for binary WebSocket play traffic; browsers authenticate with the cookie, bots may use `?token=<hex>`.

Useful environment variables:

- `MMOTHELLO_ADDR` (default `:8080`)
- `MMOTHELLO_DATA` (default `./data`)
- `MMOTHELLO_COOLDOWN_MS` (default `5000`)
- `MMOTHELLO_PLACE_RATE` and `MMOTHELLO_PLACE_BURST` for load tests or deployments
- `MMOTHELLO_CONN_CAP` (default `5`)

Single-VPS deployment:

1. Build the client with `( cd client && npm ci && npm run build )`.
2. Build/run the server with `( cd server && go build ./cmd/mmothello )`.
3. Run the binary with `MMOTHELLO_DATA` pointed at a persistent disk directory.
4. Put a reverse proxy such as Caddy or nginx in front for HTTPS/WSS and proxy `/ws`, `/session`, `/stats`, and static files to the Go server.
5. Back up `meta.json`, `snapshot-*.bin`, and `wal.log`; `/healthz` is the health-check endpoint.

Inspection tools:

```bash
./scripts/inspect/run.sh --file ./server/data/meta.json
./scripts/inspect/run.sh --file ./server/data/wal.log --kind wal
./scripts/inspect/run.sh --file ./server/data/snapshot-1700000000.bin --kind snapshot
```

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
