import { describe, expect, it } from 'vitest'
import { providerLabel, shortAgo } from './format'

describe('shortAgo', () => {
  const at = (msAgo: number) => new Date(Date.now() - msAgo).toISOString()
  it('renders compact ages', () => {
    expect(shortAgo(at(10_000))).toBe('now')
    expect(shortAgo(at(5 * 60_000))).toBe('5m')
    expect(shortAgo(at(3 * 3_600_000))).toBe('3h')
    expect(shortAgo(at(12 * 86_400_000))).toBe('12d')
    expect(shortAgo(at(400 * 86_400_000))).toBe('1y')
  })
  it('handles missing dates', () => {
    expect(shortAgo(null)).toBe('')
  })
})

describe('providerLabel', () => {
  it('labels known providers and passes unknown through', () => {
    expect(providerLabel('greenhouse')).toBe('Greenhouse')
    expect(providerLabel('smartrecruiters')).toBe('SmartRecruiters')
    expect(providerLabel('somenewthing')).toBe('somenewthing')
  })
})
