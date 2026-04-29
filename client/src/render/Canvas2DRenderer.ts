import type { Viewport } from "./Viewport";
import type { CachedChunk, ChunkCache } from "../state/ChunkCache";
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
  private readonly rasterCache = new Map<number, { version: bigint; canvas: HTMLCanvasElement }>();
  private rafId: number | null = null;
  private hoverCell: { x: number; y: number; cell: number } | null = null;
  private pendingStone: { x: number; y: number; cell: number } | null = null;

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

  setHoverCell(cell: { x: number; y: number; cell: number } | null): void {
    this.hoverCell = cell;
  }

  setPendingStone(cell: { x: number; y: number; cell: number } | null): void {
    this.pendingStone = cell;
  }

  private drawFrame(): void {
    const ctx = this.canvas.getContext("2d");
    if (!ctx) return;

    this.resizeCanvasIfNeeded();
    ctx.clearRect(0, 0, this.canvas.width, this.canvas.height);
    const gradient = ctx.createLinearGradient(0, 0, 0, this.canvas.height);
    gradient.addColorStop(0, "#4f6f52");
    gradient.addColorStop(1, "#46674b");
    ctx.fillStyle = gradient;
    ctx.fillRect(0, 0, this.canvas.width, this.canvas.height);
    drawGrid(ctx, this.viewport, this.canvas.width, this.canvas.height);

    const visibleChunks = this.viewport.getVisibleChunkIds(this.chunkSize);
    for (const chunkId of visibleChunks) {
      const chunk = this.cache.get(chunkId);
      if (!chunk) continue;
      this.drawChunk(ctx, chunk);
    }
    this.drawOverlayStone(ctx, this.pendingStone, 0.9);
    this.drawOverlayStone(ctx, this.hoverCell, 0.45);
  }

  private drawChunk(ctx: CanvasRenderingContext2D, chunk: CachedChunk): void {
    const zoomPxPerCell = this.viewport.zoomPxPerCell;
    const chunksPerRow = Math.ceil(1000 / this.chunkSize);
    const chunkX = chunk.chunkId % chunksPerRow;
    const chunkY = Math.floor(chunk.chunkId / chunksPerRow);
    const startX = (chunkX * this.chunkSize - this.viewport.x) * zoomPxPerCell;
    const startY = (chunkY * this.chunkSize - this.viewport.y) * zoomPxPerCell;

    if (zoomPxPerCell < 4) {
      const raster = this.getRasterizedChunk(chunk);
      ctx.imageSmoothingEnabled = false;
      ctx.drawImage(raster, startX, startY, this.chunkSize * zoomPxPerCell, this.chunkSize * zoomPxPerCell);
      return;
    }

    for (let y = 0; y < this.chunkSize; y += 1) {
      for (let x = 0; x < this.chunkSize; x += 1) {
        const cell = getCellFromPacked(chunk.packed, x, y);
        if (cell === 0) continue;
        const px = startX + x * zoomPxPerCell;
        const py = startY + y * zoomPxPerCell;
        if (zoomPxPerCell >= 4) {
          drawStone(ctx, px, py, zoomPxPerCell, cell);
        } else {
          ctx.fillStyle = cell === 1 ? "#05070a" : "#f4f1e8";
          ctx.fillRect(px, py, Math.max(1, zoomPxPerCell), Math.max(1, zoomPxPerCell));
        }
      }
    }
  }

  private getRasterizedChunk(chunk: CachedChunk): HTMLCanvasElement {
    const cached = this.rasterCache.get(chunk.chunkId);
    if (cached && cached.version === chunk.version) return cached.canvas;

    const canvas = document.createElement("canvas");
    canvas.width = this.chunkSize;
    canvas.height = this.chunkSize;
    const ctx = canvas.getContext("2d");
    if (ctx) {
      const img = ctx.createImageData(this.chunkSize, this.chunkSize);
      for (let y = 0; y < this.chunkSize; y += 1) {
        for (let x = 0; x < this.chunkSize; x += 1) {
          const cell = getCellFromPacked(chunk.packed, x, y);
          if (cell === 0) continue;
          const i = (y * this.chunkSize + x) * 4;
          if (cell === 1) {
            img.data[i] = 5; img.data[i + 1] = 7; img.data[i + 2] = 10; img.data[i + 3] = 255;
          } else {
            img.data[i] = 244; img.data[i + 1] = 241; img.data[i + 2] = 232; img.data[i + 3] = 255;
          }
        }
      }
      ctx.putImageData(img, 0, 0);
    }
    this.rasterCache.set(chunk.chunkId, { version: chunk.version, canvas });
    if (this.rasterCache.size > 450) {
      const first = this.rasterCache.keys().next().value;
      if (first !== undefined) this.rasterCache.delete(first);
    }
    return canvas;
  }

  private drawOverlayStone(
    ctx: CanvasRenderingContext2D,
    stone: { x: number; y: number; cell: number } | null,
    alpha: number,
  ): void {
    if (!stone) return;
    const zoom = this.viewport.zoomPxPerCell;
    const px = (stone.x - this.viewport.x) * zoom;
    const py = (stone.y - this.viewport.y) * zoom;
    if (px + zoom < 0 || py + zoom < 0 || px > this.canvas.width || py > this.canvas.height) return;
    ctx.save();
    ctx.globalAlpha = alpha;
    drawStone(ctx, px, py, zoom, stone.cell);
    ctx.restore();
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
  ctx.fillStyle = cell === 1 ? "#05070a" : "#f4f1e8";
  ctx.shadowBlur = cell === 1 ? 4 : 3;
  ctx.shadowColor = cell === 1 ? "rgba(20, 24, 20, 0.42)" : "rgba(0,0,0,0.32)";
  ctx.fill();
  ctx.lineWidth = Math.max(1, zoomPxPerCell * 0.08);
  ctx.strokeStyle = cell === 1 ? "rgba(214, 222, 207, 0.45)" : "rgba(32, 40, 34, 0.32)";
  ctx.stroke();
  if (cell === 1 && zoomPxPerCell >= 6) {
    ctx.beginPath();
    ctx.arc(
      x + zoomPxPerCell * 0.38,
      y + zoomPxPerCell * 0.34,
      Math.max(1, zoomPxPerCell * 0.1),
      0,
      Math.PI * 2,
    );
    ctx.fillStyle = "rgba(244, 241, 232, 0.22)";
    ctx.fill();
  }
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
  ctx.strokeStyle = "rgba(222, 235, 214, 0.18)";
  ctx.lineWidth = 1;
  ctx.stroke();
  ctx.restore();
}
