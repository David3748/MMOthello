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

type DemoState = {
  cacheSize: number;
  connectionState: "connecting" | "connected" | "reconnecting";
  helpOpen: boolean;
  lastAckCode: number | null;
  lastToast: string;
  pendingPlacement: boolean;
  scoreSeen: boolean;
  snapshotCount: number;
  team: Cell | null;
};

export async function bootstrapApp(root: HTMLDivElement | null): Promise<void> {
  if (!root) throw new Error("Missing #app mount point.");

  root.innerHTML = "";
  const app = document.createElement("div");
  app.className = "app-shell";

  const topBar = new TopBarPlaceholder();
  app.append(topBar.element);
  const helpButton = document.createElement("button");
  helpButton.className = "icon-button";
  helpButton.type = "button";
  helpButton.textContent = "?";
  helpButton.title = "Rules and controls";
  topBar.element.append(helpButton);

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

  const activity = document.createElement("div");
  activity.className = "activity-chip";
  activity.textContent = "Connecting...";
  viewportContainer.append(activity);

  const toast = document.createElement("div");
  toast.className = "toast";
  viewportContainer.append(toast);
  const helpPanel = document.createElement("div");
  helpPanel.className = "help-panel";
  helpPanel.hidden = true;
  helpPanel.innerHTML = `
    <h2>MMOthello</h2>
    <p>Bracket an opponent line with your color. Valid moves flip at least one stone.</p>
    <p>Drag or use arrows to pan. Wheel, pinch, +, -, or 0 to zoom. Tap or click when the cooldown says ready.</p>
  `;
  viewportContainer.append(helpPanel);
  helpButton.addEventListener("click", () => {
    helpPanel.hidden = !helpPanel.hidden;
    demoState.helpOpen = !helpPanel.hidden;
  });

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
  let lastPlace: { x: number; y: number; cell: Cell } | null = null;
  const pendingPings = new Map<number, number>();
  const demoState: DemoState = {
    cacheSize: 0,
    connectionState: "connecting",
    helpOpen: false,
    lastAckCode: null,
    lastToast: "",
    pendingPlacement: false,
    scoreSeen: false,
    snapshotCount: 0,
    team: null,
  };
  (window as Window & { __mmothelloDemo?: DemoState }).__mmothelloDemo = demoState;

  const wsClient = new WebSocketClient({
    url: WS_URL,
    onOpen: () => {
      setConnectionState("connected");
      setActivity("Connected. Loading nearby board...");
    },
    onClose: () => {
      setConnectionState("reconnecting");
      setActivity("Reconnecting...");
    },
    onFrame: (frame) => handleFrame(frame),
  });

  wsClient.connect();
  window.setInterval(() => {
    const nonce = Math.floor(Math.random() * 0xffffffff);
    pendingPings.set(nonce, performance.now());
    wsClient.ping(nonce);
  }, 3000);

  // Pan/zoom controls.
  let dragging = false;
  let movedWhileDragging = false;
  let lastX = 0, lastY = 0;
  let activePointerId: number | null = null;
  const pointers = new Map<number, PointerEvent>();
  let pinchDistance = 0;
  let pinchZoom = viewport.zoomPxPerCell;
  canvas.addEventListener("pointerdown", (e) => {
    if (e.button !== 0) return;
    pointers.set(e.pointerId, e);
    activePointerId = e.pointerId;
    canvas.setPointerCapture(e.pointerId);
    dragging = true;
    movedWhileDragging = false;
    lastX = e.clientX; lastY = e.clientY;
    viewportContainer.classList.add("is-dragging");
    if (pointers.size === 2) {
      const pair = Array.from(pointers.values());
      const a = pair[0];
      const b = pair[1];
      if (!a || !b) return;
      pinchDistance = pointerDistance(a, b);
      pinchZoom = viewport.zoomPxPerCell;
    }
  });
  canvas.addEventListener("pointerup", (e) => {
    pointers.delete(e.pointerId);
    if (activePointerId === e.pointerId) activePointerId = null;
    canvas.releasePointerCapture(e.pointerId);
    dragging = false;
    viewportContainer.classList.remove("is-dragging");
  });
  canvas.addEventListener("pointercancel", (e) => {
    pointers.delete(e.pointerId);
    activePointerId = null;
    dragging = false;
    viewportContainer.classList.remove("is-dragging");
  });
  canvas.addEventListener("lostpointercapture", () => {
    pointers.clear();
    activePointerId = null;
    dragging = false;
    viewportContainer.classList.remove("is-dragging");
  });
  canvas.addEventListener("pointermove", (e) => {
    pointers.set(e.pointerId, e);
    updateHover(e);
    if (pointers.size === 2) {
      const pair = Array.from(pointers.values());
      const a = pair[0];
      const b = pair[1];
      if (!a || !b) return;
      const nextDistance = pointerDistance(a, b);
      if (pinchDistance > 0 && nextDistance > 0) {
        const rect = canvas.getBoundingClientRect();
        const midX = ((a.clientX + b.clientX) / 2) - rect.left;
        const midY = ((a.clientY + b.clientY) / 2) - rect.top;
        const anchorX = viewport.x + midX / viewport.zoomPxPerCell;
        const anchorY = viewport.y + midY / viewport.zoomPxPerCell;
        viewport.setZoom(pinchZoom * (nextDistance / pinchDistance));
        viewport.panBy(anchorX - (viewport.x + midX / viewport.zoomPxPerCell), anchorY - (viewport.y + midY / viewport.zoomPxPerCell));
        updateZoomHud();
        refreshSubscriptions();
      }
      return;
    }
    if (!dragging || activePointerId !== e.pointerId) return;
    const dx = (e.clientX - lastX) / viewport.zoomPxPerCell;
    const dy = (e.clientY - lastY) / viewport.zoomPxPerCell;
    if (Math.abs(dx) > 0.1 || Math.abs(dy) > 0.1) movedWhileDragging = true;
    viewport.panBy(-dx, -dy);
    lastX = e.clientX; lastY = e.clientY;
    refreshSubscriptions();
  });
  canvas.addEventListener("pointerleave", () => renderer.setHoverCell(null));
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
    if (lastPlace) {
      showToast("Waiting for server ack", "info");
      return;
    }
    const rect = canvas.getBoundingClientRect();
    const cx = (e.clientX - rect.left) / viewport.zoomPxPerCell + viewport.x;
    const cy = (e.clientY - rect.top) / viewport.zoomPxPerCell + viewport.y;
    const x = Math.floor(cx);
    const y = Math.floor(cy);
    if (x < 0 || x >= 1000 || y < 0 || y >= 1000) return;
    lastPlace = { x, y, cell: team };
    demoState.pendingPlacement = true;
    viewportContainer.classList.add("placement-pending");
    setActivity(`Placing ${team === 1 ? "black" : "white"} at ${x}, ${y}...`);
    renderer.setPendingStone(lastPlace);
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
      cooldown.setRemaining(0);
    } else {
      cooldown.setRemaining(remaining);
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
    zoomHud.textContent = `Zoom ${(viewport.zoomPxPerCell).toFixed(1)}x`;
  }

  function showToast(msg: string, tone: "error" | "info" | "success" = "error") {
    toast.textContent = msg;
    toast.dataset.tone = tone;
    toast.classList.add("visible");
    demoState.lastToast = msg;
    setTimeout(() => toast.classList.remove("visible"), 1500);
  }

  function setActivity(msg: string) {
    activity.textContent = msg;
  }

  function flashPlacementState(state: "confirmed" | "rejected") {
    viewportContainer.classList.remove("placement-pending", "placement-confirmed", "placement-rejected");
    viewportContainer.classList.add(state === "confirmed" ? "placement-confirmed" : "placement-rejected");
    setTimeout(() => {
      viewportContainer.classList.remove("placement-confirmed", "placement-rejected");
    }, 700);
  }

  function setConnectionState(state: "connected" | "reconnecting") {
    topBar.setConnectionState(state);
    demoState.connectionState = state;
  }

  function handleFrame(frame: DecodedServerFrame) {
    switch (frame.opcode) {
      case 0x81: { // Welcome
        team = (frame.team || 1) as Cell;
        setConnectionState("connected");
        topBar.setTeam(team as 1 | 2);
        demoState.team = team;
        serverOffsetMs = Number(frame.serverTimeMs) - Date.now();
        setActivity(`Playing as ${team === 1 ? "Black" : "White"}`);
        refreshSubscriptions();
        break;
      }
      case 0x82: // Snapshot
        cache.setSnapshot(frame.chunkId, frame.version, frame.packed);
        demoState.snapshotCount += 1;
        demoState.cacheSize = cache.size;
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
        demoState.cacheSize = cache.size;
        break;
      }
      case 0x84: { // PlaceAck
        nextAllowedMs = Number(frame.nextAllowedMs);
        demoState.lastAckCode = frame.errCode;
        demoState.pendingPlacement = false;
        if (frame.ok === 0) {
          renderer.setPendingStone(null);
          lastPlace = null;
          viewportContainer.classList.remove("placement-pending");
          flashPlacementState("rejected");
          const msg = errorText(frame.errCode);
          setActivity(`Rejected: ${msg}`);
          showToast(msg, "error");
        } else {
          renderer.setPendingStone(null);
          lastPlace = null;
          flashPlacementState("confirmed");
          setActivity("Move confirmed");
          showToast("Move confirmed", "success");
        }
        break;
      }
      case 0x85: { // Score
        topBar.setScore(frame.black, frame.white, frame.empty);
        demoState.scoreSeen = true;
        break;
      }
      case 0x86: { // Pong
        const sentAt = pendingPings.get(frame.nonce);
        if (sentAt !== undefined) {
          topBar.setPing(performance.now() - sentAt);
          pendingPings.delete(frame.nonce);
        }
        break;
      }
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

  function updateHover(e: PointerEvent) {
    const rect = canvas.getBoundingClientRect();
    const x = Math.floor((e.clientX - rect.left) / viewport.zoomPxPerCell + viewport.x);
    const y = Math.floor((e.clientY - rect.top) / viewport.zoomPxPerCell + viewport.y);
    if (x < 0 || x >= 1000 || y < 0 || y >= 1000 || lastPlace) {
      renderer.setHoverCell(null);
      return;
    }
    renderer.setHoverCell({ x, y, cell: team });
  }
}

function pointerDistance(a: PointerEvent, b: PointerEvent): number {
  return Math.hypot(a.clientX - b.clientX, a.clientY - b.clientY);
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
