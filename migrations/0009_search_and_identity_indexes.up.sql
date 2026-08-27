-- Search is a word-boundary regex over title, company and location, which no
-- btree can serve: every query read all 5,481 rows. Trigram GIN indexes do
-- serve `~*`, so this is the difference between a scan that is fine today and
-- one that is fine at ten times the corpus.
create extension if not exists pg_trgm;

create index if not exists jobs_title_trgm_idx    on jobs using gin (title gin_trgm_ops);
create index if not exists jobs_company_trgm_idx  on jobs using gin (company gin_trgm_ops);
create index if not exists jobs_location_trgm_idx on jobs using gin (location gin_trgm_ops);

-- The same posting reaching us from two boards is one job to a reader. The
-- insert now checks for that before storing, and this is the index that lookup
-- needs.
create index if not exists jobs_identity_idx
  on jobs (lower(title), lower(company), lower(location));

-- Read nine times in the life of the deployment: both sweeps and the feed order
-- by coalesce(posted_at, first_seen_at), which a single-column index cannot
-- serve. 208 kB of nothing.
drop index if exists jobs_first_seen_at_idx;
