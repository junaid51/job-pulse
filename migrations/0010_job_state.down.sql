alter table matches add column if not exists hidden_at timestamptz;
alter table matches add column if not exists applied_at timestamptz;

update matches m set hidden_at = s.hidden_at, applied_at = s.applied_at
from job_state s, profiles p
where p.id = m.profile_id and s.owner = p.owner and s.job_id = m.job_id;

drop table if exists job_state;
