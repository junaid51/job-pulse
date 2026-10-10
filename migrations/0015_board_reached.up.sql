-- When a discovered board first delivered something this hunt can use: a
-- posting in the market, or a match on a saved search. produced_at asks only
-- whether a board ever stored anything, and a real employer whose every
-- opening is in Bangkok or Prague passes that for ever while contributing
-- nothing. The poll cycle sets this; boards that never earn it are retired.
alter table companies add column if not exists reached_at timestamptz;
