-- Delivery tracking for pushes. Without it a notification exists only for the
-- length of one poll cycle: a failed send, or a device inside quiet hours, lost
-- that announcement permanently.
alter table matches add column notified_at timestamptz;

-- Everything already stored counts as announced. Backfilling with created_at
-- rather than leaving nulls is what stops the first deploy from pushing a
-- year of history at the phone.
update matches set notified_at = created_at;

create index if not exists matches_unnotified_idx on matches (profile_id)
  where notified_at is null;
