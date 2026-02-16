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
Object.defineProperty(exports, "__esModule", { value: true });
const express_1 = require("express");
const fs = __importStar(require("fs"));
const path = __importStar(require("path"));
const db_1 = require("../db");
const router = (0, express_1.Router)();
const frames = new Map();
router.get("/", async (_req, res) => {
    const rows = await (0, db_1.query)("select id, name, status, created_at, last_seen from devices order by created_at desc");
    res.json(rows);
});
router.post("/register", async (req, res) => {
    const { name } = req.body || {};
    if (!name) {
        res.status(400).json({ error: "name is required" });
        return;
    }
    const rows = await (0, db_1.query)("insert into devices (name, status) values ($1, $2) returning id, name, status, created_at", [name, "offline"]);
    res.status(201).json(rows[0]);
});
router.post("/:id/status", async (req, res) => {
    const id = String(req.params.id);
    const { status } = req.body || {};
    const allowed = new Set(["online", "offline"]);
    if (!status || !allowed.has(String(status))) {
        res.status(400).json({ error: "invalid status" });
        return;
    }
    const rows = await (0, db_1.query)("update devices set status = $1 where id = $2 returning id, name, status, created_at", [String(status), id]);
    if (!rows[0]) {
        res.status(404).json({ error: "not found" });
        return;
    }
    res.json(rows[0]);
});
router.post("/:id/heartbeat", async (req, res) => {
    const id = String(req.params.id);
    const rows = await (0, db_1.query)("update devices set last_seen = now(), status = 'online' where id = $1 returning id, name, status, last_seen", [id]);
    if (!rows[0]) {
        res.status(404).json({ error: "not found" });
        return;
    }
    res.json(rows[0]);
});
router.get("/:id", async (req, res) => {
    const id = String(req.params.id);
    const rows = await (0, db_1.query)("select id, name, status, created_at from devices where id = $1", [id]);
    if (!rows[0]) {
        res.status(404).json({ error: "not found" });
        return;
    }
    res.json(rows[0]);
});
async function createTask(deviceId, type, payload) {
    const rows = await (0, db_1.query)("insert into tasks (device_id, type, status, payload) values ($1, $2, $3, $4) returning id, type, status, created_at", [deviceId, type, "queued", payload ?? null]);
    return rows[0];
}
router.get("/:id/tasks", async (req, res) => {
    const id = String(req.params.id);
    const rows = await (0, db_1.query)("select id, type, status, payload, result, created_at, updated_at from tasks where device_id = $1 order by created_at desc", [id]);
    res.json(rows);
});
router.post("/:id/start", async (req, res) => {
    const id = String(req.params.id);
    const task = await createTask(id, "start_stream", req.body ?? null);
    res.status(202).json(task);
});
router.post("/:id/stop", async (req, res) => {
    const id = String(req.params.id);
    const task = await createTask(id, "stop_stream");
    res.status(202).json(task);
});
router.post("/:id/snapshot", async (req, res) => {
    const id = String(req.params.id);
    const task = await createTask(id, "snapshot");
    res.status(202).json(task);
});
router.post("/:id/info", async (req, res) => {
    const id = String(req.params.id);
    const task = await createTask(id, "collect_info");
    res.status(202).json(task);
});
router.post("/:id/sources", async (req, res) => {
    const id = String(req.params.id);
    const task = await createTask(id, "list_sources");
    res.status(202).json(task);
});
router.get("/:id/sources-last", async (req, res) => {
    const id = String(req.params.id);
    const rows = await (0, db_1.query)("select id, result, created_at from tasks where device_id = $1 and type = 'list_sources' and status = 'done' order by created_at desc limit 1", [id]);
    if (!rows[0]) {
        res.status(404).json({ error: "no sources" });
        return;
    }
    res.json(rows[0]);
});
router.post("/:id/frame", async (req, res) => {
    const id = String(req.params.id);
    const { png_b64, jpeg_b64 } = req.body || {};
    const b64 = typeof jpeg_b64 === "string" ? jpeg_b64 : png_b64;
    const format = typeof jpeg_b64 === "string" ? "jpeg" : "png";
    if (!b64 || typeof b64 !== "string") {
        res.status(400).json({ error: "image b64 required" });
        return;
    }
    const ts = new Date().toISOString();
    frames.set(id, { b64, ts, format });
    res.status(201).json({ ok: true, ts, format });
});
router.get("/:id/stream-sse", async (req, res) => {
    const id = String(req.params.id);
    res.setHeader("Content-Type", "text/event-stream");
    res.setHeader("Cache-Control", "no-cache");
    res.setHeader("Connection", "keep-alive");
    res.flushHeaders?.();
    let lastTs = "";
    const iv = setInterval(() => {
        const f = frames.get(id);
        if (f && f.ts !== lastTs) {
            lastTs = f.ts;
            res.write(`data: ${JSON.stringify({ b64: f.b64, ts: f.ts, format: f.format })}\n\n`);
        }
    }, 1000);
    req.on("close", () => {
        clearInterval(iv);
    });
});
router.get("/:id/last-snapshot", async (req, res) => {
    const id = String(req.params.id);
    const rows = await (0, db_1.query)("select id, result, created_at from tasks where device_id = $1 and type = 'snapshot' and status = 'done' order by created_at desc limit 1", [id]);
    if (!rows[0]) {
        res.status(404).json({ error: "no snapshot" });
        return;
    }
    res.json(rows[0]);
});
router.get("/:id/snapshots", async (req, res) => {
    const id = String(req.params.id);
    const rows = await (0, db_1.query)("select id, result, created_at from tasks where device_id = $1 and type = 'snapshot' and status = 'done' order by created_at desc limit 20", [id]);
    res.json(rows);
});
router.get("/:id/snapshots-files", async (req, res) => {
    const id = String(req.params.id);
    const dir = path.resolve(process.cwd(), "data", "devices", id, "snapshots");
    let files = [];
    try {
        files = fs.readdirSync(dir).filter(f => f.endsWith(".png") || f.endsWith(".jpg"));
    }
    catch {
        files = [];
    }
    const items = files
        .sort((a, b) => (a > b ? -1 : 1))
        .slice(0, 50)
        .map(f => ({
        file: f,
        url: `/files/devices/${id}/snapshots/${f}`
    }));
    res.json(items);
});
router.get("/:id/files", async (req, res) => {
    const id = String(req.params.id);
    const dir = path.resolve(process.cwd(), "data", "devices", id, "files");
    let files = [];
    try {
        files = fs.readdirSync(dir).filter(f => /^[a-zA-Z0-9_.-]+$/.test(f));
    }
    catch {
        files = [];
    }
    const items = files
        .sort((a, b) => (a > b ? -1 : 1))
        .slice(0, 200)
        .map(f => ({
        file: f,
        url: `/files/devices/${id}/files/${f}`
    }));
    res.json(items);
});
router.post("/:id/files", async (req, res) => {
    const id = String(req.params.id);
    const { filename, b64 } = req.body || {};
    if (!filename || !/^[a-zA-Z0-9_.-]+$/.test(String(filename))) {
        res.status(400).json({ error: "invalid filename" });
        return;
    }
    if (!b64 || typeof b64 !== "string") {
        res.status(400).json({ error: "b64 required" });
        return;
    }
    const dir = path.resolve(process.cwd(), "data", "devices", id, "files");
    try {
        fs.mkdirSync(dir, { recursive: true });
        const buf = Buffer.from(b64, "base64");
        fs.writeFileSync(path.join(dir, String(filename)), buf);
    }
    catch {
        res.status(500).json({ error: "write failed" });
        return;
    }
    res.status(201).json({ ok: true, file: filename });
});
router.delete("/:id/files/:name", async (req, res) => {
    const id = String(req.params.id);
    const name = String(req.params.name);
    if (!/^[a-zA-Z0-9_.-]+$/.test(name)) {
        res.status(400).json({ error: "invalid name" });
        return;
    }
    const file = path.resolve(process.cwd(), "data", "devices", id, "files", name);
    try {
        fs.unlinkSync(file);
    }
    catch {
        res.status(404).json({ error: "not found" });
        return;
    }
    res.json({ ok: true });
});
exports.default = router;
