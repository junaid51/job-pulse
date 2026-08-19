drop index if exists jobs_provider_slug_idx;
alter table jobs drop column if exists slug;
