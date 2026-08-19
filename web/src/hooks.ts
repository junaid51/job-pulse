// A small fetch-cache with named keys and app-wide invalidation — the piece a
// data library would provide, hand-rolled because this app has three queries.
import { useCallback, useEffect, useSyncExternalStore } from 'react'

type Key = 'profiles' | 'notifications' | 'boards' | `jobs:${string}`

interface Entry<T> {
  data?: T
  error?: unknown
  loading: boolean
}

// One shared fallback: returning a fresh object from getSnapshot makes React
// believe the store changed on every read.
const fallback: Entry<never> = { loading: true }

const cache = new Map<Key, Entry<unknown>>()
const listeners = new Set<() => void>()

function notify() {
  for (const listener of listeners) listener()
}

// A stable identity, or useSyncExternalStore tears the subscription down and
// rebuilds it on every render.
function subscribe(listener: () => void) {
  listeners.add(listener)
  return () => { listeners.delete(listener) }
}

function load<T>(key: Key, fetcher: () => Promise<T>) {
  cache.set(key, { ...cache.get(key), loading: true })
  notify()
  fetcher().then(
    (data) => { cache.set(key, { data, loading: false }); notify() },
    (error) => { cache.set(key, { error, loading: false }); notify() },
  )
}

/** invalidate refetches every live query whose key matches the prefix, and
 *  purges matching entries nothing is watching — otherwise a cached tab you
 *  left stays stale until the whole app reloads. */
const refetchers = new Map<Key, () => void>()
export function invalidate(prefix: 'jobs' | 'profiles' | 'notifications') {
  for (const key of [...cache.keys()]) {
    if (key.startsWith(prefix) && !refetchers.has(key)) cache.delete(key)
  }
  for (const [key, refetch] of refetchers) {
    if (key.startsWith(prefix)) refetch()
  }
}

export function useQuery<T>(key: Key, fetcher: () => Promise<T>): Entry<T> & { refetch: () => void } {
  const refetch = useCallback(() => load(key, fetcher), [key]) // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    refetchers.set(key, refetch)
    // Stale-while-revalidate: cached data renders instantly, but every mount
    // refetches in the background — a view you return to is never stale, and
    // actions (applied, hide) need no cross-view bookkeeping.
    refetch()
    return () => { refetchers.delete(key) }
  }, [key, refetch])

  const entry = useSyncExternalStore(
    subscribe,
    () => (cache.get(key) as Entry<T>) ?? fallback,
  )
  return { ...entry, refetch }
}
