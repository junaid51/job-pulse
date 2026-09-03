-- The whole corpus for the end-to-end suite. Small, fixed, and chosen so that
-- each row is the evidence for one assertion — including the two that used to
-- be bugs: "Indianapolis, Indiana" must never answer a search for India, and
-- "Romania" must never answer one for the Gulf.
delete from matches;
delete from job_state;
delete from jobs;
alter sequence jobs_id_seq restart with 1;

insert into jobs (provider, external_id, company, title, location, remote, url, posted_at, first_seen_at, slug) values
  ('greenhouse', 'e2e-1',  'Tabby',      'Frontend Engineer',            'Dubai, United Arab Emirates', false, 'https://example.test/1',  now() - interval '1 hour',  now() - interval '1 hour',  'tabby'),
  ('greenhouse', 'e2e-2',  'Tamara',     'Senior Frontend Engineer',     'Riyadh, Saudi Arabia',        false, 'https://example.test/2',  now() - interval '2 hours', now() - interval '2 hours', 'tamara'),
  ('lever',      'e2e-3',  'Aldar',      'Backend Engineer',             'Abu Dhabi',                   false, 'https://example.test/3',  now() - interval '3 hours', now() - interval '3 hours', 'aldar'),
  ('ashby',      'e2e-4',  'Ziina',      'Full-Stack Developer',         'Dubai',                       false, 'https://example.test/4',  now() - interval '4 hours', now() - interval '4 hours', 'ziina'),
  ('greenhouse', 'e2e-5',  'GitLab',     'Backend Engineer, EMEA',       'Remote, United Kingdom',      true,  'https://example.test/5',  now() - interval '5 hours', now() - interval '5 hours', 'gitlab'),
  ('lever',      'e2e-6',  'Meesho',     'Frontend Engineer',            'Bengaluru, Karnataka, India', false, 'https://example.test/6',  now() - interval '6 hours', now() - interval '6 hours', 'meesho'),
  -- The word-boundary rows. Both are real locations from the live corpus, and
  -- both used to answer the wrong question.
  ('jobven',     'e2e-7',  'Dominos',    'Frontend Engineer',            'Indianapolis, Indiana',       false, 'https://example.test/7',  now() - interval '7 hours', now() - interval '7 hours', 'dominos'),
  ('jobven',     'e2e-8',  'Mindrift',   'DevOps Engineer',              'Romania',                     false, 'https://example.test/8',  now() - interval '8 hours', now() - interval '8 hours', 'mindrift'),
  -- Substring noise: "oci" inside Associate, "product" inside Production.
  ('workable',   'e2e-9',  'PwC',        'Associate Director, Assurance','Dubai',                       false, 'https://example.test/9',  now() - interval '9 hours', now() - interval '9 hours', 'pwc'),
  ('workable',   'e2e-10', 'Informa',    'Production Editor',            'Dubai',                       false, 'https://example.test/10', now() - interval '10 hours', now() - interval '10 hours', 'informa'),
  -- Plural, which must still match "platform".
  ('greenhouse', 'e2e-11', 'Elastic',    'Engineer, Cloud Platforms',    'Dubai',                       false, 'https://example.test/11', now() - interval '11 hours', now() - interval '11 hours', 'elastic'),
  -- Remote-anywhere, and a job no keyword in these tests will ever catch.
  ('himalayas',  'e2e-12', 'Buffer',     'Platform Engineer',            'Remote, Worldwide',           true,  'https://example.test/12', now() - interval '12 hours', now() - interval '12 hours', 'buffer'),
  ('workable',   'e2e-13', 'Pavago',     'Warehouse Associate',          'Dubai',                       false, 'https://example.test/13', now() - interval '13 hours', now() - interval '13 hours', 'pavago');
