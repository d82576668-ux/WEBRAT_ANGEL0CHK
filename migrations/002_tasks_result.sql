alter table tasks add column if not exists result jsonb;
create index if not exists idx_tasks_device_id on tasks(device_id);
