// One function per endpoint, returning typed models. Errors propagate; the
// screens render them via describeError.

export interface Job {
  id: number
  provider: string
  company: string
  title: string
  location: string
  remote: boolean
  url: string
  salary: string
  applied: boolean
  applied_at: string | null
  posted_at: string | null
  matched_at: string
  seen_at: string | null
}

export interface Profile {
  id: number
  name: string
  keywords: string[]
  locations: string[]
  remote_only: boolean
  unread: number
}

export interface MatchEvent {
  profile_id: number
  profile_name: string
  job: Job
}

export type JobSort = 'posted' | 'matched' | 'applied'

export interface JobPage {
  jobs: Job[]
  next: string | null
}

export interface Board {
  provider: string
  slug: string
  name: string
  jobs: number
  last_polled_at: string | null
  last_error: string | null
}

export interface ProfileInput {
  name: string
  keywords: string[]
  locations: string[]
  remote_only: boolean
}

// Baked in at build time, like every deployment constant in this app.
export const API = import.meta.env.VITE_JOBPULSE_API ?? 'http://localhost:8091'

// Identity without accounts: a UUID minted once per browser install. Everything
// this device creates on the backend belongs to this id and is invisible to
// other devices — profiles, matches, notifications and push routing all follow
// it. Clearing site data mints a new identity (and orphans the old profiles).
export const deviceId: string = (() => {
  const KEY = 'jobpulse-device'
  let id = localStorage.getItem(KEY)
  if (!id) {
    id = crypto.randomUUID()
    localStorage.setItem(KEY, id)
  }
  return id
})()

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(API + path, {
    ...init,
    headers: {
      'X-Device': deviceId,
      ...(init?.body ? { 'Content-Type': 'application/json' } : {}),
    },
  })
  if (!response.ok) {
    let message = `The backend answered ${response.status}.`
    try {
      const body = await response.json()
      if (body.error) message = body.error
    } catch { /* not JSON; keep the status message */ }
    throw new Error(message)
  }
  // DELETE and some POSTs answer 204 with no body.
  if (response.status === 204) return undefined as T
  return response.json()
}

export const api = {
  profiles: () =>
    request<{ profiles: Profile[] }>('/api/profiles').then((r) => r.profiles ?? []),

  createProfile: (input: ProfileInput) =>
    request<{ profile: Profile }>('/api/profiles', {
      method: 'POST', body: JSON.stringify(input),
    }),

  updateProfile: (id: number, input: ProfileInput) =>
    request<{ profile: Profile }>(`/api/profiles/${id}`, {
      method: 'PUT', body: JSON.stringify(input),
    }),

  deleteProfile: (id: number) =>
    request<void>(`/api/profiles/${id}`, { method: 'DELETE' }),

  jobs: (profileId: number, sort: JobSort = 'posted', locations: string[] = [],
    remote = false, cursor?: string) =>
    request<{ jobs: Job[]; next_cursor?: string }>(
      `/api/jobs?profile_id=${profileId}&limit=50&sort=${sort}` +
      locations.map((l) => `&location=${encodeURIComponent(l)}`).join('') +
      (remote ? '&remote=1' : '') +
      (cursor ? `&cursor=${encodeURIComponent(cursor)}` : ''),
    ).then((r) => ({ jobs: r.jobs ?? [], next: r.next_cursor ?? null })),

  /** Searches every stored job, not just one profile's matches — a search bar
   *  that hides jobs because they missed your keywords answers the wrong
   *  question. */
  searchJobs: (q: string, locations: string[] = [], remote = false,
    sort: JobSort = 'posted', cursor?: string) =>
    request<{ jobs: Job[]; next_cursor?: string }>(
      `/api/jobs?limit=50&sort=${sort}&q=${encodeURIComponent(q)}` +
      locations.map((l) => `&location=${encodeURIComponent(l)}`).join('') +
      (remote ? '&remote=1' : '') +
      (cursor ? `&cursor=${encodeURIComponent(cursor)}` : ''),
    ).then((r) => ({ jobs: r.jobs ?? [], next: r.next_cursor ?? null })),

  boards: () =>
    request<{ boards: Board[] }>('/api/boards').then((r) => r.boards ?? []),

  notifications: () =>
    request<{ notifications: MatchEvent[]; unread: number }>('/api/notifications?limit=50')
      .then((r) => ({ events: r.notifications ?? [], unread: r.unread ?? 0 })),

  markSeen: () => request<unknown>('/api/notifications/seen', { method: 'POST' }),

  registerDevice: (token: string) =>
    request<void>('/api/devices', {
      method: 'POST',
      body: JSON.stringify({
        token,
        platform: 'web',
        // Quiet hours run on the device's clock, not the server's.
        timezone: Intl.DateTimeFormat().resolvedOptions().timeZone ?? '',
      }),
    }),

  /** What the server knows about this browser's push: the browser's own
   *  permission says nothing about whether a usable token ever reached us. */
  pushStatus: () => request<{
    registered: boolean; devices: number; timezone?: string; last_notified_at?: string
    quiet_from?: number; quiet_to?: number
  }>('/api/devices/status'),

  /** Equal hours switch quiet hours off. */
  setQuietHours: (from: number, to: number) =>
    request<{ quiet_from: number; quiet_to: number }>('/api/devices/quiet-hours', {
      method: 'PUT', body: JSON.stringify({ from, to }),
    }),

  /** Proves the whole chain in one tap. */
  testPush: () => request<{ sent: number }>('/api/devices/test', { method: 'POST' }),

  // Best effort: a deployed backend reserves this for its cron and answers 401.
  poll: () => request<unknown>('/api/poll', { method: 'POST' }),

  /** Hides a job from this device's feeds. */
  hideJob: (id: number) => request<void>(`/api/jobs/${id}/hide`, { method: 'POST' }),

  /** The undo behind the toast. */
  unhideJob: (id: number) => request<void>(`/api/jobs/${id}/unhide`, { method: 'POST' }),

  /** Flips the applied state; answers with the new one. */
  toggleApplied: (id: number) =>
    request<{ applied: boolean }>(`/api/jobs/${id}/applied`, { method: 'POST' }),
}

/** A message worth putting on screen. */
export function describeError(error: unknown): string {
  if (error instanceof TypeError) {
    // fetch's shape for "could not connect at all".
    return 'Cannot reach the backend. On free hosting it may just be waking up — try again in a moment.'
  }
  return error instanceof Error ? error.message : String(error)
}
