import { useInfiniteQuery, useQuery } from '@tanstack/react-query'
import { useVirtualizer } from '@tanstack/react-virtual'
import { useEffect, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { api, describeError, type JobSort, type Profile } from '../api'
import { JobRow } from '../components/JobRow'
import { Empty, ErrorState, Loading, SkeletonList } from '../components/States'
import { invalidate } from '../query'
import { buildItems, parseQuery, savedPlacesLabel, whereLabel } from '../feed'
import { showToast } from '../toast'
import { useEscape } from '../useEscape'

async function refreshFeeds() {
  try { await api.poll() } catch { /* the cron's endpoint; refetch regardless */ }
  invalidate('jobs')
  invalidate('profiles') // the chips carry unread counts
}

/** What the feed is showing: one saved search, every saved search, the jobs
 *  you applied to, or — before any search exists — nothing. One question with
 *  one row of answers, which is why these are chips and not chips plus tabs. */
type Scope = number | 'all' | 'applied' | null

export function Jobs({ goToSettings }: { goToSettings: () => void }) {
  const profiles = useQuery({ queryKey: ['profiles'], queryFn: api.profiles })
  // A poller that has stopped is the one failure that makes this whole app a
  // lie: the list still looks like a list. Ask the backend, and say so.
  const health = useQuery({
    queryKey: ['health'], queryFn: api.health, refetchInterval: 60_000,
  })
  const [selected, setSelected] = useState<Scope>('all')
  const [refreshing, setRefreshing] = useState(false)
  // A typed term searches every board, so no saved search is what you are
  // looking at — nothing in the row should claim to be selected.
  const [searching, setSearching] = useState(false)

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
    // One saved search means the "all searches" chip would be a synonym for it.
    const showAll = list.length > 1
    const scope: Scope = list.length === 0 ? null
      : selected === 'applied' ? 'applied'
      : typeof selected === 'number' && list.some((p) => p.id === selected) ? selected
      : showAll ? 'all' : list[0].id
    const unread = list.reduce((n, profile) => n + profile.unread, 0)
    body = (
      <>
        {list.length > 0 && (
          <div className="chips">
            {showAll && (
              <button className={`chip ${!searching && scope === 'all' ? 'selected' : ''}`}
                onClick={() => setSelected('all')}>
                All searches
                {unread > 0 && <span className="chip-count">{unread}</span>}
              </button>
            )}
            {list.map((candidate) => (
              <button
                key={candidate.id}
                className={`chip ${!searching && candidate.id === scope ? 'selected' : ''}`}
                onClick={() => setSelected(candidate.id)}
              >
                {candidate.name}
                {candidate.unread > 0 && <span className="chip-count">{candidate.unread}</span>}
              </button>
            ))}
            <button className={`chip ${scope === 'applied' ? 'selected' : ''}`}
              onClick={() => setSelected('applied')}>Applied</button>
            <button className="chip chip-add" onClick={newSearch}>+ New</button>
          </div>
        )}
        <JobList scope={scope} profiles={list} onCreateProfile={newSearch}
          onSavedSearch={setSelected} onSearching={setSearching} />
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
      <StaleNotice health={health.data} />
      {body}
    </section>
  )
}

/** Any request to the backend revives a stalled poller, so by the time this
 *  renders a catch-up run is already going. It still has to be said: the rows
 *  above it are as old as the last cycle. */
function StaleNotice({ health }: { health?: { poller: string; poll_age_seconds?: number | null; poll_error?: string } }) {
  if (!health || health.poller === 'ok') return null
  const age = health.poll_age_seconds
  const since = age == null ? null
    : age < 3600 ? `${Math.round(age / 60)}m`
    : age < 86400 ? `${Math.round(age / 3600)}h`
    : `${Math.round(age / 86400)}d`
  return (
    <p className="notice">
      {health.poller === 'failing'
        ? `The last poll failed${health.poll_error ? `: ${health.poll_error}` : ''}. Retrying.`
        : since
          ? `Boards last checked ${since} ago — catching up now.`
          : 'The boards have not been checked yet — starting now.'}
    </p>
  )
}

// scope is null when no saved search exists yet: the feed has nothing to show,
// but the search bar still covers the whole corpus — browsing must not wait for
// a search to be saved.
function JobList({ scope, profiles, onCreateProfile, onSavedSearch, onSearching }: {
  scope: Scope
  profiles: Profile[]
  onCreateProfile: () => void
  onSavedSearch: (id: number) => void
  onSearching: (active: boolean) => void
}) {
  // Arrival order, not publication order: a job discovered ten minutes ago but
  // posted last week is news to this reader. The Applied chip is the one view
  // that orders by something else — when you applied.
  const sort: JobSort = scope === 'applied' ? 'applied' : 'matched'
  const [remoteOnly, setRemoteOnly] = useState(false)
  // Seven jobs in ten in the corpus are restricted somewhere this reader
  // cannot work: a company board is chosen whole, and brings its Ohio roles
  // along with its Dubai one. On by default, one tap to see everything.
  const [myMarkets, setMyMarkets] = useState(true)
  // Until the reader touches Where, a saved search's feed uses the places the
  // search itself was saved with. Selecting another chip hands the question
  // back to that search.
  const [whereTouched, setWhereTouched] = useState(false)
  // Kept apart from whereTouched: flipping the remote switch used to hand Where
  // to the Gulf + India default, so asking for remote roles inside a saved
  // search quietly threw away the places that search watches.
  const [remoteTouched, setRemoteTouched] = useState(false)
  const [whereOpen, setWhereOpen] = useState(false)
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

  const current = typeof scope === 'number' ? profiles.find((p) => p.id === scope) : undefined

  // "@place" typed into search filters location the same way the Where sheet
  // does, through the same alias dictionary the matcher uses.
  // "-word" rules a posting out, the same way a saved search's exclusions do,
  // and "@place" filters location. The whole query goes to the backend; only
  // the positive words are worth underlining in a title. Split by the same
  // rules the backend uses — see feed.ts.
  const { words, places: atTokens, excluded } = parseQuery(debounced)
  const term = [...words, ...excluded].join(' ')
  const rawLocations = debouncedPlace ? [...atTokens, debouncedPlace] : atTokens
  // A typed term searches every board, not just what your searches caught —
  // hiding jobs because they missed your keywords answers the wrong question.
  const searching = words.length > 0 || atTokens.length > 0 || excluded.length > 0
  // A named place is the answer to "where", so it replaces the region filter
  // rather than intersecting with it — otherwise asking for London while the
  // region said Gulf returned nothing, silently.
  // The places a saved search was saved with are part of the search. Until the
  // reader says otherwise, they are the answer to "where" — a global default
  // has no business overruling them, and it was: a search that asked for the UK
  // matched three UK jobs and the Gulf + India default hid all three.
  const savedPlaces = current?.locations ?? []
  // Applied is a record of what this reader did, not a slice of the corpus, so
  // a global region default has no business filtering it — and it did: mark a
  // job in London applied and the Applied chip showed nothing at all.
  const appliedScope = scope === 'applied'
  const scopeOwnsWhere = !searching && !whereTouched
    && (typeof scope === 'number' || appliedScope)
  const market = scopeOwnsWhere ? false : myMarkets && !debouncedPlace
  // A search saved as remote-only matched only remote roles, so a switch
  // reading "off" above that feed is the same lie in miniature. It stays the
  // search's answer until this reader gives their own.
  const remote = !searching && !remoteTouched && current?.remote_only
    ? true : remoteOnly

  // A saved search applies its own places when a job is matched, so its match
  // list holds nothing outside them and Where has nothing wider to reveal —
  // picking Anywhere on a Gulf search changed precisely nothing, which is
  // indefensible for a control that says "Anywhere". Once Where is touched, the
  // feed stops asking the match list and asks the corpus for the same keywords.
  // The search's own places still decide what it notifies about; they no longer
  // decide what you are allowed to look at.
  const locations = scopeOwnsWhere ? atTokens : rawLocations
  // Widening asks the corpus for the scope's keywords, which is meaningless for
  // Applied: those rows are the reader's own actions, and a job they applied to
  // outside their keywords would have vanished from the list of them.
  const widening = !searching && !appliedScope && scope !== null && whereTouched
    && (!myMarkets || debouncedPlace !== '')
  const scopeKeywords = typeof scope === 'number'
    ? current?.keywords ?? []
    : profiles.flatMap((p) => p.keywords)
  const anyOf = widening ? scopeKeywords.filter((k) => !k.startsWith('-')) : []
  const scopeExcluded = widening ? scopeKeywords.filter((k) => k.startsWith('-')) : []
  const widened = widening && anyOf.length > 0

  // Switching chips hands "where" back to the search that owns it.
  useEffect(() => { setWhereTouched(false); setRemoteTouched(false) }, [scope])
  useEffect(() => { onSearching(searching) }, [searching, onSearching])

  const feed = useInfiniteQuery({
    queryKey: ['jobs', searching ? 'corpus' : widened ? `wide:${scope}` : scope,
      term, sort, locations, remote, market, anyOf],
    queryFn: ({ pageParam }) => api.feed({
      // Applied spans every saved search; a typed term spans every board; a
      // widened saved search spans every board for its own keywords.
      scope: searching || widened ? 'corpus' : scope === 'applied' ? 'all' : scope!,
      q: [term, ...scopeExcluded].filter(Boolean).join(' '),
      keywords: anyOf,
      locations, remote, market, sort, cursor: pageParam,
    }),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (last) => last.next ?? undefined,
    enabled: searching || scope !== null,
    // Keep the previous results on screen while a typed term is refined —
    // otherwise every debounced keystroke flashes skeletons. But only for the
    // same scope and sort: rows from another chip, grouped by this chip's
    // clock, stacked a stale "Undated" header on top of the live one.
    placeholderData: (previous, previousQuery) => {
      const before = previousQuery?.queryKey as unknown[] | undefined
      if (!before) return previous
      const sameScope = before[1] === (searching ? 'corpus' : scope)
      return sameScope && before[3] === sort ? previous : undefined
    },
  })

  const rows = feed.data?.pages.flatMap((page) => page.jobs) ?? null

  // Looking at the arrivals is what makes them not new any more: the badge
  // clears, the dots stay for this viewing.
  const marked = useRef(false)
  useEffect(() => {
    if (marked.current || searching || sort !== 'matched') return
    if (!rows?.some((job) => !job.seen_at)) return
    marked.current = true
    api.markSeen().then(() => invalidate('profiles')).catch(() => { marked.current = false })
  }, [rows, searching, sort])

  // What caused each row: the typed term while searching, otherwise the
  // keywords of the searches in scope. Aliases match server-side without
  // highlight — the dictionary lives in Go, and duplicating it here would drift.
  const highlight = searching
    ? words
    : (current ? current.keywords : profiles.flatMap((p) => p.keywords))
      .filter((keyword) => !keyword.startsWith('-'))

  // Headers ride in the same virtual list as the rows, so grouping costs no
  // scrolling performance. The building of it lives in feed.ts, where it is
  // tested: this is where a stale header once stacked on a live one.
  const items = buildItems(rows, sort === 'applied' ? 'applied' : sort === 'matched' ? 'matched' : 'posted')

  // Real windowing: only the rows near the viewport exist in the DOM. The
  // scroller is <main>, so the list's own offset inside it is the margin.
  const listRef = useRef<HTMLDivElement>(null)
  const virtualizer = useVirtualizer({
    count: items.length,
    getScrollElement: () => listRef.current?.closest('main') ?? null,
    estimateSize: () => 100,
    overscan: 10,
    scrollMargin: listRef.current?.offsetTop ?? 0,
    getItemKey: (index) => items[index].key,
  })
  const windowed = virtualizer.getVirtualItems()

  // Infinite scroll by index instead of an IntersectionObserver: when the
  // window reaches the last few rows, ask for the next page.
  const lastIndex = windowed[windowed.length - 1]?.index ?? -1
  useEffect(() => {
    // Placeholder rows belong to the previous query — their cursor must not
    // be replayed against the new one.
    if (!rows || feed.isPlaceholderData || lastIndex < items.length - 8) return
    if (feed.hasNextPage && !feed.isFetchingNextPage) feed.fetchNextPage()
  }, [lastIndex, rows, feed.isPlaceholderData, feed.hasNextPage, feed.isFetchingNextPage, feed])

  // The rows look identical whichever feed they came from, so the frame says
  // which one is on screen.
  const scopeName = sort === 'applied' ? 'jobs you applied to'
    : searching ? 'every job from every board'
    : widened ? `every job matching “${current ? current.name : 'your searches'}”`
    : current ? `your “${current.name}” matches`
    : 'jobs your searches caught'

  // Everywhere this feed says "nothing matched", it has to name every filter it
  // applied. An "@dubai" token used to vanish from the message, so a search for
  // "react @dubai" reported "nothing matches react" — true of the words, and
  // silent about the place that did most of the excluding.
  const placesInPlay = [...new Set([...atTokens, debouncedPlace].filter(Boolean))]
  const inPlaces = placesInPlay.length ? ` in ${placesInPlay.join(' or ')}` : ''

  let list
  if (feed.error) {
    list = <ErrorState message={describeError(feed.error)} onRetry={() => feed.refetch()} />
  } else if (!searching && scope === null) {
    list = (
      <Empty
        title="Start with a search"
        detail="The bar above covers every job from every board. Type a role, look around, and tap Save to be notified when new ones land."
        actionLabel="New search"
        onAction={onCreateProfile}
      />
    )
  } else if (!rows) {
    list = <SkeletonList />
  } else if (rows.length === 0) {
    list = sort === 'applied'
      ? <Empty title="Nothing marked applied" detail={searching
          ? 'Nothing you have applied to matches this search.'
          : "The check on a job row records where you've applied."} />
      : searching
        ? words.length > 1
          ? (
            <Empty
              title="No job matches all of those words"
              detail={`No job has all of “${words.join(' ')}”${inPlaces} across its title, company and location. Every word has to match, so the more you type the narrower it gets.`}
              actionLabel={`Search “${words.slice(0, -1).join(' ')}” instead`}
              onAction={() => setQuery(words.slice(0, -1).join(' '))}
            />
          )
          : (
            <Empty
              title={words.length
                ? `Nothing matches “${words.join(' ')}”${inPlaces} yet`
                : `Nothing in ${placesInPlay.join(' or ') || 'that place'} yet`}
              detail={placesInPlay.length && !words.length
                ? 'Shorthands like uae, ksa and uk are understood.'
                : `Titles, companies and locations are searched — not job descriptions, which this app deliberately does not store.${placesInPlay.length ? ' Try without the place.' : ''}`}
              actionLabel={placesInPlay.length && words.length ? `Search “${words.join(' ')}” anywhere` : undefined}
              onAction={placesInPlay.length && words.length
                ? () => { setQuery(words.join(' ')); setPlace('') }
                : undefined}
            />
          )
        : (
          <Empty
            title="Nothing matched yet"
            detail="Try broader keywords in Settings, or widen Where — the boards are polled every few minutes."
            actionLabel="Refresh"
            onAction={refreshFeeds}
          />
        )
  } else {
    list = (
      <>
        <div ref={listRef} key={`${scope}:${sort}`} className="list virtual"
          style={{ height: virtualizer.getTotalSize() }}>
          {windowed.map((virtual) => {
            const item = items[virtual.index]
            return (
              <div key={virtual.key} data-index={virtual.index} ref={virtualizer.measureElement}
                className="vrow"
                style={{ transform: `translateY(${virtual.start - virtualizer.options.scrollMargin}px)` }}>
                {item.kind === 'header'
                  ? <div className="day-h">{item.label}</div>
                  : <JobRow job={item.job} actions highlight={highlight}
                      // Across several searches the row says which one caught
                      // it; inside one search that is already the answer.
                      via={scope !== null && scope !== 'applied' && !searching
                        ? item.job.matched_by : undefined}
                      showUnread={scope === 'all' && !searching}
                      ageOf={sort === 'applied' ? 'applied' : sort === 'matched' ? 'matched' : 'posted'} />}
              </div>
            )
          })}
        </div>
        {feed.isFetchingNextPage && (
          <div className="sentinel"><span className="spinner" /></div>
        )}
        {!feed.hasNextPage && !feed.isPlaceholderData && (
          <p className="feed-end">
            That's all — {rows.length} {rows.length === 1 ? 'job' : 'jobs'} in {scopeName}
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
            title="Every word must match. -word rules a job out, @place filters by location."
          />
          {query
            ? <button className="clear" onClick={() => setQuery('')} aria-label="Clear search">✕</button>
            : <kbd>/</kbd>}
        </div>

        <button className="where-btn" onClick={() => setWhereOpen(true)}>
          <PinIcon />
          <span>{scopeOwnsWhere
            ? savedPlacesLabel(savedPlaces, remote)
            : whereLabel(place, remote, myMarkets)}</span>
        </button>

        {searching && (
          <button
            className="save-search"
            title="Save this search"
            onClick={() => {
              api.createProfile({
                name: words.join(' ') || debouncedPlace || 'Search',
                keywords: [...words, ...excluded],
                locations,
                remote_only: remoteOnly,
              }).then((r) => {
                invalidate('profiles')
                invalidate('jobs')
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
      </div>

      {widened && (
        <p className="notice quiet">
          Showing every job for {current ? `“${current.name}”` : 'your searches'} —
          its own places only decide what it notifies you about.
        </p>
      )}

      {whereOpen && (
        <WhereSheet
          place={place} setPlace={(v) => { setWhereTouched(true); setPlace(v) }}
          remoteOnly={remote} setRemoteOnly={(v) => { setRemoteTouched(true); setRemoteOnly(v) }}
          myMarkets={myMarkets} setMyMarkets={(v) => { setWhereTouched(true); setMyMarkets(v) }}
          scopeOwns={scopeOwnsWhere}
          savedPlaces={scopeOwnsWhere ? savedPlaces : []}
          savedName={appliedScope ? undefined : current?.name}
          onClose={() => setWhereOpen(false)}
        />
      )}
      {list}
    </>
  )
}

// The two regions worth a default, then the places actually searched from here.
const WHERE_PLACES = ['dubai', 'abu dhabi', 'saudi', 'qatar', 'kuwait', 'bahrain',
  'oman', 'egypt', 'india', 'uk', 'usa']

/** Where is one question, so it is one list: a region, or a place.
 *
 *  Portalled to the document body, like every overlay here: rendered in place
 *  it sits inside the screen wrapper, whose fade-in animation makes it a
 *  stacking context — and a fixed overlay trapped in one paints *under* the
 *  tab bar, which swallowed this sheet's own Done button. */
function WhereSheet(props: {
  place: string; setPlace: (v: string) => void
  remoteOnly: boolean; setRemoteOnly: (v: boolean) => void
  myMarkets: boolean; setMyMarkets: (v: boolean) => void
  /** True while the chip on screen answers "where" itself, so no region here
   *  is in force and none of them may look selected. */
  scopeOwns: boolean
  savedPlaces: string[]
  savedName?: string
  onClose: () => void
}) {
  const [draft, setDraft] = useState(
    WHERE_PLACES.includes(props.place) ? '' : props.place)
  useEscape(props.onClose)
  const region = (markets: boolean) => {
    props.setPlace('')
    props.setMyMarkets(markets)
    setDraft('')
  }
  return createPortal(
    <div className="sheet-backdrop" onClick={props.onClose}>
      <div className="sheet" onClick={(e) => e.stopPropagation()}>
        <h2>Where</h2>
        {props.scopeOwns && (
          <small>
            {props.savedName
              ? <>“{props.savedName}” watches {props.savedPlaces.length
                  ? props.savedPlaces.join(', ') : 'anywhere'}. Choosing here
                  looks beyond it — the search keeps watching its own places.</>
              : <>Everything you applied to, wherever it was. Choosing here
                  narrows the list.</>}
          </small>
        )}

        <div className="sheet-body">
        <div className="picker">
          <button type="button"
            className={`pick ${!props.scopeOwns && !props.place && props.myMarkets ? 'on' : ''}`}
            onClick={() => region(true)}>Gulf + India</button>
          <button type="button"
            className={`pick ${!props.scopeOwns && !props.place && !props.myMarkets ? 'on' : ''}`}
            onClick={() => region(false)}>Anywhere</button>
          {WHERE_PLACES.map((name) => (
            <button type="button" key={name}
              className={`pick ${props.place === name ? 'on' : ''}`}
              onClick={() => { setDraft(''); props.setPlace(props.place === name ? '' : name) }}>
              {name}
            </button>
          ))}
          <input
            className="pick-input"
            type="text"
            value={draft}
            placeholder="or type a city…"
            autoCapitalize="none" autoCorrect="off" spellCheck={false}
            enterKeyHint="done"
            onChange={(e) => { setDraft(e.target.value); props.setPlace(e.target.value.trim()) }}
            onKeyDown={(e) => { if (e.key === 'Enter') { e.preventDefault(); props.onClose() } }}
          />
        </div>
        <small>
          Gulf + India also keeps what you could take from here: roles listed
          worldwide, remote-anywhere, EMEA or Middle East. A named place answers
          on its own — shorthands like uae, ksa and uk are understood.
        </small>

        <label className="switch-row">
          <span>Remote roles only</span>
          <input type="checkbox" checked={props.remoteOnly}
            onChange={(e) => props.setRemoteOnly(e.target.checked)} />
        </label>
        </div>

        <button className="btn-filled wide" onClick={props.onClose}>Done</button>
      </div>
    </div>,
    document.body,
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

function RefreshIcon() {
  return (
    <svg width="17" height="17" viewBox="0 0 24 24" fill="none" stroke="currentColor"
      strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden>
      <path d="M21 12a9 9 0 1 1-2.64-6.36M21 3v6h-6" />
    </svg>
  )
}
