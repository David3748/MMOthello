import "./styles.css";
import { WebSocketClient } from "./net/WebSocketClient";
import { Canvas2DRenderer } from "./render/Canvas2DRenderer";
import { MiniMap } from "./render/MiniMap";
import { Viewport } from "./render/Viewport";
import { ChunkCache } from "./state/ChunkCache";
import {
  CHUNK_SIZE,
  BOARD_CHUNKS_PER_ROW,
  type Cell,
  type DeltaEntry,
} from "./state/chunk";
import { CooldownPlaceholder } from "./ui/CooldownPlaceholder";
import { TopBarPlaceholder } from "./ui/TopBarPlaceholder";
import type { DecodedServerFrame } from "./net/protocol";

const SESSION_URL = "/session";
const WS_URL = location.protocol === "https:" ? `wss://${location.host}/ws` : `ws://${location.host}/ws`;

export async function bootstrapApp(root: HTMLDivElement | null): Promise<void> {
  if (!root) throw new Error("Missing #app mount point.");

  root.innerHTML = "";
  const app = document.createElement("div");
  app.className = "app-shell";

  const topBar = new TopBarPlaceholder();
  app.append(topBar.element);

  const viewportContainer = document.createElement("div");
  viewportContainer.className = "viewport-container";
  const canvas = document.createElement("canvas");
  canvas.className = "board-canvas";
  viewportContainer.append(canvas);
  const zoomHud = document.createElement("div");
  zoomHud.className = "zoom-hud";
  viewportContainer.append(zoomHud);

  const cooldown = new CooldownPlaceholder();
  viewportContainer.append(cooldown.element);

  const toast = document.createElement("div");
  toast.className = "toast";
  viewportContainer.append(toast);

  app.append(viewportContainer);
  root.append(app);

  // Bootstrap a session (sets cookie). If the request fails (e.g. served
  // statically without a backend), fall through and let the WS upgrade fail.
  try {
    await fetch(SESSION_URL, { credentials: "same-origin" });
  } catch {
    // best effort
  }

  const cache = new ChunkCache(400);
  const viewport = new Viewport({
    worldWidthCells: 1000,
    worldHeightCells: 1000,
    widthPx: canvas.clientWidth || 960,
    heightPx: canvas.clientHeight || 640,
    zoomPxPerCell: 8,
  });
  // Center initially.
  viewport.panBy(500 - viewport.widthPx / viewport.zoomPxPerCell / 2, 500 - viewport.heightPx / viewport.zoomPxPerCell / 2);

  const renderer = new Canvas2DRenderer({
    canvas,
    viewport,
    chunkCache: cache,
    chunkSize: CHUNK_SIZE,
  });
  renderer.start();
  updateZoomHud();

  const miniMap = new MiniMap(cache, viewport);
  viewportContainer.append(miniMap.element);
  miniMap.start();
  miniMap.element.addEventListener("click", (e) => {
    const rect = miniMap.element.getBoundingClientRect();
    const fx = (e.clientX - rect.left) / rect.width;
    const fy = (e.clientY - rect.top) / rect.height;
    const targetX = fx * 1000 - viewport.widthPx / viewport.zoomPxPerCell / 2;
    const targetY = fy * 1000 - viewport.heightPx / viewport.zoomPxPerCell / 2;
    viewport.panBy(targetX - viewport.x, targetY - viewport.y);
    refreshSubscriptions();
  });

  let team: Cell = 1;
  let nextAllowedMs = 0;
  let serverOffsetMs = 0; // server - client

  const wsClient = new WebSocketClient({
    url: WS_URL,
    onOpen: () => topBar.setConnectionState("connected"),
    onClose: () => topBar.setConnectionState("reconnecting"),
    onFrame: (frame) => handleFrame(frame),
  });

  wsClient.connect();

  // Pan/zoom controls.
  let dragging = false;
  let movedWhileDragging = false;
  let lastX = 0, lastY = 0;
  let activePointerId: number | null = null;
  canvas.addEventListener("pointerdown", (e) => {
    if (e.button !== 0) return;
    activePointerId = e.pointerId;
    canvas.setPointerCapture(e.pointerId);
    dragging = true;
    movedWhileDragging = false;
    lastX = e.clientX; lastY = e.clientY;
    viewportContainer.classList.add("is-dragging");
  });
  canvas.addEventListener("pointerup", (e) => {
    if (activePointerId === e.pointerId) activePointerId = null;
    canvas.releasePointerCapture(e.pointerId);
    dragging = false;
    viewportContainer.classList.remove("is-dragging");
  });
  canvas.addEventListener("pointercancel", () => {
    activePointerId = null;
    dragging = false;
    viewportContainer.classList.remove("is-dragging");
  });
  canvas.addEventListener("lostpointercapture", () => {
    activePointerId = null;
    dragging = false;
    viewportContainer.classList.remove("is-dragging");
  });
  canvas.addEventListener("pointermove", (e) => {
    if (!dragging || activePointerId !== e.pointerId) return;
    const dx = (e.clientX - lastX) / viewport.zoomPxPerCell;
    const dy = (e.clientY - lastY) / viewport.zoomPxPerCell;
    if (Math.abs(dx) > 0.1 || Math.abs(dy) > 0.1) movedWhileDragging = true;
    viewport.panBy(-dx, -dy);
    lastX = e.clientX; lastY = e.clientY;
    refreshSubscriptions();
  });
  canvas.addEventListener("wheel", (e) => {
    e.preventDefault();
    const rect = canvas.getBoundingClientRect();
    const localX = e.clientX - rect.left;
    const localY = e.clientY - rect.top;
    const anchorX = viewport.x + localX / viewport.zoomPxPerCell;
    const anchorY = viewport.y + localY / viewport.zoomPxPerCell;
    const wheel = Math.sign(e.deltaY) || 1;
    const step = e.ctrlKey ? 1.05 : 1.18;
    const factor = wheel < 0 ? step : 1 / step;
    viewport.setZoom(viewport.zoomPxPerCell * factor);
    viewport.panBy(anchorX - (viewport.x + localX / viewport.zoomPxPerCell), anchorY - (viewport.y + localY / viewport.zoomPxPerCell));
    updateZoomHud();
    refreshSubscriptions();
  }, { passive: false });

  canvas.addEventListener("click", (e) => {
    if (movedWhileDragging) return;
    const rect = canvas.getBoundingClientRect();
    const cx = (e.clientX - rect.left) / viewport.zoomPxPerCell + viewport.x;
    const cy = (e.clientY - rect.top) / viewport.zoomPxPerCell + viewport.y;
    const x = Math.floor(cx);
    const y = Math.floor(cy);
    if (x < 0 || x >= 1000 || y < 0 || y >= 1000) return;
    wsClient.place(x, y);
  });
  window.addEventListener("keydown", (e) => {
    if (e.repeat) return;
    const panStep = Math.max(8, 96 / viewport.zoomPxPerCell);
    if (e.key === "ArrowUp") viewport.panBy(0, -panStep);
    else if (e.key === "ArrowDown") viewport.panBy(0, panStep);
    else if (e.key === "ArrowLeft") viewport.panBy(-panStep, 0);
    else if (e.key === "ArrowRight") viewport.panBy(panStep, 0);
    else if (e.key === "+" || e.key === "=") {
      zoomAroundCanvasPoint(1.15, canvas.clientWidth / 2, canvas.clientHeight / 2);
    } else if (e.key === "-" || e.key === "_") {
      zoomAroundCanvasPoint(1 / 1.15, canvas.clientWidth / 2, canvas.clientHeight / 2);
    } else if (e.key === "0") {
      viewport.setZoom(8);
      viewport.panBy(500 - viewport.widthPx / viewport.zoomPxPerCell / 2 - viewport.x, 500 - viewport.heightPx / viewport.zoomPxPerCell / 2 - viewport.y);
      updateZoomHud();
      refreshSubscriptions();
      return;
    } else return;
    updateZoomHud();
    refreshSubscriptions();
  });

  // Drive cooldown chip every animation frame.
  function tickCooldown() {
    const now = Date.now() + serverOffsetMs;
    const remaining = Math.max(0, nextAllowedMs - now);
    if (remaining <= 0) {
      cooldown.element.className = "cooldown-chip ready";
      cooldown.element.textContent = "Ready";
    } else {
      cooldown.element.className = "cooldown-chip waiting";
      cooldown.element.textContent = `Cooldown: ${(remaining / 1000).toFixed(1)}s`;
    }
    requestAnimationFrame(tickCooldown);
  }
  requestAnimationFrame(tickCooldown);

  function refreshSubscriptions() {
    const ids = viewport.getVisibleChunkIds(CHUNK_SIZE);
    wsClient.setSubscriptions(ids);
  }

  function zoomAroundCanvasPoint(scale: number, canvasX: number, canvasY: number) {
    const anchorX = viewport.x + canvasX / viewport.zoomPxPerCell;
    const anchorY = viewport.y + canvasY / viewport.zoomPxPerCell;
    viewport.setZoom(viewport.zoomPxPerCell * scale);
    viewport.panBy(anchorX - (viewport.x + canvasX / viewport.zoomPxPerCell), anchorY - (viewport.y + canvasY / viewport.zoomPxPerCell));
  }

  function updateZoomHud() {
    zoomHud.textContent = `Zoom ${(viewport.zoomPxPerCell).toFixed(1)}x  |  Drag to pan  |  Wheel or +/- to zoom`;
  }

  function showToast(msg: string) {
    toast.textContent = msg;
    toast.classList.add("visible");
    setTimeout(() => toast.classList.remove("visible"), 1500);
  }

  function handleFrame(frame: DecodedServerFrame) {
    switch (frame.opcode) {
      case 0x81: { // Welcome
        team = (frame.team || 1) as Cell;
        topBar.setConnectionState("connected");
        const teamSpan = topBar.element.querySelectorAll("span")[1];
        if (teamSpan) teamSpan.textContent = `Team: ${team === 1 ? "Black" : "White"}`;
        serverOffsetMs = Number(frame.serverTimeMs) - Date.now();
        refreshSubscriptions();
        break;
      }
      case 0x82: // Snapshot
        cache.setSnapshot(frame.chunkId, frame.version, frame.packed);
        break;
      case 0x83: { // Delta
        // Group entries by chunk and apply.
        const byChunk = new Map<number, DeltaEntry[]>();
        for (const e of frame.entries) {
          const cx = Math.floor(e.x / CHUNK_SIZE);
          const cy = Math.floor(e.y / CHUNK_SIZE);
          const id = cy * BOARD_CHUNKS_PER_ROW + cx;
          const local: DeltaEntry = { x: e.x - cx * CHUNK_SIZE, y: e.y - cy * CHUNK_SIZE, cell: e.cell };
          const arr = byChunk.get(id);
          if (arr) arr.push(local); else byChunk.set(id, [local]);
        }
        for (const [id, deltas] of byChunk) {
          const existing = cache.get(id);
          if (existing) cache.applyDelta(id, existing.version + 1n, deltas);
        }
        break;
      }
      case 0x84: { // PlaceAck
        nextAllowedMs = Number(frame.nextAllowedMs);
        if (frame.ok === 0) {
          showToast(errorText(frame.errCode));
        }
        break;
      }
      case 0x85: { // Score
        const total = frame.black + frame.white + frame.empty;
        const bp = ((frame.black / total) * 100).toFixed(1);
        const wp = ((frame.white / total) * 100).toFixed(1);
        const scoreSpan = topBar.element.querySelectorAll("span")[2];
        if (scoreSpan) scoreSpan.textContent = `Score B/W: ${bp}% / ${wp}%`;
        break;
      }
      case 0x86: // Pong
        break;
      case 0x87:
        showToast(frame.message || `error ${frame.code}`);
        break;
    }
  }

  // Re-subscribe whenever the canvas resizes.
  const ro = new ResizeObserver(() => {
    updateZoomHud();
    refreshSubscriptions();
  });
  ro.observe(canvas);
}

function errorText(code: number): string {
  switch (code) {
    case 1: return "On cooldown";
    case 2: return "Cell occupied";
    case 3: return "No flips — pick a frontier";
    case 4: return "Out of bounds";
    case 5: return "Not authenticated";
    case 6: return "Rate limited";
    default: return `Error ${code}`;
  }
}
