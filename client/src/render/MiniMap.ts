import type { ChunkCache } from "../state/ChunkCache";
import type { Viewport } from "./Viewport";
import {
  BOARD_CHUNKS_PER_ROW,
  CHUNK_SIZE,
  getCellFromPacked,
} from "../state/chunk";

// MiniMap renders a small overview canvas of the whole 1000×1000 board,
// downsampled by averaging 50×50 cells per chunk into a single (B/W ratio)
// pixel. Re-uses the existing ChunkCache so it shows whatever the player
// has already loaded; missing chunks are dark.
export class MiniMap {
  readonly element: HTMLCanvasElement;
  private readonly cache: ChunkCache;
  private readonly viewport: Viewport;
  private rafId: number | null = null;

  constructor(cache: ChunkCache, viewport: Viewport) {
    this.cache = cache;
    this.viewport = viewport;
    this.element = document.createElement("canvas");
    this.element.className = "minimap";
    this.element.width = BOARD_CHUNKS_PER_ROW;
    this.element.height = BOARD_CHUNKS_PER_ROW;
  }

  start(): void {
    if (this.rafId !== null) return;
    const tick = () => {
      this.draw();
      this.rafId = requestAnimationFrame(tick);
    };
    tick();
  }

  stop(): void {
    if (this.rafId !== null) {
      cancelAnimationFrame(this.rafId);
      this.rafId = null;
    }
  }

  private draw(): void {
    const ctx = this.element.getContext("2d");
    if (!ctx) return;
    ctx.fillStyle = "#2f4034";
    ctx.fillRect(0, 0, this.element.width, this.element.height);
    const totalCellsPerChunk = CHUNK_SIZE * CHUNK_SIZE;

    for (const cached of this.cache.snapshot()) {
      const cx = cached.chunkId % BOARD_CHUNKS_PER_ROW;
      const cy = Math.floor(cached.chunkId / BOARD_CHUNKS_PER_ROW);
      let black = 0;
      let white = 0;
      // Sample every 5th cell for speed (5×5 grid of samples per chunk).
      for (let y = 0; y < CHUNK_SIZE; y += 5) {
        for (let x = 0; x < CHUNK_SIZE; x += 5) {
          const c = getCellFromPacked(cached.packed, x, y);
          if (c === 1) black++;
          else if (c === 2) white++;
        }
      }
      const samples = Math.ceil(CHUNK_SIZE / 5) * Math.ceil(CHUNK_SIZE / 5);
      const fill = pickFill(black, white, samples, totalCellsPerChunk);
      ctx.fillStyle = fill;
      ctx.fillRect(cx, cy, 1, 1);
    }

    // Outline the current viewport.
    const x = (this.viewport.x / 1000) * this.element.width;
    const y = (this.viewport.y / 1000) * this.element.height;
    const wCells = this.viewport.widthPx / this.viewport.zoomPxPerCell;
    const hCells = this.viewport.heightPx / this.viewport.zoomPxPerCell;
    const w = (wCells / 1000) * this.element.width;
    const h = (hCells / 1000) * this.element.height;
    ctx.strokeStyle = "#38bdf8";
    ctx.lineWidth = 0.5;
    ctx.strokeRect(x, y, w, h);
  }
}

function pickFill(black: number, white: number, _samples: number, _total: number): string {
  if (black === 0 && white === 0) return "#3b5140";
  if (black > white) return "#111513";
  if (white > black) return "#f4f1e8";
  return "#9aa893";
}
