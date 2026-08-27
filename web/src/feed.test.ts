import { describe, expect, it } from 'vitest'
import { arrivalLabel, jobIdentity, parseQuery, whereLabel } from './feed'

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
