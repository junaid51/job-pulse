drop index if exists profiles_owner_idx;
alter table profiles drop column if exists owner;
alter table devices drop column if exists owner;
