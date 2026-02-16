import { Router, Request, Response } from "express";
import * as fs from "fs";
import * as path from "path";
import { query } from "../db";

const router = Router();
const frames = new Map<string, { b64: string; ts: string; format: "png" | "jpeg" }>();

router.get("/", async (_req: Request, res: Response) => {
  const rows = await query(
    "select id, name, status, created_at, last_seen from devices order by created_at desc"
  );
  res.json(rows);
});

router.post("/register", async (req: Request, res: Response) => {
  const { name } = req.body || {};
  if (!name) {
    res.status(400).json({ error: "name is required" });
    return;
  }
  const rows = await query(
    "insert into devices (name, status) values ($1, $2) returning id, name, status, created_at",
    [name, "offline"]
  );
  res.status(201).json(rows[0]);
});

router.post("/:id/status", async (req: Request, res: Response) => {
  const id = String(req.params.id);
  const { status } = req.body || {};
  const allowed = new Set(["online", "offline"]);
  if (!status || !allowed.has(String(status))) {
    res.status(400).json({ error: "invalid status" });
    return;
  }
  const rows = await query(
    "update devices set status = $1 where id = $2 returning id, name, status, created_at",
    [String(status), id]
  );
  if (!rows[0]) {
    res.status(404).json({ error: "not found" });
    return;
  }
  res.json(rows[0]);
});

router.post("/:id/heartbeat", async (req: Request, res: Response) => {
  const id = String(req.params.id);
  const rows = await query(
    "update devices set last_seen = now(), status = 'online' where id = $1 returning id, name, status, last_seen",
    [id]
  );
  if (!rows[0]) {
    res.status(404).json({ error: "not found" });
    return;
  }
  res.json(rows[0]);
});

router.get("/:id", async (req: Request, res: Response) => {
  const id = String(req.params.id);
  const rows = await query(
    "select id, name, status, created_at from devices where id = $1",
    [id]
  );
  if (!rows[0]) {
    res.status(404).json({ error: "not found" });
    return;
  }
  res.json(rows[0]);
});

async function createTask(deviceId: string, type: string, payload?: any) {
  const rows = await query(
    "insert into tasks (device_id, type, status, payload) values ($1, $2, $3, $4) returning id, type, status, created_at",
    [deviceId, type, "queued", payload ?? null]
  );
  return rows[0];
}

router.get("/:id/tasks", async (req: Request, res: Response) => {
  const id = String(req.params.id);
  const rows = await query(
    "select id, type, status, payload, result, created_at, updated_at from tasks where device_id = $1 order by created_at desc",
    [id]
  );
  res.json(rows);
});

router.post("/:id/start", async (req: Request, res: Response) => {
  const id = String(req.params.id);
  const task = await createTask(id, "start_stream", req.body ?? null);
  res.status(202).json(task);
});

router.post("/:id/stop", async (req: Request, res: Response) => {
  const id = String(req.params.id);
  const task = await createTask(id, "stop_stream");
  res.status(202).json(task);
});

router.post("/:id/snapshot", async (req: Request, res: Response) => {
  const id = String(req.params.id);
  const task = await createTask(id, "snapshot");
  res.status(202).json(task);
});

router.post("/:id/info", async (req: Request, res: Response) => {
  const id = String(req.params.id);
  const task = await createTask(id, "collect_info");
  res.status(202).json(task);
});

router.post("/:id/sources", async (req: Request, res: Response) => {
  const id = String(req.params.id);
  const task = await createTask(id, "list_sources");
  res.status(202).json(task);
});

router.get("/:id/sources-last", async (req: Request, res: Response) => {
  const id = String(req.params.id);
  const rows = await query(
    "select id, result, created_at from tasks where device_id = $1 and type = 'list_sources' and status = 'done' order by created_at desc limit 1",
    [id]
  );
  if (!rows[0]) {
    res.status(404).json({ error: "no sources" });
    return;
  }
  res.json(rows[0]);
});

router.post("/:id/frame", async (req: Request, res: Response) => {
  const id = String(req.params.id);
  const { png_b64, jpeg_b64 } = req.body || {};
  const b64 = typeof jpeg_b64 === "string" ? jpeg_b64 : png_b64;
  const format: "png" | "jpeg" = typeof jpeg_b64 === "string" ? "jpeg" : "png";
  if (!b64 || typeof b64 !== "string") {
    res.status(400).json({ error: "image b64 required" });
    return;
  }
  const ts = new Date().toISOString();
  frames.set(id, { b64, ts, format });
  res.status(201).json({ ok: true, ts, format });
});

router.get("/:id/stream-sse", async (req: Request, res: Response) => {
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

router.get("/:id/last-snapshot", async (req: Request, res: Response) => {
  const id = String(req.params.id);
  const rows = await query(
    "select id, result, created_at from tasks where device_id = $1 and type = 'snapshot' and status = 'done' order by created_at desc limit 1",
    [id]
  );
  if (!rows[0]) {
    res.status(404).json({ error: "no snapshot" });
    return;
  }
  res.json(rows[0]);
});

router.get("/:id/snapshots", async (req: Request, res: Response) => {
  const id = String(req.params.id);
  const rows = await query(
    "select id, result, created_at from tasks where device_id = $1 and type = 'snapshot' and status = 'done' order by created_at desc limit 20",
    [id]
  );
  res.json(rows);
});

router.get("/:id/snapshots-files", async (req: Request, res: Response) => {
  const id = String(req.params.id);
  const dir = path.resolve(process.cwd(), "data", "devices", id, "snapshots");
  let files: string[] = [];
  try {
    files = fs.readdirSync(dir).filter(f => f.endsWith(".png") || f.endsWith(".jpg"));
  } catch {
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

router.get("/:id/files", async (req: Request, res: Response) => {
  const id = String(req.params.id);
  const dir = path.resolve(process.cwd(), "data", "devices", id, "files");
  let files: string[] = [];
  try {
    files = fs.readdirSync(dir).filter(f => /^[a-zA-Z0-9_.-]+$/.test(f));
  } catch {
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

router.post("/:id/files", async (req: Request, res: Response) => {
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
  } catch {
    res.status(500).json({ error: "write failed" });
    return;
  }
  res.status(201).json({ ok: true, file: filename });
});

router.delete("/:id/files/:name", async (req: Request, res: Response) => {
  const id = String(req.params.id);
  const name = String(req.params.name);
  if (!/^[a-zA-Z0-9_.-]+$/.test(name)) {
    res.status(400).json({ error: "invalid name" });
    return;
  }
  const file = path.resolve(process.cwd(), "data", "devices", id, "files", name);
  try {
    fs.unlinkSync(file);
  } catch {
    res.status(404).json({ error: "not found" });
    return;
  }
  res.json({ ok: true });
});

export default router;
