/** Compact age: now, 5m, 3h, 12d, 2y. */
export function shortAgo(iso: string | null): string {
  if (!iso) return ''
  const ms = Date.now() - new Date(iso).getTime()
  const minutes = Math.floor(ms / 60_000)
  if (minutes < 1) return 'now'
  if (minutes < 60) return `${minutes}m`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours}h`
  const days = Math.floor(hours / 24)
  if (days < 365) return `${days}d`
  return `${Math.floor(days / 365)}y`
}

const PROVIDERS: Record<string, string> = {
  greenhouse: 'Greenhouse',
  lever: 'Lever',
  ashby: 'Ashby',
  smartrecruiters: 'SmartRecruiters',
  workable: 'Workable',
  recruitee: 'Recruitee',
  teamtailor: 'Teamtailor',
  phenom: 'Phenom',
  oracle: 'Oracle',
  himalayas: 'Himalayas',
  jobicy: 'Jobicy',
  jobven: 'Jobven',
  jobspipe: 'JobsPipe',
  workday: 'Workday',
}

export const providerLabel = (provider: string) => PROVIDERS[provider] ?? provider
