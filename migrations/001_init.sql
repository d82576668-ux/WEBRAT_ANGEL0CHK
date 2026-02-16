create extension if not exists pgcrypto;

create table if not exists migrations (
  id serial primary key,
  name text not null unique,
  applied_at timestamptz not null default now()
);

create table if not exists users (
  id uuid primary key default gen_random_uuid(),
  email text unique,
  role text not null default 'admin',
  created_at timestamptz not null default now()
);

create table if not exists devices (
  id uuid primary key default gen_random_uuid(),
  name text not null,
  status text not null default 'offline',
  created_at timestamptz not null default now()
);

create table if not exists tasks (
  id uuid primary key default gen_random_uuid(),
  device_id uuid references devices(id) on delete cascade,
  type text not null,
  status text not null default 'queued',
  payload jsonb,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);
