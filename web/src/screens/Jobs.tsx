import { useEffect, useRef, useState } from 'react'
import { api, describeError, type JobSort } from '../api'
import { JobRow } from '../components/JobRow'
import { Empty, ErrorState, Loading, SkeletonList } from '../components/States'
import { invalidate, useQuery } from '../hooks'

async function refreshFeeds() {
  try { await api.poll() } catch { /* the cron's endpoint; refetch regardless */ }
  invalidate('jobs')
  invalidate('notifications')
}

export function Jobs({ goToSettings }: { goToSettings: () => void }) {
  const profiles = useQuery('profiles', api.profiles)
  const [selected, setSelected] = useState<number | null>(null)

  let body
  if (profiles.error) {
    body = <ErrorState message={describeError(profiles.error)} onRetry={profiles.refetch} />
  } else if (!profiles.data) {
    body = <Loading />
  } else if (profiles.data.length === 0) {
    body = (
      <Empty
        title="No search profiles yet"
        detail="A profile is what jobs get matched against: keywords, locations, remote or not."
        actionLabel="Create a profile"
        onAction={goToSettings}
      />
    )
  } else {
    const list = profiles.data
    const profile = list.find((candidate) => candidate.id === selected) ?? list[0]
    body = (
      <>
        {list.length > 1 && (
          <div className="chips">
            {list.map((candidate) => (
              <button
                key={candidate.id}
                className={`chip ${candidate.id === profile.id ? 'selected' : ''}`}
                onClick={() => setSelected(candidate.id)}
              >
                {candidate.name}
              </button>
            ))}
          </div>
        )}
        <JobList profileId={profile.id} />
      </>
    )
  }

  return (
    <section>
      <header className="bar">
        <h1>Jobs</h1>
        <button className="icon-btn" title="Refresh" onClick={refreshFeeds}>
          <RefreshIcon />
        </button>
      </header>
      {body}
    </section>
  )
}

function JobList({ profileId }: { profileId: number }) {
  const [sort, setSort] = useState<JobSort>('posted')
  const [query, setQuery] = useState('')
  const [debounced, setDebounced] = useState('')
  const searchRef = useRef<HTMLInputElement>(null)

  // Debounce typing so each keystroke does not become a request.
  useEffect(() => {
    const timer = setTimeout(() => setDebounced(query.trim()), 250)
    return () => clearTimeout(timer)
  }, [query])

  // "/" focuses search from anywhere — unless something is already being typed.
  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      const typing = (event.target as HTMLElement)?.tagName === 'INPUT'
      if (event.key === '/' && !typing) {
        event.preventDefault()
        searchRef.current?.focus()
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [])

  const jobs = useQuery(
    `jobs:${profileId}:${sort}:${debounced}`,
    () => api.jobs(profileId, sort, debounced),
  )

  let list
  if (jobs.error) {
    list = <ErrorState message={describeError(jobs.error)} onRetry={jobs.refetch} />
  } else if (!jobs.data) {
    list = <SkeletonList />
  } else if (jobs.data.length === 0) {
    list = debounced
      ? <Empty title={`Nothing for “${debounced}”`} detail="Search covers title, company and location within this profile's matches." />
      : (
        <Empty
          title="Nothing matched yet"
          detail="Try broader keywords in Settings, or refresh — the boards are polled every half hour."
          actionLabel="Refresh"
          onAction={refreshFeeds}
        />
      )
  } else {
    list = <div className="list">{jobs.data.map((job) => <JobRow key={job.id} job={job} />)}</div>
  }

  return (
    <>
      <div className="toolbar">
        <div className="search">
          <SearchIcon />
          <input
            ref={searchRef}
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            placeholder="Search matches"
            aria-label="Search matches"
          />
          {query
            ? <button className="clear" onClick={() => setQuery('')} aria-label="Clear search">✕</button>
            : <kbd>/</kbd>}
        </div>
        <div className="segment" role="tablist" aria-label="Sort">
          <button role="tab" aria-selected={sort === 'posted'}
            className={sort === 'posted' ? 'on' : ''} onClick={() => setSort('posted')}>
            Newest
          </button>
          <button role="tab" aria-selected={sort === 'matched'}
            className={sort === 'matched' ? 'on' : ''} onClick={() => setSort('matched')}>
            Matched
          </button>
        </div>
      </div>
      {list}
    </>
  )
}

function SearchIcon() {
  return (
    <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor"
      strokeWidth="2" strokeLinecap="round" aria-hidden>
      <circle cx="11" cy="11" r="7" /><path d="m20 20-3.8-3.8" />
    </svg>
  )
}

export function RefreshIcon() {
  return (
    <svg width="17" height="17" viewBox="0 0 24 24" fill="none" stroke="currentColor"
      strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden>
      <path d="M21 12a9 9 0 1 1-2.64-6.36M21 3v6h-6" />
    </svg>
  )
}
