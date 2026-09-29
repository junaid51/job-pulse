-- Employers the scout searched for and found no board. Without this the work
-- list is the same every day: it ranks by matches, the top of it is whoever
-- cannot be found (Amazon hires through its own site), and a daily agent spends
-- every run re-searching them while new employers never reach the front.
-- A miss rests an employer for a while rather than forever: boards appear.
create table if not exists scout_misses (
  employer text        primary key,  -- lower-cased, as the warm list compares
  tried_at timestamptz not null default now(),
  reason   text        not null default ''
);
