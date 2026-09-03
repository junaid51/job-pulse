-- Hiding a job and marking it applied are things a device says about a *job*.
-- They were stored on the match row, so a job that no saved search caught had
-- nowhere to record them: the X and the tick on any search result answered 404,
-- and the row silently did nothing. Widening a saved search made that the
-- common case rather than the rare one.
--
-- This table is the right key: (device, job). What stays on a match is what
-- genuinely belongs to the match — when it was caught, seen and announced.
create table if not exists job_state (
  owner      text        not null,
  job_id     bigint      not null references jobs(id) on delete cascade,
  hidden_at  timestamptz,
  applied_at timestamptz,
  primary key (owner, job_id)
);

-- Nothing is lost: a device's several matches on one job collapse into one row,
-- and the earliest gesture wins, because hiding or applying happened once.
insert into job_state (owner, job_id, hidden_at, applied_at)
select p.owner, m.job_id, min(m.hidden_at), min(m.applied_at)
from matches m
join profiles p on p.id = m.profile_id
where m.hidden_at is not null or m.applied_at is not null
group by p.owner, m.job_id
on conflict (owner, job_id) do nothing;

alter table matches drop column if exists hidden_at;
alter table matches drop column if exists applied_at;
