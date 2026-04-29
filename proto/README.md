# Protocol Notes

This document defines the shared binary websocket protocol contract for MMOthello. It mirrors the decisions in `PLAN.md` and exists to keep server/client encoders and decoders aligned.

## Binary Frame Layout

All websocket application frames are binary and length-prefixed by websocket framing itself. Within each frame:

1. Byte `0`: opcode (`uint8`)
2. Remaining bytes: opcode-specific payload (little-endian for multi-byte numeric fields)

## Cell and Snapshot Packing

Board cells use 2 bits each:

- `00` empty
- `01` black
- `10` white
- `11` reserved

Chunk model (for snapshot messages):

- Chunk size: `50x50` cells
- Cells per chunk: `2500`
- Packed bytes per chunk: `2500 * 2 / 8 = 625`

Packing order:

- Row-major cell index: `i = y * 50 + x`
- Byte index: `i / 4`
- Bit shift: `(3 - (i % 4)) * 2` (MSB-first inside each byte)

Reference pseudocode:

```text
idx = y * 50 + x
b = packed[idx / 4]
shift = (3 - (idx % 4)) * 2
cell = (b >> shift) & 0b11
```

## Opcodes

### Client -> Server

- `0x01` `Hello { token[32] }`
- `0x02` `Subscribe { chunkID uint16 }`
- `0x03` `Unsubscribe { chunkID uint16 }`
- `0x04` `Place { x uint16, y uint16 }`
- `0x05` `Ping { nonce uint32 }`

### Server -> Client

- `0x81` `Welcome { sessionID uint64, token[32], team uint8, serverTimeMs int64 }`
- `0x82` `Snapshot { chunkID uint16, version uint64, packed[625] }`
- `0x83` `Delta { count uint16, entries[count]{ x uint16, y uint16, cell uint8 } }`
- `0x84` `PlaceAck { ok uint8, nextAllowedMs int64, errCode uint8 }`
- `0x85` `Score { black uint32, white uint32, empty uint32 }`
- `0x86` `Pong { nonce uint32, serverTimeMs int64 }`
- `0x87` `Error { code uint8, msg string }`

## WebSocket framing

Each application message is a single binary WebSocket frame. The frame
payload **is** the protocol opcode + body shown above; there is no
additional length prefix because WebSocket already provides framing.

## Authentication

`GET /session` issues a 32-byte token in an HttpOnly `mmothello_token`
cookie. The browser uses the cookie automatically when upgrading `/ws`.
For programmatic clients that can't send custom headers (e.g. Node's
built-in `WebSocket`), the server also accepts `?token=<hex>` on `/ws`.

## Error Codes

Placement and protocol errors should use the shared numeric codes:

- `1` cooldown active
- `2` cell occupied
- `3` no flips (illegal Othello placement)
- `4` out of bounds
- `5` not authenticated
- `6` rate limited

When additional codes are introduced, update this document and corresponding server/client enums together.
