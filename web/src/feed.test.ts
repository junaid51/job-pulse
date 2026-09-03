import { describe, expect, it } from 'vitest'
import { arrivalLabel, buildItems, jobIdentity, parseQuery, savedPlacesLabel, whereLabel } from './feed'
import type { FeedRow } from './feed'

describe('parseQuery', () => {
  it('separates words, places and exclusions', () => {
    const { words, places, excluded } = parseQuery('  senior engineer @dubai -civil -hvac ')
    expect(words).toEqual(['senior', 'engineer'])
    expect(places).toEqual(['dubai'])
    expect(excluded).toEqual(['-civil', '-hvac'])
  })
  it('ignores bare markers', () => {
    expect(parseQuery('@ - engineer')).toEqual({ words: ['engineer'], places: [], excluded: [] })
  })
  it('is empty for an empty query', () => {
    expect(parseQuery('   ')).toEqual({ words: [], places: [], excluded: [] })
  })
})

describe('arrivalLabel', () => {
  const now = new Date('2026-08-27T12:00:00Z').getTime()
  it('calls the last hour "Just now"', () => {
    expect(arrivalLabel('2026-08-27T11:30:00Z', now)).toBe('Just now')
  })
  it('separates earlier today from yesterday', () => {
    expect(arrivalLabel(new Date(now - 5 * 3600_000).toISOString(), now)).toBe('Earlier today')
    expect(arrivalLabel(new Date(now - 30 * 3600_000).toISOString(), now)).toBe('Yesterday')
  })
  it('names the day for anything older', () => {
    const label = arrivalLabel(new Date(now - 5 * 24 * 3600_000).toISOString(), now)
    expect(label).not.toBe('Yesterday')
    expect(label).toMatch(/\w/)
  })
})

describe('whereLabel', () => {
  it('reads back the region when no place is set', () => {
    expect(whereLabel('', false, true)).toBe('Gulf + India')
    expect(whereLabel('', false, false)).toBe('Anywhere')
  })
  it('prefers a named place, and says when remote is on', () => {
    expect(whereLabel('dubai', false, true)).toBe('dubai')
    expect(whereLabel('  dubai  ', true, true)).toBe('dubai · remote')
    expect(whereLabel('', true, false)).toBe('Anywhere · remote')
  })
})

describe('jobIdentity', () => {
  it('treats the same posting from two boards as one job', () => {
    const a = { title: 'Backend Engineer', company: 'Acme', location: 'Dubai' }
    const b = { title: 'backend engineer', company: 'ACME', location: 'dubai' }
    expect(jobIdentity(a)).toBe(jobIdentity(b))
  })
  it('keeps the same title in two cities apart', () => {
    expect(jobIdentity({ title: 'X', company: 'Y', location: 'Dubai' }))
      .not.toBe(jobIdentity({ title: 'X', company: 'Y', location: 'Riyadh' }))
  })
})

describe('buildItems', () => {
  const now = new Date('2026-08-27T12:00:00Z').getTime()
  const row = (id: number, over: Partial<FeedRow> = {}): FeedRow => ({
    id, title: `Job ${id}`, company: 'Acme', location: 'Dubai',
    matched_at: new Date(now - 10 * 60_000).toISOString(),
    posted_at: null, applied_at: null, ...over,
  })

  it('puts one header above each run of arrivals', () => {
    const items = buildItems([
      row(1),
      row(2),
      row(3, { matched_at: new Date(now - 5 * 3600_000).toISOString() }),
    ], 'matched', now)
    expect(items.map((i) => (i.kind === 'header' ? i.label : 'job')))
      .toEqual(['Just now', 'job', 'job', 'Earlier today', 'job'])
  })

  it('never emits two headers in a row', () => {
    const items = buildItems([row(1), row(2)], 'matched', now)
    const headers = items.filter((i) => i.kind === 'header')
    expect(headers).toHaveLength(1)
  })

  it('collapses the same posting carried by two boards', () => {
    const items = buildItems([
      row(1, { title: 'Backend Engineer' }),
      row(2, { title: 'backend engineer' }),
    ], 'matched', now)
    expect(items.filter((i) => i.kind === 'job')).toHaveLength(1)
  })

  it('keeps the same title in two cities', () => {
    const items = buildItems([
      row(1, { title: 'Backend Engineer', location: 'Dubai' }),
      row(2, { title: 'Backend Engineer', location: 'Riyadh' }),
    ], 'matched', now)
    expect(items.filter((i) => i.kind === 'job')).toHaveLength(2)
  })

  it('ages by the applied date in the applied view', () => {
    const items = buildItems([
      row(1, { applied_at: new Date(now - 30 * 3600_000).toISOString() }),
    ], 'applied', now)
    expect(items[0]).toMatchObject({ kind: 'header', label: 'Yesterday' })
  })

  it('groups undated postings under one header', () => {
    const items = buildItems([
      row(1, { matched_at: '', posted_at: null }),
      row(2, { matched_at: '', posted_at: null, title: 'Other' }),
    ], 'posted', now)
    expect(items.filter((i) => i.kind === 'header')).toEqual([
      { kind: 'header', key: 'h:Undated', label: 'Undated' },
    ])
  })

  it('is empty while rows are still loading', () => {
    expect(buildItems(null, 'matched', now)).toEqual([])
  })
})

// The Where button used to read "Gulf + India" whatever search was selected —
// and that default hid three UK jobs a search had explicitly asked for.
describe('savedPlacesLabel', () => {
  it('names one or two places outright', () => {
    expect(savedPlacesLabel(['gulf'], false)).toBe('gulf')
    expect(savedPlacesLabel(['dubai', 'gulf'], false)).toBe('dubai · gulf')
  })
  it('counts the rest, so the button stays a button', () => {
    expect(savedPlacesLabel(['dubai', 'abu dhabi', 'gulf', 'uae', 'saudi'], false))
      .toBe('dubai +4')
  })
  it('calls a search with no place anywhere, because it is', () => {
    expect(savedPlacesLabel([], false)).toBe('Anywhere')
    expect(savedPlacesLabel(['  '], false)).toBe('Anywhere')
  })
  it('still says when remote-only is on', () => {
    expect(savedPlacesLabel(['gulf'], true)).toBe('gulf · remote')
  })
})
