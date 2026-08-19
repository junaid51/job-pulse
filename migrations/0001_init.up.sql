create table profiles (
    id          bigserial   primary key,
    name        text        not null,
    keywords    text[]      not null default '{}',
    locations   text[]      not null default '{}',
    remote_only boolean     not null default false,
    created_at  timestamptz not null default now()
);

create table companies (
    provider       text not null,
    slug           text not null,
    name           text not null default '',
    last_polled_at timestamptz,
    last_error     text,
    primary key (provider, slug)
);

create table jobs (
    id            bigserial   primary key,
    provider      text        not null,
    external_id   text        not null,
    company       text        not null,
    title         text        not null,
    location      text        not null default '',
    remote        boolean     not null default false,
    url           text        not null,
    posted_at     timestamptz,
    first_seen_at timestamptz not null default now(),
    unique (provider, external_id)
);

create table matches (
    profile_id bigint      not null references profiles (id) on delete cascade,
    job_id     bigint      not null references jobs (id) on delete cascade,
    created_at timestamptz not null default now(),
    seen_at    timestamptz,
    primary key (profile_id, job_id)
);

create table devices (
    token      text        primary key,
    platform   text        not null,
    created_at timestamptz not null default now()
);

create index jobs_first_seen_at_idx on jobs (first_seen_at desc);
create index matches_profile_created_idx on matches (profile_id, created_at desc);
