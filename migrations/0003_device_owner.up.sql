-- Identity without accounts: each device generates an anonymous id and
-- everything it creates belongs to that id. Rows from before this migration
-- carry '' and belong to nobody.
alter table profiles add column owner text not null default '';
alter table devices add column owner text not null default '';
create index profiles_owner_idx on profiles (owner);
