import { useEffect, useRef } from 'react'
import { api, describeError } from '../api'
import { JobRow } from '../components/JobRow'
import { Empty, ErrorState, SkeletonList } from '../components/States'
import { RefreshIcon } from './Jobs'
import { invalidate, useQuery } from '../hooks'

export function Notifications() {
  const feed = useQuery('notifications', api.notifications)
  const marked = useRef(false)

  // Opening the screen marks the feed read. The dots stay for this viewing —
  // only the tab badge clears.
  useEffect(() => {
    if (!marked.current && feed.data && feed.data.unread > 0) {
      marked.current = true
      api.markSeen().catch(() => { marked.current = false })
    }
  }, [feed.data])

  let body
  if (feed.error) {
    body = <ErrorState message={describeError(feed.error)} onRetry={feed.refetch} />
  } else if (!feed.data) {
    body = <SkeletonList />
  } else if (feed.data.events.length === 0) {
    body = (
      <Empty
        title="No matches yet"
        detail="When the poller finds a job that fits one of your profiles, it lands here — and pings your phone."
      />
    )
  } else {
    body = (
      <div className="list">
        {feed.data.events.map((event) => (
          <JobRow
            key={`${event.profile_id}:${event.job.id}`}
            job={event.job}
            label={event.profile_name}
            showUnread
          />
        ))}
      </div>
    )
  }

  return (
    <section>
      <header className="bar">
        <h1>Notifications</h1>
        <button
          className="icon-btn"
          title="Refresh"
          onClick={() => { marked.current = false; invalidate('notifications') }}
        ><RefreshIcon /></button>
      </header>
      {body}
    </section>
  )
}
