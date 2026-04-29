// Simple WebSocket bot loadtester for MMOthello.
//
// Usage:
//   node scripts/loadtest/bots.mjs --base http://localhost:8080 --clients 100 --duration 60
//
// Each bot:
//   1. GET /session and parse the mmothello_token cookie value out of Set-Cookie.
//   2. Open ws://.../ws?token=<hex> (server accepts a query-string fallback
//      because Node's built-in WebSocket can't send a Cookie header).
//   3. Subscribe to a 2x2 chunk neighborhood at a per-bot offset.
//   4. Try to place a stone every ~5 seconds at one of the four cells next
//      to the seeded white stone in the home chunk.
//
// Reports placed_ok and error counts per code every 2 seconds.

const args = parseArgs(process.argv.slice(2));
const base = args.base ?? "http://localhost:8080";
const clients = Number(args.clients ?? 100);
const durationSec = Number(args.duration ?? 60);
const cooldownMs = Number(args["cooldown-ms"] ?? process.env.MMOTHELLO_COOLDOWN_MS ?? 5000);
const placeJitterMs = 250;

const stats = {
  placedOk: 0,
  errorsByCode: new Map(),
  latencies: [],
  pendingByBot: new Map(),
};

async function main() {
  const wsBase = base.replace(/^http/, "ws");
  console.log(`launching ${clients} bots → ${base} for ${durationSec}s cooldown_ms=${cooldownMs}`);
  for (let i = 0; i < clients; i++) {
    startBot(base, wsBase + "/ws", i).catch((e) => console.error("bot", i, e.message));
    await sleep(5);
  }
  const reporter = setInterval(report, 2000);
  await sleep(durationSec * 1000);
  clearInterval(reporter);
  report();
  process.exit(0);
}

async function startBot(httpBase, wsURL, idx) {
  const sessionResp = await fetch(httpBase + "/session");
  const cookie = sessionResp.headers.get("set-cookie");
  if (!cookie) throw new Error("no Set-Cookie from /session");
  const tokenHex = parseCookieValue(cookie, "mmothello_token");
  if (!tokenHex) throw new Error("no mmothello_token in cookie");

  const ws = new WebSocket(`${wsURL}?token=${tokenHex}`);
  ws.binaryType = "arraybuffer";
  await new Promise((resolve, reject) => {
    ws.addEventListener("open", () => resolve(undefined), { once: true });
    ws.addEventListener("error", (e) => reject(new Error("ws error")), { once: true });
  });

  const cx = idx % 20;
  const cy = Math.floor(idx / 20) % 20;
  for (const [dx, dy] of [[0, 0], [1, 0], [0, 1], [1, 1]]) {
    const id = ((cy + dy) % 20) * 20 + ((cx + dx) % 20);
    ws.send(encodeSubscribe(id));
  }

  ws.addEventListener("message", (event) => {
    if (!(event.data instanceof ArrayBuffer)) return;
    handleFrame(event.data, idx);
  });

  const tryPlace = () => {
    if (ws.readyState !== WebSocket.OPEN) return;
    const baseX = cx * 50 + 24;
    const baseY = cy * 50 + 24;
    const candidates = [
      [baseX - 1, baseY],
      [baseX + 2, baseY],
      [baseX, baseY - 1],
      [baseX, baseY + 2],
    ];
    const [x, y] = candidates[Math.floor(Math.random() * candidates.length)];
    stats.pendingByBot.set(idx, performance.now());
    ws.send(encodePlace(x, y));
    setTimeout(tryPlace, cooldownMs + Math.random() * placeJitterMs);
  };
  setTimeout(tryPlace, Math.random() * cooldownMs);
}

function handleFrame(buf, botIdx) {
  const view = new DataView(buf);
  const op = view.getUint8(0);
  if (op === 0x84) {
    const sentAt = stats.pendingByBot.get(botIdx);
    if (sentAt !== undefined) {
      stats.latencies.push(performance.now() - sentAt);
      stats.pendingByBot.delete(botIdx);
    }
    const ok = view.getUint8(1);
    if (ok === 1) stats.placedOk++;
    else {
      const code = view.getUint8(10);
      stats.errorsByCode.set(code, (stats.errorsByCode.get(code) ?? 0) + 1);
    }
  }
}

function percentile(sorted, p) {
  if (sorted.length === 0) return 0;
  const idx = Math.min(sorted.length - 1, Math.floor((p / 100) * sorted.length));
  return sorted[idx];
}

function report() {
  const errs = [...stats.errorsByCode.entries()].map(([k, v]) => `${k}=${v}`).join(",");
  const sorted = [...stats.latencies].sort((a, b) => a - b);
  const p50 = percentile(sorted, 50).toFixed(1);
  const p95 = percentile(sorted, 95).toFixed(1);
  const p99 = percentile(sorted, 99).toFixed(1);
  console.log(`placed_ok=${stats.placedOk} errors[${errs || "none"}] latency_ms p50=${p50} p95=${p95} p99=${p99}`);
}

function parseCookieValue(setCookie, name) {
  for (const pair of setCookie.split(/, (?=[^;]+=)|; */)) {
    const m = pair.match(/^\s*([^=]+)=(.*)$/);
    if (m && m[1] === name) return m[2];
  }
  return null;
}

function encodeSubscribe(chunkId) {
  const out = new Uint8Array(3);
  const dv = new DataView(out.buffer);
  dv.setUint8(0, 0x02);
  dv.setUint16(1, chunkId, true);
  return out;
}

function encodePlace(x, y) {
  const out = new Uint8Array(5);
  const dv = new DataView(out.buffer);
  dv.setUint8(0, 0x04);
  dv.setUint16(1, x, true);
  dv.setUint16(3, y, true);
  return out;
}

function parseArgs(argv) {
  const out = {};
  for (let i = 0; i < argv.length; i++) {
    if (argv[i].startsWith("--")) out[argv[i].slice(2)] = argv[i + 1];
  }
  return out;
}

function sleep(ms) { return new Promise((r) => setTimeout(r, ms)); }

main().catch((e) => { console.error(e); process.exit(1); });
