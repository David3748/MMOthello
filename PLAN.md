# MMOthello — Implementation Plan

A 1000×1000 persistent Othello board with massive concurrent play. Inspired by [one-million-chessboards](https://github.com/nolenroyalty/one-million-chessboards).

---

## 0. Decisions locked in

- **Board**: 1000×1000, persistent, no game-end.
- **Teams**: two — Black and White. Players assigned on join.
- **Cadence**: 1 placement per player per 5 s, server-enforced.
- **Placement rule**: strict Othello — must flip ≥1 opponent piece.
- **Seeding**: grid of standard 4-stone Othello starting clusters every 50 cells at boot, so frontiers exist immediately.
- **Stack**: Go server, raw WebSocket, TypeScript + Canvas client, SQLite (or flat file) for snapshots.
- **Auth**: anonymous play with cookie-issued session tokens. Optional account upgrade later.
- **Score**: live territory % per team. No win condition.

---

## 1. Repository layout

- [ ] `server/` — Go module
  - [ ] `cmd/mmothello/main.go` — entrypoint
  - [ ] `internal/board/` — packed board, chunks, flip logic
  - [ ] `internal/game/` — placement validation, cooldown, scoring
  - [ ] `internal/net/` — WebSocket hub, protocol encode/decode
  - [ ] `internal/persist/` — snapshot + WAL
  - [ ] `internal/auth/` — session tokens
- [ ] `client/` — Vite + TypeScript
  - [ ] `src/render/` — canvas renderer, viewport
  - [ ] `src/net/` — WS client, protocol
  - [ ] `src/state/` — local board cache, chunk LRU
  - [ ] `src/ui/` — cooldown ring, score, controls
- [ ] `proto/` — shared protocol definitions (binary layout doc)
- [ ] `scripts/` — load test, snapshot inspector
- [ ] `README.md`, `PLAN.md`, `Makefile`

---

## 2. Data model

### 2.1 Board

- [ ] Single packed `[]byte` of length `1_000_000 / 4 = 250_000` bytes (2 bits/cell).
  - Encoding: `00` empty, `01` black, `10` white, `11` reserved.
  - Helpers: `Get(x, y) Cell`, `Set(x, y, Cell)`.
- [ ] Chunk grid: 20×20 chunks of 50×50 cells each = 400 chunks.
- [ ] Per-chunk metadata struct:
  - [ ] `version uint64` — bumped on every cell change in chunk
  - [ ] `mu sync.Mutex` — guards writes to cells in this chunk
  - [ ] `subscribers map[clientID]struct{}` — for broadcast routing
  - [ ] `blackCount, whiteCount uint32` — for live score

### 2.2 Player session

```go
type Session struct {
    ID            uint64    // monotonic
    Token         [32]byte  // random, in cookie
    Team          uint8     // 1=black, 2=white
    LastPlaceUnix int64     // ms
    IP            netip.Addr
    Conn          *websocket.Conn
}
```

- [ ] Sessions held in `map[uint64]*Session` under `sync.RWMutex`.
- [ ] Token → SessionID lookup map.

### 2.3 Wire types

- [ ] `Cell = uint8` (0/1/2)
- [ ] `Coord = struct{ X, Y uint16 }`
- [ ] `Delta = struct{ X, Y uint16; Cell uint8 }`

---

## 3. Wire protocol (binary, little-endian)

All frames are length-prefixed binary WS messages. First byte is opcode.

### 3.1 Client → Server

- [ ] `0x01 Hello { token[32] }` — resume session, or omit token to get new one
- [ ] `0x02 Subscribe { chunkID uint16 }` — chunk in `[0, 400)`
- [ ] `0x03 Unsubscribe { chunkID uint16 }`
- [ ] `0x04 Place { x uint16, y uint16 }`
- [ ] `0x05 Ping { nonce uint32 }`

### 3.2 Server → Client

- [ ] `0x81 Welcome { sessionID uint64, token[32], team uint8, serverTimeMs int64 }`
- [ ] `0x82 Snapshot { chunkID uint16, version uint64, packed[625] }` (625 = 50·50/4·... see §3.3)
- [ ] `0x83 Delta { count uint16, entries[count]{ x uint16, y uint16, cell uint8 } }`
- [ ] `0x84 PlaceAck { ok uint8, nextAllowedMs int64, errCode uint8 }`
- [ ] `0x85 Score { black uint32, white uint32, empty uint32 }` — every 1 s broadcast
- [ ] `0x86 Pong { nonce uint32, serverTimeMs int64 }`
- [ ] `0x87 Error { code uint8, msg string }`

### 3.3 Snapshot encoding

- [ ] 50×50 chunk = 2500 cells × 2 bits = 625 bytes.
- [ ] Row-major, MSB-first within byte: cell `i` is at byte `i/4`, bit shift `(3 - i%4) * 2`.
- [ ] Document this in `proto/README.md` so client/server agree.

### 3.4 Error codes

- [ ] `1` cooldown active
- [ ] `2` cell occupied
- [ ] `3` no flips (illegal Othello placement)
- [ ] `4` out of bounds
- [ ] `5` not authenticated
- [ ] `6` rate limited

---

## 4. Server: core systems

### 4.1 Board package (`internal/board/`)

- [ ] `Board` struct: packed bytes + 400 chunks.
- [ ] `Get(x, y) Cell`, `Set(x, y, Cell)` — no locking, caller's responsibility.
- [ ] `ChunkOf(x, y) uint16` and `ChunkBounds(id) (x0, y0, x1, y1)`.
- [ ] `Seed()` — write starting 4-stone clusters every 50 cells:
  - At each `(50i+24, 50j+24)` for `i,j ∈ [0,20)`, place the 2×2 Othello opening.
- [ ] **Tests**:
  - [ ] Round-trip Get/Set every cell value.
  - [ ] Packing endianness matches doc.
  - [ ] Seed produces correct counts.

### 4.2 Flip algorithm (`internal/board/flip.go`)

- [ ] `ComputeFlips(b *Board, x, y int, team Cell) []Coord`:
  - For each of 8 directions `(dx, dy)`:
    - Walk from `(x+dx, y+dy)`, collecting opponent cells.
    - Stop at: own color (commit run), empty (discard run), edge (discard run), or **50 steps** (discard run — cap).
  - Return concatenated runs.
- [ ] Reject placement if `len(flips) == 0`.
- [ ] **Tests** (table-driven):
  - [ ] Each of 8 directions individually.
  - [ ] Multi-direction simultaneous flips.
  - [ ] No-flip cases (empty terminator, edge terminator, lone opponent).
  - [ ] 50-cell cap respected.
  - [ ] Corner and edge placements.

### 4.3 Game logic (`internal/game/`)

- [ ] `Place(sess *Session, x, y int) (flips []Coord, err error)`:
  1. Bounds check.
  2. Cooldown: `now - sess.LastPlaceUnix >= 5000ms`. Else `ErrCooldown`.
  3. Identify all chunks touched by `(x,y)` ∪ flip cells. Worst case: a flip line crosses chunks.
     - First pass: compute flips with `RLock` on chunks involved.
     - Second pass: take `Lock` on every touched chunk in **sorted order by chunkID** to avoid deadlock.
     - Re-validate cell still empty under lock; if not, abort and retry once.
  4. Apply: set cell, flip pieces, bump each touched chunk's `version`, update `blackCount/whiteCount`.
  5. Set `sess.LastPlaceUnix = now`.
  6. Return flips for broadcast.
- [ ] **Tests**:
  - [ ] Cooldown rejects within 5 s.
  - [ ] Concurrent placements at same cell — exactly one wins.
  - [ ] Score counts stay consistent under concurrent flips (run with `-race`).

### 4.4 Net package (`internal/net/`)

- [ ] `Hub` struct holding all sessions + per-chunk subscriber sets.
- [ ] `Hub.Broadcast(chunkIDs []uint16, deltas []Delta)`:
  - Group deltas per chunk, send `0x83 Delta` to each subscriber of those chunks.
  - Per-client send via buffered channel (size 256). If full → mark `needsResnapshot=true`, drop until they catch up.
- [ ] `Hub.HandleConn(ws)`:
  - Read `Hello`, issue/restore session, send `Welcome`.
  - Loop reading frames; dispatch by opcode.
  - On disconnect, remove from all chunk subscriber sets.
- [ ] Subscribe handler: send latest `Snapshot` for the chunk synchronously, then add to subscribers.
- [ ] Place handler: call `game.Place`, broadcast `Delta` to affected chunks, send `PlaceAck` to player.
- [ ] Score broadcaster: ticker at 1 Hz computes totals, sends `Score` to all clients.
- [ ] **Tests**:
  - [ ] Frame encode/decode round-trip for every opcode.
  - [ ] Slow-consumer drop logic doesn't stall fast peers (load test).

### 4.5 Auth (`internal/auth/`)

- [ ] Generate 32-byte random token, store hash in memory map.
- [ ] HTTP bootstrap endpoint `GET /session` issues a token in `HttpOnly` cookie before WS upgrade.
- [ ] Token rotation on `Hello` if older than 30 days.

### 4.6 Persistence (`internal/persist/`)

- [ ] **Snapshot**: every 60 s, serialize board to `snapshot-<unix>.bin` (250 KB) + `meta.json` (chunk versions, score, last placed-id).
- [ ] **WAL**: append every committed placement as `(sessionID uint64, x, y uint16, ts int64)` to `wal.log`. Fsync every 250 ms (batch).
- [ ] **Recovery**: on boot, load latest snapshot, replay WAL entries past snapshot timestamp.
- [ ] **Compaction**: on snapshot success, truncate WAL.
- [ ] **Tests**:
  - [ ] Kill-9 mid-write — recovery yields consistent board.
  - [ ] Snapshot + replay equals continuous run.

### 4.7 Rate limiting & abuse

- [ ] Per-IP token bucket: 1 placement per 5 s (matches per-session cooldown), burst 1.
- [ ] Per-IP connection cap: 5 simultaneous WS.
- [ ] Global place rate cap: configurable (start at 5000/s server-wide).
- [ ] Optional hCaptcha gate on `/session` once we ship.

---

## 5. Client

### 5.1 Renderer (`client/src/render/`)

- [ ] `Viewport { x, y, zoom }` — camera in world coords; zoom in `[0.25, 16]` px/cell.
- [ ] `Canvas2DRenderer`:
  - Draw cells in viewport from local chunk cache.
  - At zoom < 1 px/cell: rasterize chunk to offscreen canvas, blit.
  - At zoom ≥ 4 px/cell: draw stones as circles with shadow + grid.
- [ ] Pan: drag with mouse / touch. Zoom: wheel / pinch.
- [ ] Cooldown ring around cursor showing time-until-next-placement.
- [ ] Hover preview: ghost stone of own team color at hovered cell.

### 5.2 State (`client/src/state/`)

- [ ] `ChunkCache` — LRU, capacity 200 chunks (~125 KB raw + overhead, fine).
- [ ] On viewport change: diff currently-visible chunks vs subscribed; send `Subscribe` / `Unsubscribe`.
- [ ] Apply `Snapshot` and `Delta` messages to cache; mark dirty regions for renderer.

### 5.3 Net (`client/src/net/`)

- [ ] WS client with reconnect + backoff (250 ms → 8 s).
- [ ] Frame encoder/decoder mirroring server (shared spec doc).
- [ ] On reconnect: re-`Hello` with stored token, re-subscribe to current viewport chunks.

### 5.4 UI (`client/src/ui/`)

- [ ] Top bar: live score (B / W / empty %), team badge, ping ms.
- [ ] Optimistic placement: on click, render local ghost in own color; lock to confirmed state on `PlaceAck`. On reject, revert + flash error toast.
- [ ] Help overlay: rules, cooldown, controls.

### 5.5 Tests

- [ ] Unit tests for chunk-cache apply/snapshot.
- [ ] Headless integration test: spin local server, drive WS client, assert renderer state after scripted placements.

---

## 6. Phased build

### Phase 1 — MVP (100×100, single server)
- [ ] Board package + flip algorithm with tests
- [ ] Game.Place with cooldown
- [ ] WS hub, full protocol
- [ ] Minimal client: fixed viewport on whole 100×100, click to place
- [ ] Verify rules end-to-end with 5 simultaneous browser tabs

### Phase 2 — Scale to 1000×1000
- [ ] Chunked subscriptions on server and client
- [ ] Pan/zoom canvas renderer
- [ ] Snapshot encoding for chunks
- [ ] Score broadcaster
- [ ] Load-test script: spawn 1000 bot clients, place at random valid frontiers

### Phase 3 — Persistence
- [ ] Snapshot timer + writer
- [ ] WAL writer with batched fsync
- [ ] Recovery on boot
- [ ] Crash test in CI

### Phase 4 — Anti-abuse + accounts
- [ ] Per-IP rate limit
- [ ] Connection caps
- [ ] hCaptcha on `/session`
- [ ] Bot detection: log accounts placing at perfectly periodic intervals

### Phase 5 — Polish
- [ ] Mobile touch controls
- [ ] Mini-map showing whole 1000×1000 territory map
- [ ] Per-player attribution heatmap (defer 4 MB `last_placer` array until here)
- [ ] Leaderboards (most flips, longest line, etc.)

---

## 7. Performance targets

- [ ] Place latency P99 ≤ 50 ms server-side under 10k concurrent players.
- [ ] Delta broadcast fanout: 1 placement → ≤ 1k clients receive an update within 100 ms.
- [ ] Snapshot serialization < 20 ms (250 KB; trivial).
- [ ] Client maintains 60 fps while panning at zoom 4.

---

## 8. Open questions to revisit

- [ ] Should we expose a public read-only HTTP endpoint that returns a PNG of the whole board for embedding?
- [ ] Team auto-balancing if one side dominates — auto-assign new players to losing side?
- [ ] Soft "regions" or chat? Out of scope for v1.
- [ ] Replay/timelapse playback from WAL?

---

## 9. Definition of done (v1)

- [ ] 1000×1000 board playable by 1000+ concurrent browser clients.
- [ ] Strict Othello rules enforced server-side.
- [ ] 5 s cooldown enforced server-side.
- [ ] Server survives crash with no lost placements past last fsync.
- [ ] Live score updates every second.
- [ ] Public deploy with HTTPS + WSS behind a reverse proxy.
