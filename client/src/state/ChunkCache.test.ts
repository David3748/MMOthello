import { describe, expect, it } from "vitest";
import { ChunkCache } from "./ChunkCache";
import {
  CHUNK_PACKED_BYTES,
  applyDelta,
  applySnapshot,
  getCellFromPacked,
  setCellInPacked,
} from "./chunk";

describe("chunk helpers", () => {
  it("applies snapshot and delta updates to packed chunk bytes", () => {
    const initial = new Uint8Array(CHUNK_PACKED_BYTES);
    setCellInPacked(initial, 2, 3, 1);

    const fromSnapshot = applySnapshot(initial);
    const updated = applyDelta(fromSnapshot, [
      { x: 2, y: 3, cell: 2 },
      { x: 10, y: 7, cell: 1 },
    ]);

    expect(getCellFromPacked(updated, 2, 3)).toBe(2);
    expect(getCellFromPacked(updated, 10, 7)).toBe(1);
  });
});

describe("ChunkCache", () => {
  it("evicts least recently used chunk", () => {
    const cache = new ChunkCache(2);
    const empty = new Uint8Array(CHUNK_PACKED_BYTES);

    cache.setSnapshot(1, 1n, empty);
    cache.setSnapshot(2, 1n, empty);
    cache.get(1);
    cache.setSnapshot(3, 1n, empty);

    expect(cache.get(1)).toBeDefined();
    expect(cache.get(2)).toBeUndefined();
    expect(cache.get(3)).toBeDefined();
  });

  it("applies delta to existing cache entry", () => {
    const cache = new ChunkCache(1);
    cache.setSnapshot(7, 1n, new Uint8Array(CHUNK_PACKED_BYTES));
    cache.applyDelta(7, 2n, [{ x: 4, y: 4, cell: 1 }]);

    const chunk = cache.get(7);
    expect(chunk).toBeDefined();
    expect(chunk?.version).toBe(2n);
    expect(getCellFromPacked(chunk!.packed, 4, 4)).toBe(1);
  });
});
