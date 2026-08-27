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

/** Two boards can carry the same posting; to a reader it is one job. */
export function jobIdentity(job: { title: string; company: string; location: string }): string {
  return `${job.title}|${job.company}|${job.location}`.toLowerCase()
}
