-- When a push last actually reached this device. Without it, "notifications
-- aren't arriving" is unfalsifiable: the app can only report what the browser
-- thinks, and a token that expired months ago looks exactly like a quiet week.
alter table devices add column last_notified_at timestamptz;
