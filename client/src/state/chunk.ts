export const CHUNK_SIZE = 50;
export const CHUNK_CELL_COUNT = CHUNK_SIZE * CHUNK_SIZE;
export const CHUNK_PACKED_BYTES = CHUNK_CELL_COUNT / 4;
export const BOARD_CHUNKS_PER_ROW = 20;

export type Cell = 0 | 1 | 2;

export type DeltaEntry = {
  x: number;
  y: number;
  cell: Cell;
};

export function chunkIdFromChunkCoord(chunkX: number, chunkY: number): number {
  return chunkY * BOARD_CHUNKS_PER_ROW + chunkX;
}

export function setCellInPacked(packed: Uint8Array, x: number, y: number, cell: Cell): void {
  const i = y * CHUNK_SIZE + x;
  const byteIndex = Math.floor(i / 4);
  const shift = (3 - (i % 4)) * 2;
  const current = packed[byteIndex];
  if (current === undefined) throw new Error("cell index out of range");
  const mask = ~(0b11 << shift) & 0xff;
  packed[byteIndex] = (current & mask) | ((cell & 0b11) << shift);
}

export function getCellFromPacked(packed: Uint8Array, x: number, y: number): Cell {
  const i = y * CHUNK_SIZE + x;
  const byteIndex = Math.floor(i / 4);
  const shift = (3 - (i % 4)) * 2;
  const current = packed[byteIndex];
  if (current === undefined) throw new Error("cell index out of range");
  return ((current >> shift) & 0b11) as Cell;
}

export function applySnapshot(packedBytes: Uint8Array): Uint8Array {
  if (packedBytes.length !== CHUNK_PACKED_BYTES) {
    throw new Error(`snapshot packed length must be ${CHUNK_PACKED_BYTES}`);
  }
  return new Uint8Array(packedBytes);
}

export function applyDelta(chunkPacked: Uint8Array, deltas: DeltaEntry[]): Uint8Array {
  const next = new Uint8Array(chunkPacked);
  for (const delta of deltas) {
    setCellInPacked(next, delta.x, delta.y, delta.cell);
  }
  return next;
}
