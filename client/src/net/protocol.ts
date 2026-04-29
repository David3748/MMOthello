import type { Cell, DeltaEntry } from "../state/chunk";

export const ClientOpcode = {
  Hello: 0x01,
  Subscribe: 0x02,
  Unsubscribe: 0x03,
  Place: 0x04,
  Ping: 0x05,
} as const;

export const ServerOpcode = {
  Welcome: 0x81,
  Snapshot: 0x82,
  Delta: 0x83,
  PlaceAck: 0x84,
  Score: 0x85,
  Pong: 0x86,
  Error: 0x87,
} as const;

export type DecodedServerFrame =
  | { opcode: 0x81; sessionId: bigint; token: Uint8Array; team: Cell; serverTimeMs: bigint }
  | { opcode: 0x82; chunkId: number; version: bigint; packed: Uint8Array }
  | { opcode: 0x83; count: number; entries: DeltaEntry[] }
  | { opcode: 0x84; ok: number; nextAllowedMs: bigint; errCode: number }
  | { opcode: 0x85; black: number; white: number; empty: number }
  | { opcode: 0x86; nonce: number; serverTimeMs: bigint }
  | { opcode: 0x87; code: number; message: string };

export function encodeHello(token?: Uint8Array): ArrayBuffer {
  const payloadLength = 1 + (token?.length ?? 0);
  const out = new Uint8Array(payloadLength);
  out[0] = ClientOpcode.Hello;
  if (token) out.set(token, 1);
  return out.buffer;
}

export function encodeSubscribe(chunkId: number): ArrayBuffer {
  return encodeU16Frame(ClientOpcode.Subscribe, chunkId);
}

export function encodeUnsubscribe(chunkId: number): ArrayBuffer {
  return encodeU16Frame(ClientOpcode.Unsubscribe, chunkId);
}

export function encodePlace(x: number, y: number): ArrayBuffer {
  const out = new ArrayBuffer(5);
  const view = new DataView(out);
  view.setUint8(0, ClientOpcode.Place);
  view.setUint16(1, x, true);
  view.setUint16(3, y, true);
  return out;
}

export function encodePing(nonce: number): ArrayBuffer {
  const out = new ArrayBuffer(5);
  const view = new DataView(out);
  view.setUint8(0, ClientOpcode.Ping);
  view.setUint32(1, nonce, true);
  return out;
}

export function decodeServerFrame(buffer: ArrayBuffer): DecodedServerFrame {
  const view = new DataView(buffer);
  const bytes = new Uint8Array(buffer);
  const opcode = view.getUint8(0);
  switch (opcode) {
    case ServerOpcode.Welcome:
      return {
        opcode,
        sessionId: view.getBigUint64(1, true),
        token: bytes.slice(9, 41),
        team: view.getUint8(41) as Cell,
        serverTimeMs: view.getBigInt64(42, true),
      };
    case ServerOpcode.Snapshot:
      return {
        opcode,
        chunkId: view.getUint16(1, true),
        version: view.getBigUint64(3, true),
        packed: bytes.slice(11),
      };
    case ServerOpcode.Delta: {
      const count = view.getUint16(1, true);
      const entries: DeltaEntry[] = [];
      let offset = 3;
      for (let i = 0; i < count; i += 1) {
        entries.push({
          x: view.getUint16(offset, true),
          y: view.getUint16(offset + 2, true),
          cell: view.getUint8(offset + 4) as Cell,
        });
        offset += 5;
      }
      return { opcode, count, entries };
    }
    case ServerOpcode.PlaceAck:
      return {
        opcode,
        ok: view.getUint8(1),
        nextAllowedMs: view.getBigInt64(2, true),
        errCode: view.getUint8(10),
      };
    case ServerOpcode.Score:
      return {
        opcode,
        black: view.getUint32(1, true),
        white: view.getUint32(5, true),
        empty: view.getUint32(9, true),
      };
    case ServerOpcode.Pong:
      return {
        opcode,
        nonce: view.getUint32(1, true),
        serverTimeMs: view.getBigInt64(5, true),
      };
    case ServerOpcode.Error:
      if (buffer.byteLength < 4) throw new Error("Error frame too short");
      {
        const msgLen = view.getUint16(2, true);
        if (buffer.byteLength !== 4 + msgLen) throw new Error("Error frame length mismatch");
        return {
          opcode,
          code: view.getUint8(1),
          message: new TextDecoder().decode(bytes.slice(4, 4 + msgLen)),
        };
      }
    default:
      throw new Error(`Unknown server opcode: ${opcode}`);
  }
}

function encodeU16Frame(opcode: number, value: number): ArrayBuffer {
  const out = new ArrayBuffer(3);
  const view = new DataView(out);
  view.setUint8(0, opcode);
  view.setUint16(1, value, true);
  return out;
}
