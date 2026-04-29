import { chunkIdFromChunkCoord } from "../state/chunk";

const MIN_ZOOM = 0.25;
const MAX_ZOOM = 16;

type ViewportInit = {
  worldWidthCells: number;
  worldHeightCells: number;
  widthPx: number;
  heightPx: number;
  zoomPxPerCell: number;
};

export class Viewport {
  x = 0;
  y = 0;
  zoomPxPerCell: number;
  readonly worldWidthCells: number;
  readonly worldHeightCells: number;
  widthPx: number;
  heightPx: number;

  constructor(init: ViewportInit) {
    this.worldWidthCells = init.worldWidthCells;
    this.worldHeightCells = init.worldHeightCells;
    this.widthPx = init.widthPx;
    this.heightPx = init.heightPx;
    this.zoomPxPerCell = this.clampZoom(init.zoomPxPerCell);
    this.clampToWorld();
  }

  setSize(widthPx: number, heightPx: number): void {
    this.widthPx = widthPx;
    this.heightPx = heightPx;
    this.zoomPxPerCell = this.clampZoom(this.zoomPxPerCell);
    this.clampToWorld();
  }

  panBy(dxCells: number, dyCells: number): void {
    this.x += dxCells;
    this.y += dyCells;
    this.clampToWorld();
  }

  setZoom(zoomPxPerCell: number): void {
    this.zoomPxPerCell = this.clampZoom(zoomPxPerCell);
    this.clampToWorld();
  }

  getVisibleChunkIds(chunkSize: number): number[] {
    const startX = Math.max(0, Math.floor(this.x / chunkSize));
    const startY = Math.max(0, Math.floor(this.y / chunkSize));
    const endX = Math.min(
      Math.ceil((this.x + this.widthPx / this.zoomPxPerCell) / chunkSize),
      Math.ceil(this.worldWidthCells / chunkSize),
    );
    const endY = Math.min(
      Math.ceil((this.y + this.heightPx / this.zoomPxPerCell) / chunkSize),
      Math.ceil(this.worldHeightCells / chunkSize),
    );

    const ids: number[] = [];
    for (let cy = startY; cy < endY; cy += 1) {
      for (let cx = startX; cx < endX; cx += 1) {
        ids.push(chunkIdFromChunkCoord(cx, cy));
      }
    }
    return ids;
  }

  private clampToWorld(): void {
    const maxX = Math.max(0, this.worldWidthCells - this.widthPx / this.zoomPxPerCell);
    const maxY = Math.max(0, this.worldHeightCells - this.heightPx / this.zoomPxPerCell);
    this.x = clamp(this.x, 0, maxX);
    this.y = clamp(this.y, 0, maxY);
  }

  private clampZoom(value: number): number {
    const fitZoomX = this.widthPx / this.worldWidthCells;
    const fitZoomY = this.heightPx / this.worldHeightCells;
    const dynamicMinZoom = Math.max(MIN_ZOOM, fitZoomX, fitZoomY);
    return clamp(value, dynamicMinZoom, MAX_ZOOM);
  }
}

function clamp(value: number, min: number, max: number): number {
  return Math.min(max, Math.max(min, value));
}

