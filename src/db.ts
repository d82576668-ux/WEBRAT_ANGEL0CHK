import dotenv from "dotenv";
import { Pool, QueryResultRow } from "pg";
import * as fs from "fs";
import * as path from "path";
import * as crypto from "crypto";

dotenv.config();

const connectionString = process.env.DATABASE_URL;

const useFallback = !connectionString || String(process.env.DB_FALLBACK || "").toLowerCase() === "1" || String(process.env.DB_FALLBACK || "").toLowerCase() === "true";

export const pool = useFallback
  ? (undefined as any)
  : new Pool({
      connectionString,
      ssl: { rejectUnauthorized: false }
    });

type DeviceRow = { id: string; name: string; status: string; created_at: string; last_seen?: string | null };
type TaskRow = { id: string; device_id: string; type: string; status: string; payload?: any; result?: any; created_at: string; updated_at?: string | null };

const storeDir = path.resolve(process.cwd(), "data", "store");
const devicesFile = path.join(storeDir, "devices.json");
const tasksFile = path.join(storeDir, "tasks.json");

function loadJson<T>(file: string): T[] {
  try {
    const s = fs.readFileSync(file, "utf8");
    const arr = JSON.parse(s);
    if (Array.isArray(arr)) return arr as T[];
    return [];
  } catch {
    return [];
  }
}

function saveJson<T>(file: string, arr: T[]) {
  try {
    fs.mkdirSync(path.dirname(file), { recursive: true });
    fs.writeFileSync(file, JSON.stringify(arr, null, 2), "utf8");
  } catch {}
}

function nowIso() {
  return new Date().toISOString();
}

async function fallbackQuery<T extends QueryResultRow = QueryResultRow>(text: string, params?: any[]) {
  let devices = loadJson<DeviceRow>(devicesFile);
  let tasks = loadJson<TaskRow>(tasksFile);
  const t = text.toLowerCase();
  if (t.startsWith("insert into devices")) {
    const id = crypto.randomUUID();
    const name = String(params?.[0] ?? "");
    const status = String(params?.[1] ?? "offline");
    const row: DeviceRow = { id, name, status, created_at: nowIso(), last_seen: null };
    devices.push(row);
    saveJson(devicesFile, devices);
    return [{ id: row.id, name: row.name, status: row.status, created_at: row.created_at }] as any as T[];
  }
  if (t.startsWith("select id, name, status, created_at, last_seen from devices")) {
    const rows = devices
      .slice()
      .sort((a, b) => (a.created_at > b.created_at ? -1 : 1))
      .map(d => ({ id: d.id, name: d.name, status: d.status, created_at: d.created_at, last_seen: d.last_seen ?? null }));
    return rows as any as T[];
  }
  if (t.startsWith("update devices set status")) {
    const status = String(params?.[0] ?? "offline");
    const id = String(params?.[1] ?? "");
    const d = devices.find(x => x.id === id);
    if (!d) return [] as any as T[];
    d.status = status;
    saveJson(devicesFile, devices);
    return [{ id: d.id, name: d.name, status: d.status, created_at: d.created_at }] as any as T[];
  }
  if (t.startsWith("update devices set last_seen")) {
    const id = String(params?.[0] ?? "");
    const d = devices.find(x => x.id === id);
    if (!d) return [] as any as T[];
    d.last_seen = nowIso();
    d.status = "online";
    saveJson(devicesFile, devices);
    return [{ id: d.id, name: d.name, status: d.status, last_seen: d.last_seen }] as any as T[];
  }
  if (t.startsWith("select id, name, status, created_at from devices where id =")) {
    const id = String(params?.[0] ?? "");
    const d = devices.find(x => x.id === id);
    if (!d) return [] as any as T[];
    return [{ id: d.id, name: d.name, status: d.status, created_at: d.created_at }] as any as T[];
  }
  if (t.startsWith("insert into tasks")) {
    const id = crypto.randomUUID();
    const device_id = String(params?.[0] ?? "");
    const type = String(params?.[1] ?? "");
    const status = String(params?.[2] ?? "queued");
    const payload = params?.[3] ?? null;
    const row: TaskRow = { id, device_id, type, status, payload, result: null, created_at: nowIso(), updated_at: null };
    tasks.push(row);
    saveJson(tasksFile, tasks);
    return [{ id: row.id, type: row.type, status: row.status, created_at: row.created_at }] as any as T[];
  }
  if (t.startsWith("select id, type, status, payload, result, created_at, updated_at from tasks where device_id")) {
    const device_id = String(params?.[0] ?? "");
    const rows = tasks
      .filter(x => x.device_id === device_id)
      .sort((a, b) => (a.created_at > b.created_at ? -1 : 1))
      .map(x => ({ id: x.id, type: x.type, status: x.status, payload: x.payload ?? null, result: x.result ?? null, created_at: x.created_at, updated_at: x.updated_at ?? null }));
    return rows as any as T[];
  }
  if (t.includes("from tasks") && t.includes("type = 'list_sources'") && t.includes("status = 'done'") && t.includes("limit 1")) {
    const device_id = String(params?.[0] ?? "");
    const rows = tasks
      .filter(x => x.device_id === device_id && x.type === "list_sources" && x.status === "done")
      .sort((a, b) => (a.created_at > b.created_at ? -1 : 1))
      .slice(0, 1)
      .map(x => ({ id: x.id, result: x.result ?? null, created_at: x.created_at }));
    return rows as any as T[];
  }
  if (t.includes("from tasks") && t.includes("type = 'snapshot'") && t.includes("status = 'done'") && t.includes("limit 1")) {
    const device_id = String(params?.[0] ?? "");
    const rows = tasks
      .filter(x => x.device_id === device_id && x.type === "snapshot" && x.status === "done")
      .sort((a, b) => (a.created_at > b.created_at ? -1 : 1))
      .slice(0, 1)
      .map(x => ({ id: x.id, result: x.result ?? null, created_at: x.created_at }));
    return rows as any as T[];
  }
  if (t.includes("from tasks") && t.includes("type = 'snapshot'") && t.includes("status = 'done'") && t.includes("order by created_at desc limit")) {
    const device_id = String(params?.[0] ?? "");
    const rows = tasks
      .filter(x => x.device_id === device_id && x.type === "snapshot" && x.status === "done")
      .sort((a, b) => (a.created_at > b.created_at ? -1 : 1))
      .map(x => ({ id: x.id, result: x.result ?? null, created_at: x.created_at }));
    return rows as any as T[];
  }
  if (t.startsWith("select id, device_id, type, status, payload, result, created_at, updated_at from tasks where id =")) {
    const id = String(params?.[0] ?? "");
    const x = tasks.find(y => y.id === id);
    if (!x) return [] as any as T[];
    return [{ id: x.id, device_id: x.device_id, type: x.type, status: x.status, payload: x.payload ?? null, result: x.result ?? null, created_at: x.created_at, updated_at: x.updated_at ?? null }] as any as T[];
  }
  if (t.startsWith("update tasks set status =")) {
    const status = String(params?.[0] ?? "");
    const result = params?.[1] ?? null;
    const id = String(params?.[2] ?? "");
    const x = tasks.find(y => y.id === id);
    if (!x) return [] as any as T[];
    x.status = status;
    x.result = result ?? null;
    x.updated_at = nowIso();
    saveJson(tasksFile, tasks);
    return [{ id: x.id, device_id: x.device_id, type: x.type, status: x.status, result: x.result ?? null, updated_at: x.updated_at }] as any as T[];
  }
  return [] as any as T[];
}

export async function query<T extends QueryResultRow = QueryResultRow>(text: string, params?: any[]) {
  if (useFallback) {
    return fallbackQuery<T>(text, params);
  }
  try {
    const client = await pool.connect();
    try {
      const res = await client.query(text, params);
      return res.rows as any as T[];
    } finally {
      client.release();
    }
  } catch {
    return fallbackQuery<T>(text, params);
  }
}
