import fs from "fs";
import path from "path";
import { query } from "../src/db";

async function ensureMigrationsTable() {
  await query(
    "create table if not exists migrations (id serial primary key, name text not null unique, applied_at timestamptz not null default now())"
  );
}

async function appliedNames(): Promise<Set<string>> {
  const rows = await query<{ name: string }>("select name from migrations");
  return new Set(rows.map((r: { name: string }) => r.name));
}

async function applyMigration(name: string, sql: string) {
  await query(sql);
  await query("insert into migrations (name) values ($1)", [name]);
}

async function main() {
  await ensureMigrationsTable();
  const applied = await appliedNames();
  const dir = path.resolve(process.cwd(), "migrations");
  const files = fs
    .readdirSync(dir)
    .filter(f => f.endsWith(".sql"))
    .sort();
  for (const file of files) {
    if (applied.has(file)) continue;
    const sql = fs.readFileSync(path.join(dir, file), "utf8");
    await applyMigration(file, sql);
    console.log(`applied ${file}`);
  }
  console.log("migrations complete");
}

main().catch(err => {
  console.error(err);
  process.exit(1);
});
