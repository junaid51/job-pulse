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
  /** Which saved searches caught this — only the all-searches feed says. */
  matched_by?: string
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
    // crypto.randomUUID exists only in a secure context. Served over plain
    // http on a LAN address — which is how you open the dev build on a phone —
    // it is undefined, and calling it crashed the bundle at module load: a
    // blank screen with one line in the console. The fallback does not need to
    // be cryptographic; it needs to be unique to this browser.
    id = crypto.randomUUID?.()
      ?? `d-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 12)}`
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

  /** The one feed, at one of three scopes: a saved search, every saved search
   *  ("all"), or the whole corpus (a typed query, which must never be narrowed
   *  to what your keywords happened to catch). Same filters either way. */
  feed: (p: {
    scope: number | 'all' | 'corpus'
    q?: string; locations?: string[]; remote?: boolean; market?: boolean
    /** ORed, the way a saved search means its keywords. */
    keywords?: string[]
    sort?: JobSort; cursor?: string
  }) => {
    const query = [
      'limit=50',
      `sort=${p.sort ?? 'matched'}`,
      typeof p.scope === 'number' ? `profile_id=${p.scope}` : '',
      p.scope === 'all' ? 'mine=1' : '',
      p.q ? `q=${encodeURIComponent(p.q)}` : '',
      p.remote ? 'remote=1' : '',
      p.market ? 'market=1' : '',
      p.cursor ? `cursor=${encodeURIComponent(p.cursor)}` : '',
      ...(p.locations ?? []).map((l) => `location=${encodeURIComponent(l)}`),
      ...(p.keywords ?? []).map((k) => `keyword=${encodeURIComponent(k)}`),
    ].filter(Boolean).join('&')
    return request<{ jobs: Job[]; next_cursor?: string }>(`/api/jobs?${query}`)
      .then((r) => ({ jobs: r.jobs ?? [], next: r.next_cursor ?? null }))
  },

  /** Whether the boards are actually being polled. The feed is only as fresh
   *  as the last cycle, and a poller that has stopped used to be invisible. */
  health: () => request<{
    status: string; poller: string
    last_poll_at?: string; poll_age_seconds?: number | null
    poll_error?: string
  }>('/healthz'),

  boards: () =>
    request<{ boards: Board[] }>('/api/boards').then((r) => r.boards ?? []),

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
