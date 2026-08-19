-- The two states a real hunt needs on a match, and the one field worth
-- surfacing from boards that publish it.
alter table matches add column hidden_at timestamptz;
alter table matches add column applied_at timestamptz;
alter table jobs add column salary text not null default '';
