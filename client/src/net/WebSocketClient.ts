import {
  decodeServerFrame,
  encodePing,
  encodePlace,
  encodeSubscribe,
  encodeUnsubscribe,
  type DecodedServerFrame,
} from "./protocol";

type WebSocketClientOptions = {
  url: string;
  onOpen?: () => void;
  onClose?: () => void;
  onFrame?: (frame: DecodedServerFrame) => void;
};

const INITIAL_RECONNECT_MS = 250;
const MAX_RECONNECT_MS = 8000;

export class WebSocketClient {
  private readonly url: string;
  private readonly onOpen?: () => void;
  private readonly onClose?: () => void;
  private readonly onFrame?: (frame: DecodedServerFrame) => void;
  private socket?: WebSocket;
  private reconnectMs = INITIAL_RECONNECT_MS;
  private reconnectTimer?: number;
  private subscribedChunks = new Set<number>();

  constructor(options: WebSocketClientOptions) {
    this.url = options.url;
    this.onOpen = options.onOpen;
    this.onClose = options.onClose;
    this.onFrame = options.onFrame;
  }

  connect(): void {
    this.clearReconnectTimer();
    this.socket = new WebSocket(this.url);
    this.socket.binaryType = "arraybuffer";
    this.socket.addEventListener("open", () => this.handleOpen());
    this.socket.addEventListener("message", (event) => this.handleMessage(event));
    this.socket.addEventListener("close", () => this.handleClose());
    this.socket.addEventListener("error", () => this.socket?.close());
  }

  disconnect(): void {
    this.clearReconnectTimer();
    this.socket?.close();
  }

  send(frame: ArrayBuffer): void {
    if (this.socket?.readyState !== WebSocket.OPEN) return;
    this.socket.send(frame);
  }

  subscribe(chunkId: number): void {
    if (this.subscribedChunks.has(chunkId)) return;
    this.subscribedChunks.add(chunkId);
    this.send(encodeSubscribe(chunkId));
  }

  unsubscribe(chunkId: number): void {
    if (!this.subscribedChunks.has(chunkId)) return;
    this.subscribedChunks.delete(chunkId);
    this.send(encodeUnsubscribe(chunkId));
  }

  setSubscriptions(nextIds: Iterable<number>): void {
    const next = new Set(nextIds);
    for (const id of this.subscribedChunks) {
      if (!next.has(id)) this.unsubscribe(id);
    }
    for (const id of next) {
      if (!this.subscribedChunks.has(id)) this.subscribe(id);
    }
  }

  place(x: number, y: number): void {
    this.send(encodePlace(x, y));
  }

  ping(nonce: number): void {
    this.send(encodePing(nonce));
  }

  private handleOpen(): void {
    this.reconnectMs = INITIAL_RECONNECT_MS;
    // Re-subscribe to whatever the renderer was tracking before reconnect.
    const previous = Array.from(this.subscribedChunks);
    this.subscribedChunks.clear();
    for (const id of previous) this.subscribe(id);
    this.onOpen?.();
  }

  private handleMessage(event: MessageEvent): void {
    if (!(event.data instanceof ArrayBuffer)) return;
    let frame: DecodedServerFrame;
    try {
      frame = decodeServerFrame(event.data);
    } catch {
      return;
    }
    this.onFrame?.(frame);
  }

  private handleClose(): void {
    this.onClose?.();
    this.reconnectTimer = window.setTimeout(() => this.connect(), this.reconnectMs);
    this.reconnectMs = Math.min(this.reconnectMs * 2, MAX_RECONNECT_MS);
  }

  private clearReconnectTimer(): void {
    if (this.reconnectTimer !== undefined) {
      window.clearTimeout(this.reconnectTimer);
      this.reconnectTimer = undefined;
    }
  }
}
