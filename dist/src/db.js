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
exports.pool = void 0;
exports.query = query;
const dotenv_1 = __importDefault(require("dotenv"));
const pg_1 = require("pg");
const fs = __importStar(require("fs"));
const path = __importStar(require("path"));
const crypto = __importStar(require("crypto"));
dotenv_1.default.config();
const connectionString = process.env.DATABASE_URL;
const useFallback = !connectionString || String(process.env.DB_FALLBACK || "").toLowerCase() === "1" || String(process.env.DB_FALLBACK || "").toLowerCase() === "true";
exports.pool = useFallback
    ? undefined
    : new pg_1.Pool({
        connectionString,
        ssl: { rejectUnauthorized: false }
    });
const storeDir = path.resolve(process.cwd(), "data", "store");
const devicesFile = path.join(storeDir, "devices.json");
const tasksFile = path.join(storeDir, "tasks.json");
function loadJson(file) {
    try {
        const s = fs.readFileSync(file, "utf8");
        const arr = JSON.parse(s);
        if (Array.isArray(arr))
            return arr;
        return [];
    }
    catch {
        return [];
    }
}
function saveJson(file, arr) {
    try {
        fs.mkdirSync(path.dirname(file), { recursive: true });
        fs.writeFileSync(file, JSON.stringify(arr, null, 2), "utf8");
    }
    catch { }
}
function nowIso() {
    return new Date().toISOString();
}
async function fallbackQuery(text, params) {
    let devices = loadJson(devicesFile);
    let tasks = loadJson(tasksFile);
    const t = text.toLowerCase();
    if (t.startsWith("insert into devices")) {
        const id = crypto.randomUUID();
        const name = String(params?.[0] ?? "");
        const status = String(params?.[1] ?? "offline");
        const row = { id, name, status, created_at: nowIso(), last_seen: null };
        devices.push(row);
        saveJson(devicesFile, devices);
        return [{ id: row.id, name: row.name, status: row.status, created_at: row.created_at }];
    }
    if (t.startsWith("select id, name, status, created_at, last_seen from devices")) {
        const rows = devices
            .slice()
            .sort((a, b) => (a.created_at > b.created_at ? -1 : 1))
            .map(d => ({ id: d.id, name: d.name, status: d.status, created_at: d.created_at, last_seen: d.last_seen ?? null }));
        return rows;
    }
    if (t.startsWith("update devices set status")) {
        const status = String(params?.[0] ?? "offline");
        const id = String(params?.[1] ?? "");
        const d = devices.find(x => x.id === id);
        if (!d)
            return [];
        d.status = status;
        saveJson(devicesFile, devices);
        return [{ id: d.id, name: d.name, status: d.status, created_at: d.created_at }];
    }
    if (t.startsWith("update devices set last_seen")) {
        const id = String(params?.[0] ?? "");
        const d = devices.find(x => x.id === id);
        if (!d)
            return [];
        d.last_seen = nowIso();
        d.status = "online";
        saveJson(devicesFile, devices);
        return [{ id: d.id, name: d.name, status: d.status, last_seen: d.last_seen }];
    }
    if (t.startsWith("select id, name, status, created_at from devices where id =")) {
        const id = String(params?.[0] ?? "");
        const d = devices.find(x => x.id === id);
        if (!d)
            return [];
        return [{ id: d.id, name: d.name, status: d.status, created_at: d.created_at }];
    }
    if (t.startsWith("insert into tasks")) {
        const id = crypto.randomUUID();
        const device_id = String(params?.[0] ?? "");
        const type = String(params?.[1] ?? "");
        const status = String(params?.[2] ?? "queued");
        const payload = params?.[3] ?? null;
        const row = { id, device_id, type, status, payload, result: null, created_at: nowIso(), updated_at: null };
        tasks.push(row);
        saveJson(tasksFile, tasks);
        return [{ id: row.id, type: row.type, status: row.status, created_at: row.created_at }];
    }
    if (t.startsWith("select id, type, status, payload, result, created_at, updated_at from tasks where device_id")) {
        const device_id = String(params?.[0] ?? "");
        const rows = tasks
            .filter(x => x.device_id === device_id)
            .sort((a, b) => (a.created_at > b.created_at ? -1 : 1))
            .map(x => ({ id: x.id, type: x.type, status: x.status, payload: x.payload ?? null, result: x.result ?? null, created_at: x.created_at, updated_at: x.updated_at ?? null }));
        return rows;
    }
    if (t.includes("from tasks") && t.includes("type = 'list_sources'") && t.includes("status = 'done'") && t.includes("limit 1")) {
        const device_id = String(params?.[0] ?? "");
        const rows = tasks
            .filter(x => x.device_id === device_id && x.type === "list_sources" && x.status === "done")
            .sort((a, b) => (a.created_at > b.created_at ? -1 : 1))
            .slice(0, 1)
            .map(x => ({ id: x.id, result: x.result ?? null, created_at: x.created_at }));
        return rows;
    }
    if (t.includes("from tasks") && t.includes("type = 'snapshot'") && t.includes("status = 'done'") && t.includes("limit 1")) {
        const device_id = String(params?.[0] ?? "");
        const rows = tasks
            .filter(x => x.device_id === device_id && x.type === "snapshot" && x.status === "done")
            .sort((a, b) => (a.created_at > b.created_at ? -1 : 1))
            .slice(0, 1)
            .map(x => ({ id: x.id, result: x.result ?? null, created_at: x.created_at }));
        return rows;
    }
    if (t.includes("from tasks") && t.includes("type = 'snapshot'") && t.includes("status = 'done'") && t.includes("order by created_at desc limit")) {
        const device_id = String(params?.[0] ?? "");
        const rows = tasks
            .filter(x => x.device_id === device_id && x.type === "snapshot" && x.status === "done")
            .sort((a, b) => (a.created_at > b.created_at ? -1 : 1))
            .map(x => ({ id: x.id, result: x.result ?? null, created_at: x.created_at }));
        return rows;
    }
    if (t.startsWith("select id, device_id, type, status, payload, result, created_at, updated_at from tasks where id =")) {
        const id = String(params?.[0] ?? "");
        const x = tasks.find(y => y.id === id);
        if (!x)
            return [];
        return [{ id: x.id, device_id: x.device_id, type: x.type, status: x.status, payload: x.payload ?? null, result: x.result ?? null, created_at: x.created_at, updated_at: x.updated_at ?? null }];
    }
    if (t.startsWith("update tasks set status =")) {
        const status = String(params?.[0] ?? "");
        const result = params?.[1] ?? null;
        const id = String(params?.[2] ?? "");
        const x = tasks.find(y => y.id === id);
        if (!x)
            return [];
        x.status = status;
        x.result = result ?? null;
        x.updated_at = nowIso();
        saveJson(tasksFile, tasks);
        return [{ id: x.id, device_id: x.device_id, type: x.type, status: x.status, result: x.result ?? null, updated_at: x.updated_at }];
    }
    return [];
}
async function query(text, params) {
    if (useFallback) {
        return fallbackQuery(text, params);
    }
    try {
        const client = await exports.pool.connect();
        try {
            const res = await client.query(text, params);
            return res.rows;
        }
        finally {
            client.release();
        }
    }
    catch {
        return fallbackQuery(text, params);
    }
}
