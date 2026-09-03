// The feed's pure decisions, out of the component so they can be tested: how a
// typed query splits, what a row's arrival is called, what "where" reads as,
// and when two postings are the same job.

/** A typed query, split the way the backend splits it. Keep the two in step:
 *  `-word` excludes, `@place` filters location, everything else must match. */
export function parseQuery(raw: string): {
  words: string[]
  places: string[]
  excluded: string[]
} {
  const words: string[] = []
  const places: string[] = []
  const excluded: string[] = []
  for (const token of raw.trim().split(/\s+/).filter(Boolean)) {
    if (token.startsWith('@')) {
      const place = token.slice(1)
      if (place) places.push(place)
    } else if (token.startsWith('-')) {
      const word = token.slice(1)
      if (word) excluded.push(token)
    } else {
      words.push(token)
    }
  }
  return { words, places, excluded }
}

/** The feed is a timeline of arrivals, so a group header says when — and the
 *  row beneath it is then free to be just the job. */
export function arrivalLabel(iso: string, now = Date.now()): string {
  const at = new Date(iso)
  if (now - at.getTime() < 60 * 60 * 1000) return 'Just now'
  const today = new Date(now)
  const yesterday = new Date(now - 24 * 60 * 60 * 1000)
  const sameDay = (a: Date, b: Date) => a.toDateString() === b.toDateString()
  if (sameDay(at, today)) return 'Earlier today'
  if (sameDay(at, yesterday)) return 'Yesterday'
  return at.toLocaleDateString(undefined, { weekday: 'short', month: 'short', day: 'numeric' })
}

/** One phrase for whatever "where" currently means, shown on the button so the
 *  filter never hides anything silently. */
export function whereLabel(place: string, remoteOnly: boolean, myMarkets: boolean): string {
  const where = place.trim() || (myMarkets ? 'Gulf + India' : 'Anywhere')
  return remoteOnly ? `${where} · remote` : where
}

/** What a saved search's own places read as on the Where button.
 *
 *  Two names fit; beyond that it becomes a count, because the button has to
 *  stay a button. Empty means the search never named a place, which is
 *  genuinely anywhere. */
export function savedPlacesLabel(places: string[], remoteOnly: boolean): string {
  const named = places.map((p) => p.trim()).filter(Boolean)
  const where = named.length === 0 ? 'Anywhere'
    : named.length <= 2 ? named.join(' · ')
    : `${named[0]} +${named.length - 1}`
  return remoteOnly ? `${where} · remote` : where
}

/** Two boards can carry the same posting; to a reader it is one job. */
export function jobIdentity(job: { title: string; company: string; location: string }): string {
  return `${job.title}|${job.company}|${job.location}`.toLowerCase()
}

export type FeedRow = {
  id: number
  title: string
  company: string
  location: string
  matched_at: string
  posted_at: string | null
  applied_at: string | null
}

export type Item<T> =
  | { kind: 'header'; key: string; label: string }
  | { kind: 'job'; key: string; job: T }

/** The rows of the feed as the virtualized list consumes them: arrival headers
 *  interleaved with jobs, and the same posting from two boards collapsed to one.
 *  Kept out of the component because this is where its bugs have been — a stale
 *  header stacked on a live one, a duplicate a page apart. */
export function buildItems<T extends FeedRow>(
  rows: T[] | null,
  sort: 'matched' | 'applied' | 'posted',
  now = Date.now(),
): Item<T>[] {
  if (!rows) return []
  const items: Item<T>[] = []
  const seen = new Set<string>()
  let group = ''
  for (const job of rows) {
    const identity = jobIdentity(job)
    if (seen.has(identity)) continue
    seen.add(identity)
    const when = sort === 'applied' ? job.applied_at
      : sort === 'matched' ? job.matched_at
      : job.posted_at ?? job.matched_at
    const label = when ? arrivalLabel(when, now) : 'Undated'
    if (label !== group) {
      group = label
      items.push({ kind: 'header', key: `h:${label}`, label })
    }
    items.push({ kind: 'job', key: `j:${job.id}`, job })
  }
  return items
}
