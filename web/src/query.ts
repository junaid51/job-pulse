// One QueryClient for the app, importable outside React (row actions and the
// push handler invalidate from plain modules). This replaced a hand-rolled
// ~80-line cache after it produced its second real bug; the library's defaults
// are the behavior we had converged on by hand — serve cache instantly,
// refetch on mount and focus.
import { QueryClient } from '@tanstack/react-query'

export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: 1, // the backend may be waking from scale-to-zero; one retry, not three
    },
  },
})

export function invalidate(prefix: 'jobs' | 'profiles' | 'boards' | 'push-status') {
  queryClient.invalidateQueries({ queryKey: [prefix] })
}
