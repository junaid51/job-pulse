import { useInfiniteQuery, useQuery } from '@tanstack/react-query'
import { useVirtualizer } from '@tanstack/react-virtual'
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
  invalidate('profiles') // the chips carry unread counts
}

export function Jobs({ goToSettings }: { goToSettings: () => void }) {
  const profiles = useQuery({ queryKey: ['profiles'], queryFn: api.profiles })
  const [selected, setSelected] = useState<number | null>(null)
  const [refreshing, setRefreshing] = useState(false)

  const refresh = async () => {
    if (refreshing) return
    setRefreshing(true)
    try { await refreshFeeds() } finally { setRefreshing(false) }
    showToast('Up to date')
  }

  // Straight into the editor, not just the Settings screen.
  const newSearch = () => {
    location.hash = '#/settings/new'
    goToSettings()
  }

  let body
  if (profiles.error) {
    body = <ErrorState message={describeError(profiles.error)} onRetry={() => profiles.refetch()} />
  } else if (!profiles.data) {
    body = <Loading />
  } else {
    const list = profiles.data
    const profile = list.find((candidate) => candidate.id === selected) ?? list[0] ?? null
    body = (
      <>
        {list.length > 0 && (
          <div className="chips">
            {list.map((candidate) => (
              <button
                key={candidate.id}
                className={`chip ${candidate.id === profile?.id ? 'selected' : ''}`}
                onClick={() => setSelected(candidate.id)}
              >
                {candidate.name}
                {candidate.unread > 0 && <span className="chip-count">{candidate.unread}</span>}
              </button>
            ))}
            <button className="chip chip-add" onClick={newSearch}>+ New</button>
          </div>
        )}
        <JobList profileId={profile?.id ?? null} keywords={profile?.keywords ?? []}
          profileName={profile?.name ?? ''} onCreateProfile={newSearch}
          onSavedSearch={setSelected} />
      </>
    )
  }

  return (
    <section>
      <header className="bar">
        <h1>Jobs</h1>
        <button className={`icon-btn ${refreshing ? 'spinning' : ''}`} title="Refresh"
          onClick={refresh} disabled={refreshing}>
          <RefreshIcon />
        </button>
      </header>
      {body}
    </section>
  )
}

// profileId is null when no profile exists yet: the feed has nothing to show,
// but the search bar still covers the whole corpus — browsing must not wait
// for a profile to be created.
function JobList({ profileId, keywords, profileName, onCreateProfile, onSavedSearch }: {
  profileId: number | null; keywords: string[]; profileName: string
  onCreateProfile: () => void
  onSavedSearch: (id: number) => void
}) {
  const [sort, setSort] = useState<JobSort>('posted')
  const [remoteOnly, setRemoteOnly] = useState(false)
  const [query, setQuery] = useState('')
  const [debounced, setDebounced] = useState('')
  const [place, setPlace] = useState('')
  const [debouncedPlace, setDebouncedPlace] = useState('')
  const searchRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    const timer = setTimeout(() => setDebounced(query.trim()), 400)
    return () => clearTimeout(timer)
  }, [query])
  useEffect(() => {
    const timer = setTimeout(() => setDebouncedPlace(place.trim()), 400)
    return () => clearTimeout(timer)
  }, [place])

  // "/" focuses search from anywhere — unless something is already being typed.
  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      const typing = (event.target as HTMLElement)?.tagName === 'INPUT'
      // offsetParent is null while the Jobs screen is hidden behind another
      // tab — "/" must not focus an invisible input.
      if (event.key === '/' && !typing && searchRef.current?.offsetParent) {
        event.preventDefault()
        searchRef.current.focus()
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
  // With no profile there is no feed for the location box to filter, so a
  // place on its own also counts as a search of everything.
  const searching = term !== '' || atTokens.length > 0
    || (profileId === null && debouncedPlace !== '')

  const feed = useInfiniteQuery({
    queryKey: ['jobs', searching ? 'search' : 'profile',
      searching ? term : profileId, sort, locations, remoteOnly],
    queryFn: ({ pageParam }) => searching
      ? api.searchJobs(term, locations, remoteOnly, sort, pageParam)
      : api.jobs(profileId!, sort, locations, remoteOnly, pageParam),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (last) => last.next ?? undefined,
    enabled: searching || profileId !== null,
    // While a refined search loads, keep the previous results on screen
    // instead of flashing skeletons on every debounced keystroke.
    placeholderData: (previous) => previous,
  })

  // What caused each row: the search term while searching, the profile's
  // positive keywords otherwise. Aliases match server-side without highlight —
  // the dictionary lives in Go, and duplicating it here would drift.
  const highlight = searching
    ? (term ? [term] : [])
    : keywords.filter((keyword) => !keyword.startsWith('-'))

  const rows = feed.data?.pages.flatMap((page) => page.jobs) ?? null

  // Real windowing: only the rows near the viewport exist in the DOM. The
  // scroller is <main>, so the list's own offset inside it is the margin.
  const listRef = useRef<HTMLDivElement>(null)
  const virtualizer = useVirtualizer({
    count: rows?.length ?? 0,
    getScrollElement: () => listRef.current?.closest('main') ?? null,
    estimateSize: () => 100,
    overscan: 10,
    scrollMargin: listRef.current?.offsetTop ?? 0,
    getItemKey: (index) => rows![index].id,
  })
  const windowed = virtualizer.getVirtualItems()

  // Infinite scroll by index instead of an IntersectionObserver: when the
  // window reaches the last few rows, ask for the next page.
  const lastIndex = windowed[windowed.length - 1]?.index ?? -1
  useEffect(() => {
    // Placeholder rows belong to the previous query — their cursor must not
    // be replayed against the new one.
    if (!rows || feed.isPlaceholderData || lastIndex < rows.length - 8) return
    if (feed.hasNextPage && !feed.isFetchingNextPage) feed.fetchNextPage()
  }, [lastIndex, rows, feed.isPlaceholderData, feed.hasNextPage, feed.isFetchingNextPage, feed])

  let list
  if (feed.error) {
    list = <ErrorState message={describeError(feed.error)} onRetry={() => feed.refetch()} />
  } else if (!searching && profileId === null) {
    list = (
      <Empty
        title="Start with a search"
        detail="The bar above covers every job from every board — type a role and look around. Like the results? Tap Save and they become a standing search that notifies you. Or build one directly:"
        actionLabel="New search"
        onAction={onCreateProfile}
      />
    )
  } else if (!rows) {
    list = <SkeletonList />
  } else if (rows.length === 0) {
    list = sort === 'applied'
      ? <Empty title="Nothing marked applied" detail={searching || debouncedPlace
          ? 'Nothing you have applied to matches this search.'
          : "The check on a job row records where you've applied."} />
      : searching || debouncedPlace
      ? <Empty title="Nothing here" detail={debouncedPlace
          ? `No ${searching ? 'results' : 'matches'} in “${debouncedPlace}” — shorthands like uae and uk are understood.`
          : 'Search covers every live job the boards currently list.'} />
      : sort === 'matched'
        ? <Empty title="Nothing matched yet" detail="Your searches have not caught anything with this filter." />
        : (
          <Empty
            title="Nothing matched yet"
            detail="Try broader keywords in Settings, or refresh — the boards are polled every few minutes."
            actionLabel="Refresh"
            onAction={refreshFeeds}
          />
        )
  } else {
    // The two datasets look identical row by row, so the frame has to say
    // which one is on screen: the profile's matches, or the whole corpus.
    const scope = sort === 'applied'
      ? 'jobs you applied to'
      : searching
        ? (sort === 'matched' ? 'jobs your searches caught' : 'every job from every board')
        : `your “${profileName}” matches`
    list = (
      <>
        {searching && (
          <p className="scope-note">
            {sort === 'applied'
              ? 'Searching the jobs you marked applied.'
              : sort === 'matched'
                ? 'Searching the jobs your saved searches caught.'
                : profileId === null
                  ? 'Searching every job from every board. Tap Save to turn this into a standing search.'
                  : 'Searching every job from every board — not just your matches.'}
          </p>
        )}
        <div ref={listRef} className="list virtual"
          style={{ height: virtualizer.getTotalSize() }}>
          {windowed.map((item) => (
            <div key={item.key} data-index={item.index} ref={virtualizer.measureElement}
              className="vrow"
              style={{ transform: `translateY(${item.start - virtualizer.options.scrollMargin}px)` }}>
              <JobRow job={rows[item.index]} actions highlight={highlight}
                ageOf={sort === 'applied' ? 'applied' : 'posted'} />
            </div>
          ))}
        </div>
        {feed.isFetchingNextPage && (
          <div className="sentinel"><span className="spinner" /></div>
        )}
        {!feed.hasNextPage && !feed.isPlaceholderData && (
          <p className="feed-end">
            That's all — {rows.length} {rows.length === 1 ? 'job' : 'jobs'} in {scope}
          </p>
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
            placeholder="Search all jobs"
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
        <button
          className={`remote-toggle ${remoteOnly ? 'on' : ''}`}
          aria-pressed={remoteOnly}
          onClick={() => setRemoteOnly((v) => !v)}
        >
          Remote
        </button>
        {(searching || debouncedPlace) && (
          <button
            className="save-search"
            title="Save this search as a profile"
            onClick={() => {
              api.createProfile({
                name: term || debouncedPlace || 'Search',
                keywords: term ? [term] : [],
                locations,
                remote_only: remoteOnly,
              }).then((r) => {
                invalidate('profiles')
                invalidate('jobs')
                // Land on the saved search's own feed: the loop closes here.
                onSavedSearch(r.profile.id)
                setQuery('')
                setPlace('')
                showToast('Saved — new matches will notify you. Edit it in Settings.')
              }).catch(() => showToast('Could not save'))
            }}
          >
            + Save
          </button>
        )}
        {/* The views stay put while searching: a control that vanishes as you
            type takes your place in the app with it. */}
        {profileId === null && !searching ? null : (
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
