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
}

export interface MatchEvent {
  profile_id: number
  profile_name: string
  job: Job
}

export type JobSort = 'posted' | 'matched'

export interface ProfileInput {
  name: string
  keywords: string[]
  locations: string[]
  remote_only: boolean
}

// Baked in at build time, like every deployment constant in this app.
export const API = import.meta.env.VITE_JOBPULSE_API ?? 'http://localhost:8091'

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(API + path, {
    ...init,
    headers: init?.body ? { 'Content-Type': 'application/json' } : undefined,
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

  jobs: (profileId: number, sort: JobSort = 'posted', q = '') =>
    request<{ jobs: Job[] }>(
      `/api/jobs?profile_id=${profileId}&limit=50&sort=${sort}&q=${encodeURIComponent(q)}`,
    ).then((r) => r.jobs ?? []),

  notifications: () =>
    request<{ notifications: MatchEvent[]; unread: number }>('/api/notifications?limit=50')
      .then((r) => ({ events: r.notifications ?? [], unread: r.unread ?? 0 })),

  markSeen: () => request<unknown>('/api/notifications/seen', { method: 'POST' }),

  registerDevice: (token: string) =>
    request<void>('/api/devices', {
      method: 'POST', body: JSON.stringify({ token, platform: 'web' }),
    }),

  // Best effort: a deployed backend reserves this for its cron and answers 401.
  poll: () => request<unknown>('/api/poll', { method: 'POST' }),
}

/** A message worth putting on screen. */
export function describeError(error: unknown): string {
  if (error instanceof TypeError) {
    // fetch's shape for "could not connect at all".
    return 'Cannot reach the backend. On free hosting it may just be waking up — try again in a moment.'
  }
  return error instanceof Error ? error.message : String(error)
}
