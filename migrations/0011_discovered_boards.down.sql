drop table if exists board_candidates;
drop index if exists companies_active_idx;
alter table companies drop column if exists added_reason;
alter table companies drop column if exists added_at;
alter table companies drop column if exists active;
alter table companies drop column if exists origin;
