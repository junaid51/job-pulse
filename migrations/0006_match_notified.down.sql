drop index if exists matches_unnotified_idx;
alter table matches drop column if exists notified_at;
