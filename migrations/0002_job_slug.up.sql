-- Which board a job came from, so a poll can delete what the board no longer
-- lists. Existing rows get '' and are cleaned up by the age sweep instead.
alter table jobs add column slug text not null default '';
create index jobs_provider_slug_idx on jobs (provider, slug);
