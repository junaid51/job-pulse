import { useEffect, useRef } from 'react'
import { api, describeError } from '../api'
import { JobRow } from '../components/JobRow'
import { Empty, ErrorState, SkeletonList } from '../components/States'
import { RefreshIcon } from './Jobs'
import { useQuery } from '@tanstack/react-query'
import { invalidate } from '../query'

function dayLabel(iso: string): string {
  const day = new Date(iso)
  const today = new Date()
  const yesterday = new Date(today.getTime() - 24 * 60 * 60 * 1000)
  const sameDay = (a: Date, b: Date) => a.toDateString() === b.toDateString()
  if (sameDay(day, today)) return 'Today'
  if (sameDay(day, yesterday)) return 'Yesterday'
  return day.toLocaleDateString(undefined, { weekday: 'short', month: 'short', day: 'numeric' })
}

export function Notifications() {
  const feed = useQuery({ queryKey: ['notifications'], queryFn: api.notifications })
  const marked = useRef(false)

  // Opening the screen marks the feed read. The dots stay for this viewing —
  // only the tab badge clears.
  useEffect(() => {
    if (!marked.current && feed.data && feed.data.unread > 0) {
      marked.current = true
      api.markSeen()
        // The Jobs screen stays mounted, so its chips' unread counts only
        // clear if told.
        .then(() => invalidate('profiles'))
        .catch(() => { marked.current = false })
    }
  }, [feed.data])

  let body
  if (feed.error) {
    body = <ErrorState message={describeError(feed.error)} onRetry={() => feed.refetch()} />
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
    // Group the feed by the day the match landed.
    const groups: { label: string; events: typeof feed.data.events }[] = []
    for (const event of feed.data.events) {
      const label = dayLabel(event.job.matched_at)
      const last = groups[groups.length - 1]
      if (last && last.label === label) last.events.push(event)
      else groups.push({ label, events: [event] })
    }
    body = (
      <div className="list">
        {groups.map((group) => (
          <div key={group.label}>
            <div className="day-h">{group.label}</div>
            {group.events.map((event) => (
              <JobRow
                key={`${event.profile_id}:${event.job.id}`}
                job={event.job}
                label={event.profile_name}
                showUnread
                ageOf="matched"
              />
            ))}
          </div>
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
