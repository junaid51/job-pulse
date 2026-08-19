import { useEffect, useRef, useState } from 'react'
import { api, describeError, type Job, type JobPage, type JobSort } from '../api'
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
        <JobList profileId={profile.id} keywords={profile.keywords} />
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

function JobList({ profileId, keywords }: { profileId: number; keywords: string[] }) {
  const [sort, setSort] = useState<JobSort>('posted')
  const [query, setQuery] = useState('')
  const [debounced, setDebounced] = useState('')
  const [place, setPlace] = useState('')
  const [debouncedPlace, setDebouncedPlace] = useState('')
  const searchRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    const timer = setTimeout(() => setDebouncedPlace(place.trim()), 250)
    return () => clearTimeout(timer)
  }, [place])

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

  // The location box filters every view; "@place" tokens typed into search do
  // the same, through the same alias dictionary the matcher uses.
  const tokens = debounced.split(/\s+/).filter(Boolean)
  const atTokens = tokens.filter((t) => t.startsWith('@')).map((t) => t.slice(1)).filter(Boolean)
  const term = tokens.filter((t) => !t.startsWith('@')).join(' ')
  const locations = debouncedPlace ? [...atTokens, debouncedPlace] : atTokens

  const searching = term !== '' || atTokens.length > 0
  const first = useQuery<JobPage>(
    searching
      ? `jobs:search:${term}@${locations.join(',')}`
      : `jobs:${profileId}:${sort}@${locations.join(',')}`,
    () => (searching ? api.searchJobs(term, locations) : api.jobs(profileId, sort, locations)),
  )

  // Later pages live beside the cached first page and reset whenever it
  // changes — a refetch (pull, refresh button) starts the feed over.
  const [more, setMore] = useState<Job[]>([])
  const [next, setNext] = useState<string | null>(null)
  const [loadingMore, setLoadingMore] = useState(false)
  useEffect(() => {
    setMore([])
    setNext(first.data?.next ?? null)
  }, [first.data])

  const sentinel = useRef<HTMLDivElement>(null)
  useEffect(() => {
    const node = sentinel.current
    if (!node || !next) return
    const observer = new IntersectionObserver((entries) => {
      if (!entries[0].isIntersecting || loadingMore) return
      setLoadingMore(true)
      const fetchPage = searching
        ? api.searchJobs(term, locations, next)
        : api.jobs(profileId, sort, locations, next)
      fetchPage
        .then((page) => { setMore((old) => [...old, ...page.jobs]); setNext(page.next) })
        .catch(() => setNext(null)) // stop asking; pull-to-refresh starts over
        .finally(() => setLoadingMore(false))
    }, { rootMargin: '600px' })
    observer.observe(node)
    return () => observer.disconnect()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [next, loadingMore, searching, debounced, debouncedPlace, profileId, sort])

  // What caused each row: the search term while searching, the profile's
  // positive keywords otherwise. Aliases match server-side without highlight —
  // the dictionary lives in Go, and duplicating it here would drift.
  const highlight = searching
    ? (term ? [term] : [])
    : keywords.filter((keyword) => !keyword.startsWith('-'))

  let list
  if (first.error) {
    list = <ErrorState message={describeError(first.error)} onRetry={first.refetch} />
  } else if (!first.data) {
    list = <SkeletonList />
  } else if (first.data.jobs.length === 0) {
    list = searching || debouncedPlace
      ? <Empty title="Nothing here" detail={debouncedPlace
          ? `No ${searching ? 'results' : 'matches'} in “${debouncedPlace}” — shorthands like uae and uk are understood.`
          : 'Search covers every live job the boards currently list.'} />
      : sort === 'applied'
        ? <Empty title="Nothing marked applied" detail="The check on a job row records where you've applied." />
        : (
          <Empty
            title="Nothing matched yet"
            detail="Try broader keywords in Settings, or refresh — the boards are polled every half hour."
            actionLabel="Refresh"
            onAction={refreshFeeds}
          />
        )
  } else {
    const rows = [...first.data.jobs, ...more]
    list = (
      <>
        <div className="list">
          {rows.map((job) => <JobRow key={job.id} job={job} actions highlight={highlight} />)}
        </div>
        {next && <div ref={sentinel} className="sentinel">{loadingMore ? <span className="spinner" /> : null}</div>}
      </>
    )
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
            placeholder="Search"
            aria-label="Search all jobs"
          />
          {query
            ? <button className="clear" onClick={() => setQuery('')} aria-label="Clear search">✕</button>
            : <kbd>/</kbd>}
        </div>
        <div className="place">
          <PinIcon />
          <input
            value={place}
            onChange={(event) => setPlace(event.target.value)}
            placeholder="Location"
            aria-label="Filter by location"
          />
          {place && (
            <button className="clear" onClick={() => setPlace('')} aria-label="Clear location">✕</button>
          )}
        </div>
        {searching ? null : (
          <div className="segment" role="tablist" aria-label="View">
            <button role="tab" aria-selected={sort === 'posted'}
              className={sort === 'posted' ? 'on' : ''} onClick={() => setSort('posted')}>
              Newest
            </button>
            <button role="tab" aria-selected={sort === 'matched'}
              className={sort === 'matched' ? 'on' : ''} onClick={() => setSort('matched')}>
              Matched
            </button>
            <button role="tab" aria-selected={sort === 'applied'}
              className={sort === 'applied' ? 'on' : ''} onClick={() => setSort('applied')}>
              Applied
            </button>
          </div>
        )}
      </div>
      {list}
    </>
  )
}

function PinIcon() {
  return (
    <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor"
      strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden>
      <path d="M12 21s-7-5.5-7-11a7 7 0 0 1 14 0c0 5.5-7 11-7 11Z" />
      <circle cx="12" cy="10" r="2.6" />
    </svg>
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
