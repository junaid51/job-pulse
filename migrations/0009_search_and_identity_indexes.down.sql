create index if not exists jobs_first_seen_at_idx on jobs (first_seen_at desc);
drop index if exists jobs_identity_idx;
drop index if exists jobs_location_trgm_idx;
drop index if exists jobs_company_trgm_idx;
drop index if exists jobs_title_trgm_idx;
