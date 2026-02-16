"use strict";
var __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};
Object.defineProperty(exports, "__esModule", { value: true });
const fs_1 = __importDefault(require("fs"));
const path_1 = __importDefault(require("path"));
const db_1 = require("../src/db");
async function ensureMigrationsTable() {
    await (0, db_1.query)("create table if not exists migrations (id serial primary key, name text not null unique, applied_at timestamptz not null default now())");
}
async function appliedNames() {
    const rows = await (0, db_1.query)("select name from migrations");
    return new Set(rows.map((r) => r.name));
}
async function applyMigration(name, sql) {
    await (0, db_1.query)(sql);
    await (0, db_1.query)("insert into migrations (name) values ($1)", [name]);
}
async function main() {
    await ensureMigrationsTable();
    const applied = await appliedNames();
    const dir = path_1.default.resolve(process.cwd(), "migrations");
    const files = fs_1.default
        .readdirSync(dir)
        .filter(f => f.endsWith(".sql"))
        .sort();
    for (const file of files) {
        if (applied.has(file))
            continue;
        const sql = fs_1.default.readFileSync(path_1.default.join(dir, file), "utf8");
        await applyMigration(file, sql);
        console.log(`applied ${file}`);
    }
    console.log("migrations complete");
}
main().catch(err => {
    console.error(err);
    process.exit(1);
});
