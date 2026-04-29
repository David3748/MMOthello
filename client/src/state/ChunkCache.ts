import { applyDelta, applySnapshot, type DeltaEntry } from "./chunk";

export type CachedChunk = {
  chunkId: number;
  version: bigint;
  packed: Uint8Array;
  dirty: boolean;
};

export class ChunkCache {
  private readonly capacity: number;
  private readonly entries = new Map<number, CachedChunk>();

  constructor(capacity: number) {
    if (capacity < 1) throw new Error("capacity must be > 0");
    this.capacity = capacity;
  }

  get size(): number {
    return this.entries.size;
  }

  get(chunkId: number): CachedChunk | undefined {
    const item = this.entries.get(chunkId);
    if (!item) return undefined;
    this.touch(chunkId, item);
    return item;
  }

  setSnapshot(chunkId: number, version: bigint, packedBytes: Uint8Array): CachedChunk {
    const chunk: CachedChunk = {
      chunkId,
      version,
      packed: applySnapshot(packedBytes),
      dirty: true,
    };
    this.entries.set(chunkId, chunk);
    this.touch(chunkId, chunk);
    this.trim();
    return chunk;
  }

  applyDelta(chunkId: number, version: bigint, deltas: DeltaEntry[]): CachedChunk | undefined {
    const existing = this.entries.get(chunkId);
    if (!existing) return undefined;
    const next: CachedChunk = {
      ...existing,
      version,
      packed: applyDelta(existing.packed, deltas),
      dirty: true,
    };
    this.entries.set(chunkId, next);
    this.touch(chunkId, next);
    return next;
  }

  snapshot(): CachedChunk[] {
    return Array.from(this.entries.values());
  }

  private touch(chunkId: number, chunk: CachedChunk): void {
    this.entries.delete(chunkId);
    this.entries.set(chunkId, chunk);
  }

  private trim(): void {
    while (this.entries.size > this.capacity) {
      const first = this.entries.keys().next().value;
      if (first === undefined) return;
      this.entries.delete(first);
    }
  }
}
