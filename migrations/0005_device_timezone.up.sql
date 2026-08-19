-- Where the device lives, so a 3am match waits for morning.
alter table devices add column timezone text not null default '';
