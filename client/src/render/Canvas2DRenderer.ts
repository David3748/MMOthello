import type { Viewport } from "./Viewport";
import type { ChunkCache } from "../state/ChunkCache";
import { CHUNK_SIZE, getCellFromPacked } from "../state/chunk";

type Canvas2DRendererParams = {
  canvas: HTMLCanvasElement;
  viewport: Viewport;
  chunkCache: ChunkCache;
  chunkSize?: number;
};

export class Canvas2DRenderer {
  private readonly canvas: HTMLCanvasElement;
  private readonly viewport: Viewport;
  private readonly cache: ChunkCache;
  private readonly chunkSize: number;
  private rafId: number | null = null;

  constructor(params: Canvas2DRendererParams) {
    this.canvas = params.canvas;
    this.viewport = params.viewport;
    this.cache = params.chunkCache;
    this.chunkSize = params.chunkSize ?? CHUNK_SIZE;
  }

  start(): void {
    if (this.rafId !== null) return;
    const tick = () => {
      this.drawFrame();
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

  private drawFrame(): void {
    const ctx = this.canvas.getContext("2d");
    if (!ctx) return;

    this.resizeCanvasIfNeeded();
    ctx.clearRect(0, 0, this.canvas.width, this.canvas.height);
    const gradient = ctx.createLinearGradient(0, 0, 0, this.canvas.height);
    gradient.addColorStop(0, "#0a1a11");
    gradient.addColorStop(1, "#07110c");
    ctx.fillStyle = gradient;
    ctx.fillRect(0, 0, this.canvas.width, this.canvas.height);
    drawGrid(ctx, this.viewport, this.canvas.width, this.canvas.height);

    const zoom = this.viewport.zoomPxPerCell;
    const visibleChunks = this.viewport.getVisibleChunkIds(this.chunkSize);
    for (const chunkId of visibleChunks) {
      const chunk = this.cache.get(chunkId);
      if (!chunk) continue;
      this.drawChunk(ctx, chunkId, chunk.packed, zoom);
    }
  }

  private drawChunk(
    ctx: CanvasRenderingContext2D,
    chunkId: number,
    packed: Uint8Array,
    zoomPxPerCell: number,
  ): void {
    const chunksPerRow = Math.ceil(1000 / this.chunkSize);
    const chunkX = chunkId % chunksPerRow;
    const chunkY = Math.floor(chunkId / chunksPerRow);
    const startX = (chunkX * this.chunkSize - this.viewport.x) * zoomPxPerCell;
    const startY = (chunkY * this.chunkSize - this.viewport.y) * zoomPxPerCell;

    for (let y = 0; y < this.chunkSize; y += 1) {
      for (let x = 0; x < this.chunkSize; x += 1) {
        const cell = getCellFromPacked(packed, x, y);
        if (cell === 0) continue;
        const px = startX + x * zoomPxPerCell;
        const py = startY + y * zoomPxPerCell;
        if (zoomPxPerCell >= 4) {
          drawStone(ctx, px, py, zoomPxPerCell, cell);
        } else {
          ctx.fillStyle = cell === 1 ? "#0b0b0b" : "#f8fafc";
          ctx.fillRect(px, py, Math.max(1, zoomPxPerCell), Math.max(1, zoomPxPerCell));
        }
      }
    }
  }

  private resizeCanvasIfNeeded(): void {
    const width = this.canvas.clientWidth || this.canvas.width;
    const height = this.canvas.clientHeight || this.canvas.height;
    if (this.canvas.width !== width || this.canvas.height !== height) {
      this.canvas.width = width;
      this.canvas.height = height;
      this.viewport.setSize(width, height);
    }
  }
}

function drawStone(
  ctx: CanvasRenderingContext2D,
  x: number,
  y: number,
  zoomPxPerCell: number,
  cell: number,
): void {
  const radius = zoomPxPerCell * 0.45;
  ctx.save();
  ctx.beginPath();
  ctx.arc(x + zoomPxPerCell / 2, y + zoomPxPerCell / 2, radius, 0, Math.PI * 2);
  ctx.fillStyle = cell === 1 ? "#0a0a0a" : "#f8fafc";
  ctx.shadowBlur = 4;
  ctx.shadowColor = "rgba(0,0,0,0.45)";
  ctx.fill();
  ctx.restore();
}

function drawGrid(
  ctx: CanvasRenderingContext2D,
  viewport: Viewport,
  width: number,
  height: number,
): void {
  const zoom = viewport.zoomPxPerCell;
  if (zoom < 6) return;
  const step = zoom;
  const offsetX = -((viewport.x * zoom) % step);
  const offsetY = -((viewport.y * zoom) % step);

  ctx.save();
  ctx.beginPath();
  for (let x = offsetX; x < width; x += step) {
    ctx.moveTo(Math.round(x) + 0.5, 0);
    ctx.lineTo(Math.round(x) + 0.5, height);
  }
  for (let y = offsetY; y < height; y += step) {
    ctx.moveTo(0, Math.round(y) + 0.5);
    ctx.lineTo(width, Math.round(y) + 0.5);
  }
  ctx.strokeStyle = "rgba(148, 163, 184, 0.18)";
  ctx.lineWidth = 1;
  ctx.stroke();
  ctx.restore();
}
