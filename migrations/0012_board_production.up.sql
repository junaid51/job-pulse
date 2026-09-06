-- Probation asked the wrong question. The rule is "this board has produced
-- nothing", but the query asked whether it holds anything *right now* — and
-- postings age out after a fortnight. A board that delivered in its first week
-- and went quiet was retired exactly as if it had never worked at all, which is
-- wrong for the small employer who posts one role every couple of months.
alter table companies add column if not exists produced_at timestamptz;

-- Anything already holding postings has plainly produced; the rest keep null
-- and have to earn it.
update companies c set produced_at = now()
where exists (select 1 from jobs j where j.provider = c.provider and j.slug = c.slug);
