import { useInfiniteQuery, useQuery } from '@tanstack/react-query'
import { useEffect, useRef, useState } from 'react'
import { api, describeError, type JobSort } from '../api'
import { JobRow } from '../components/JobRow'
import { Empty, ErrorState, Loading, SkeletonList } from '../components/States'
import { invalidate } from '../query'
import { showToast } from '../toast'

async function refreshFeeds() {
  try { await api.poll() } catch { /* the cron's endpoint; refetch regardless */ }
  invalidate('jobs')
  invalidate('notifications')
}

export function Jobs({ goToSettings }: { goToSettings: () => void }) {
  const profiles = useQuery({ queryKey: ['profiles'], queryFn: api.profiles })
  const [selected, setSelected] = useState<number | null>(null)

  let body
  if (profiles.error) {
    body = <ErrorState message={describeError(profiles.error)} onRetry={() => profiles.refetch()} />
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
                {candidate.unread > 0 && <span className="chip-count">{candidate.unread}</span>}
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
    const timer = setTimeout(() => setDebounced(query.trim()), 250)
    return () => clearTimeout(timer)
  }, [query])
  useEffect(() => {
    const timer = setTimeout(() => setDebouncedPlace(place.trim()), 250)
    return () => clearTimeout(timer)
  }, [place])

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

  const feed = useInfiniteQuery({
    queryKey: ['jobs', searching ? 'search' : profileId, searching ? term : sort, locations],
    queryFn: ({ pageParam }) => searching
      ? api.searchJobs(term, locations, pageParam)
      : api.jobs(profileId, sort, locations, pageParam),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (last) => last.next ?? undefined,
  })

  const sentinel = useRef<HTMLDivElement>(null)
  useEffect(() => {
    const node = sentinel.current
    if (!node || !feed.hasNextPage) return
    const observer = new IntersectionObserver((entries) => {
      if (entries[0].isIntersecting && !feed.isFetchingNextPage) feed.fetchNextPage()
    }, { rootMargin: '600px' })
    observer.observe(node)
    return () => observer.disconnect()
  }, [feed.hasNextPage, feed.isFetchingNextPage, feed])

  // What caused each row: the search term while searching, the profile's
  // positive keywords otherwise. Aliases match server-side without highlight —
  // the dictionary lives in Go, and duplicating it here would drift.
  const highlight = searching
    ? (term ? [term] : [])
    : keywords.filter((keyword) => !keyword.startsWith('-'))

  const rows = feed.data?.pages.flatMap((page) => page.jobs) ?? null

  let list
  if (feed.error) {
    list = <ErrorState message={describeError(feed.error)} onRetry={() => feed.refetch()} />
  } else if (!rows) {
    list = <SkeletonList />
  } else if (rows.length === 0) {
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
    list = (
      <>
        <div className="list">
          {rows.map((job) => (
            <JobRow key={job.id} job={job} actions highlight={highlight}
              ageOf={sort === 'applied' && !searching ? 'applied' : 'posted'} />
          ))}
        </div>
        {feed.hasNextPage && (
          <div ref={sentinel} className="sentinel">
            {feed.isFetchingNextPage ? <span className="spinner" /> : null}
          </div>
        )}
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
        {(searching || debouncedPlace) && (
          <button
            className="save-search"
            title="Save this search as a profile"
            onClick={() => {
              api.createProfile({
                name: term || debouncedPlace || 'Search',
                keywords: term ? [term] : [],
                locations,
                remote_only: false,
              }).then(() => {
                invalidate('profiles')
                invalidate('jobs')
                showToast('Saved as a profile — edit it in Settings')
              }).catch(() => showToast('Could not save'))
            }}
          >
            + Save
          </button>
        )}
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
