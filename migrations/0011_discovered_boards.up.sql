-- Boards were only ever a projection of companies.txt: the file was written
-- into this table at every boot, and anything the file did not contain was
-- deleted. A board discovered while the app was running could not survive a
-- deploy, so discovery had to end in a pull request somebody remembers to
-- merge — and an unmerged pull request finds nobody a job.
--
-- origin says who owns a row. The file still owns its own and may still delete
-- them; rows discovered at runtime are left alone.
alter table companies add column if not exists origin       text        not null default 'file';
alter table companies add column if not exists active       boolean     not null default true;
alter table companies add column if not exists added_at     timestamptz not null default now();
alter table companies add column if not exists added_reason text        not null default '';

-- Retirement sets active = false rather than deleting: the row is the memory
-- of having tried, and without it the same board is rediscovered next week.
create index if not exists companies_active_idx on companies (active) where active;

-- What companies.txt does in comments — "dubizzle: third sweep, every posting
-- still dated 2023, stop probing it" — a table has to do too, or a tireless
-- scout re-probes the same dead board forever.
create table if not exists board_candidates (
  provider   text not null,
  slug       text not null,
  employer   text not null default '',
  verdict    text not null,
  reason     text not null default '',
  postings   int  not null default 0,
  reachable  int  not null default 0,
  decided_at timestamptz not null default now(),
  primary key (provider, slug)
);
