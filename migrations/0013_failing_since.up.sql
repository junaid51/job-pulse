-- A board that dies goes quiet, not loud. It answers 404 once a cycle, its
-- postings age out over a fortnight, and the notifications it would have sent
-- simply never arrive. ClickHouse and PhonePe both died this week — PhonePe was
-- 64 postings with 52 in reachable markets — and the only reason either was
-- noticed is that somebody read the poll logs by hand.
--
-- last_error alone cannot tell a dead board from a blip: a Supabase connect
-- wobble and a renamed slug look identical in it. This is the column that can.
alter table companies add column if not exists failing_since timestamptz;

-- Anything failing right now has been failing for at least as long as its last
-- attempt; claiming otherwise would date the problem to this migration.
update companies set failing_since = last_polled_at
where coalesce(last_error, '') <> '' and failing_since is null;
