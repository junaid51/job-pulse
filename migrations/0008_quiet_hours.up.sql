-- Quiet hours become the device's choice, and the choice starts as "off".
-- The old fixed 22:00-08:00 cost a job hunter exactly the lead they care about:
-- a posting landing at 06:45 was held until 08:00, by which time being first is
-- no longer on offer. Silence should be asked for, not assumed.
--
-- Hours are local to the device's own timezone, already stored alongside.
-- from = to (the default 0/0) means never quiet.
alter table devices
  add column quiet_from smallint not null default 0,
  add column quiet_to   smallint not null default 0;
