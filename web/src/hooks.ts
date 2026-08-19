// A small fetch-cache with named keys and app-wide invalidation — the piece a
// data library would provide, hand-rolled because this app has three queries.
import { useCallback, useEffect, useSyncExternalStore } from 'react'

type Key = 'profiles' | 'notifications' | `jobs:${number}`

interface Entry<T> {
  data?: T
  error?: unknown
  loading: boolean
}

const cache = new Map<Key, Entry<unknown>>()
const listeners = new Set<() => void>()

function notify() {
  for (const listener of listeners) listener()
}

function load<T>(key: Key, fetcher: () => Promise<T>) {
  cache.set(key, { ...cache.get(key), loading: true })
  notify()
  fetcher().then(
    (data) => { cache.set(key, { data, loading: false }); notify() },
    (error) => { cache.set(key, { error, loading: false }); notify() },
  )
}

/** invalidate refetches every live query whose key matches the prefix. */
const refetchers = new Map<Key, () => void>()
export function invalidate(prefix: 'jobs' | 'profiles' | 'notifications') {
  for (const [key, refetch] of refetchers) {
    if (key.startsWith(prefix)) refetch()
  }
}

export function useQuery<T>(key: Key, fetcher: () => Promise<T>): Entry<T> & { refetch: () => void } {
  const refetch = useCallback(() => load(key, fetcher), [key]) // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    refetchers.set(key, refetch)
    if (!cache.has(key)) refetch()
    return () => { refetchers.delete(key) }
  }, [key, refetch])

  const entry = useSyncExternalStore(
    (listener) => { listeners.add(listener); return () => listeners.delete(listener) },
    () => (cache.get(key) as Entry<T>) ?? { loading: true },
  )
  return { ...entry, refetch }
}
