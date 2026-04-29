#!/usr/bin/env node
import { execFileSync, spawn } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";

class CDP {
  constructor(ws) {
    this.ws = ws;
    this.nextId = 1;
    this.pending = new Map();
    ws.addEventListener("message", (event) => this.handleMessage(event.data));
    ws.addEventListener("close", () => {
      for (const { reject } of this.pending.values()) reject(new Error("CDP socket closed"));
      this.pending.clear();
    });
  }

  static async connect(wsUrl) {
    const ws = new WebSocket(wsUrl);
    await new Promise((resolve, reject) => {
      ws.addEventListener("open", resolve, { once: true });
      ws.addEventListener("error", () => reject(new Error("failed to connect to CDP")), { once: true });
    });
    return new CDP(ws);
  }

  send(method, params = {}, sessionId) {
    const id = this.nextId++;
    const payload = { id, method, params };
    if (sessionId) payload.sessionId = sessionId;
    const promise = new Promise((resolve, reject) => {
      this.pending.set(id, { resolve, reject });
      setTimeout(() => {
        if (this.pending.delete(id)) reject(new Error(`CDP timeout: ${method}`));
      }, 5000);
    });
    this.ws.send(JSON.stringify(payload));
    return promise;
  }

  handleMessage(raw) {
    const text = typeof raw === "string" ? raw : Buffer.from(raw).toString("utf8");
    const msg = JSON.parse(text);
    if (!msg.id) return;
    const pending = this.pending.get(msg.id);
    if (!pending) return;
    this.pending.delete(msg.id);
    if (msg.error) pending.reject(new Error(msg.error.message));
    else pending.resolve(msg.result ?? {});
  }

  close() {
    this.ws.close();
  }
}

const args = parseArgs(process.argv.slice(2));
const url = args.url ?? "http://127.0.0.1:5173/";
const timeoutMs = Number(args.timeout ?? 15000);
const port = Number(args.port ?? (9400 + Math.floor(Math.random() * 400)));
const browserPath = findBrowser();

if (!browserPath) {
  console.error("No Chrome/Chromium browser found. Set BROWSER=/path/to/chrome and retry.");
  process.exit(1);
}

const profileDir = fs.mkdtempSync(path.join(os.tmpdir(), "mmothello-smoke-"));
const chrome = spawn(browserPath, [
  "--headless=old",
  `--remote-debugging-port=${port}`,
  `--user-data-dir=${profileDir}`,
  "--disable-background-networking",
  "--disable-default-apps",
  "--disable-gpu",
  "--disable-popup-blocking",
  "--no-default-browser-check",
  "--no-first-run",
  "about:blank",
], { stdio: ["ignore", "ignore", "pipe"] });

let stderr = "";
chrome.stderr.on("data", (chunk) => { stderr += chunk.toString(); });

try {
  await runSmoke();
} catch (err) {
  console.error(`smoke failed: ${err.message}`);
  if (stderr.trim()) console.error(stderr.trim().split("\n").slice(-6).join("\n"));
  process.exitCode = 1;
} finally {
  chrome.kill("SIGTERM");
  fs.rmSync(profileDir, { recursive: true, force: true });
}

async function runSmoke() {
  const wsUrl = await waitForDevToolsWs(port, Math.min(timeoutMs, 15000), () => stderr);
  const cdp = await CDP.connect(wsUrl);
  const { targetId } = await cdp.send("Target.createTarget", { url: "about:blank" });
  const { sessionId } = await cdp.send("Target.attachToTarget", { targetId, flatten: true });
  const evaluate = (expression) => evaluateInPage(cdp, sessionId, expression);

  await cdp.send("Page.enable", {}, sessionId);
  await cdp.send("Runtime.enable", {}, sessionId);
  await cdp.send("Page.navigate", { url }, sessionId);

  await waitFor(async () => {
    return await evaluate(`Boolean(
      document.querySelector('.board-canvas') &&
      document.querySelector('.minimap') &&
      window.__mmothelloDemo &&
      window.__mmothelloDemo.connectionState === 'connected' &&
      window.__mmothelloDemo.team &&
      window.__mmothelloDemo.snapshotCount > 0 &&
      window.__mmothelloDemo.scoreSeen
    )`);
  }, timeoutMs, "app connected, scored, and received a snapshot");
  console.log("ok app connected with session, score, and snapshot");

  await evaluate(`document.querySelector('.icon-button')?.click(); true`);
  await waitFor(async () => await evaluate(`window.__mmothelloDemo?.helpOpen === true`), 3000, "help panel opens");
  console.log("ok help panel toggles");

  await evaluate(`
    (() => {
      const canvas = document.querySelector('.board-canvas');
      const rect = canvas.getBoundingClientRect();
      canvas.dispatchEvent(new MouseEvent('click', {
        bubbles: true,
        cancelable: true,
        clientX: rect.left + rect.width / 2,
        clientY: rect.top + rect.height / 2,
        button: 0
      }));
      return true;
    })()
  `);
  await waitFor(async () => {
    return await evaluate(`Boolean(
      window.__mmothelloDemo &&
      (window.__mmothelloDemo.lastAckCode !== null || window.__mmothelloDemo.lastToast)
    )`);
  }, timeoutMs, "placement acknowledgement or rejection");
  console.log("ok placement feedback observed");

  await evaluate(`if (window.__mmothelloDemo?.helpOpen) document.querySelector('.icon-button')?.click(); true`);
  await cdp.send("Emulation.setDeviceMetricsOverride", {
    width: 390,
    height: 844,
    deviceScaleFactor: 2,
    mobile: true,
  }, sessionId);
  await delay(300);
  const noOverlap = await evaluate(`
    (() => {
      const selectors = ['.minimap', '.cooldown-chip', '.zoom-hud', '.activity-chip'];
      const boxes = selectors.map((selector) => {
        const el = document.querySelector(selector);
        if (!el) return null;
        const r = el.getBoundingClientRect();
        return { selector, left: r.left, top: r.top, right: r.right, bottom: r.bottom };
      }).filter(Boolean);
      for (let i = 0; i < boxes.length; i += 1) {
        for (let j = i + 1; j < boxes.length; j += 1) {
          const a = boxes[i], b = boxes[j];
          const overlap = a.left < b.right && a.right > b.left && a.top < b.bottom && a.bottom > b.top;
          if (overlap) return false;
        }
      }
      return true;
    })()
  `);
  if (!noOverlap) throw new Error("mobile HUD controls overlap");
  console.log("ok mobile HUD controls do not overlap");

  await cdp.send("Target.closeTarget", { targetId });
  cdp.close();
}

async function evaluateInPage(cdp, sessionId, expression) {
  const result = await cdp.send("Runtime.evaluate", {
    expression,
    awaitPromise: true,
    returnByValue: true,
  }, sessionId);
  if (result.exceptionDetails) {
    throw new Error(result.exceptionDetails.text ?? "page evaluation failed");
  }
  return result.result?.value;
}

async function waitFor(fn, timeout, label) {
  const deadline = Date.now() + timeout;
  let lastError;
  while (Date.now() < deadline) {
    try {
      if (await fn()) return;
    } catch (err) {
      lastError = err;
    }
    await delay(200);
  }
  throw new Error(`timed out waiting for ${label}${lastError ? `: ${lastError.message}` : ""}`);
}

async function waitForDevToolsWs(port, timeout, getStderr) {
  let lastError;
  return await waitFor(async () => {
    const match = getStderr().match(/DevTools listening on (ws:\/\/[^\s]+)/);
    if (match) {
      waitForDevToolsWs.value = match[1];
      return true;
    }
    try {
      const res = await fetch(`http://127.0.0.1:${port}/json/version`, { signal: AbortSignal.timeout(500) });
      if (!res.ok) return false;
      const version = await res.json();
      waitForDevToolsWs.value = version.webSocketDebuggerUrl;
      return Boolean(waitForDevToolsWs.value);
    } catch (err) {
      lastError = err;
      return false;
    }
  }, timeout, `Chrome DevTools endpoint${lastError ? ` (${lastError.message})` : ""}`).then(() => waitForDevToolsWs.value);
}

function findBrowser() {
  const candidates = [
    process.env.BROWSER,
    "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
    "/Applications/Chromium.app/Contents/MacOS/Chromium",
    "/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
    which("google-chrome"),
    which("chromium"),
    which("chromium-browser"),
    which("chrome"),
    which("msedge"),
  ].filter(Boolean);
  return candidates.find((candidate) => fs.existsSync(candidate));
}

function which(command) {
  try {
    return execFileSync("which", [command], { encoding: "utf8", stdio: ["ignore", "pipe", "ignore"] }).trim();
  } catch {
    return "";
  }
}

function delay(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

function parseArgs(argv) {
  const out = {};
  for (let i = 0; i < argv.length; i += 1) {
    if (!argv[i].startsWith("--")) continue;
    const key = argv[i].slice(2);
    const next = argv[i + 1];
    if (!next || next.startsWith("--")) out[key] = "1";
    else {
      out[key] = next;
      i += 1;
    }
  }
  return out;
}
