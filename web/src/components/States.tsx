// The three things a screen shows when it has no list to show.
import { useEffect, useState } from 'react'

/** After a few seconds the spinner explains itself: on free hosting the
 *  backend scales to zero, and the first request of the day wakes it. */
export function Loading() {
  const [slow, setSlow] = useState(false)
  useEffect(() => {
    const timer = setTimeout(() => setSlow(true), 5000)
    return () => clearTimeout(timer)
  }, [])
  return (
    <div className="state">
      <div className="spinner" aria-label="Loading" />
      <p className={`state-detail fade ${slow ? 'visible' : ''}`}>
        Still working — the backend may be waking up.<br />
        Free hosting takes up to a minute.
      </p>
    </div>
  )
}

export function ErrorState({ message, onRetry }: { message: string; onRetry: () => void }) {
  return (
    <div className="state">
      <p className="state-title">{message}</p>
      <button className="btn-tonal" onClick={onRetry}>Try again</button>
    </div>
  )
}

export function Empty(props: {
  title: string
  detail?: string
  actionLabel?: string
  onAction?: () => void
}) {
  return (
    <div className="state">
      <p className="state-title">{props.title}</p>
      {props.detail && <p className="state-detail">{props.detail}</p>}
      {props.actionLabel && (
        <button className="btn-tonal" onClick={props.onAction}>{props.actionLabel}</button>
      )}
    </div>
  )
}

/** Placeholder rows while a list loads: content-shaped, so the screen does not
 *  jump when the real rows arrive. */
export function SkeletonList({ rows = 8 }: { rows?: number }) {
  return (
    <div className="list" aria-hidden>
      {Array.from({ length: rows }, (_, i) => (
        <div className="job-row skeleton" key={i}>
          <span className="mark shimmer" />
          <span className="job-main">
            <span className="shimmer line w60" />
            <span className="shimmer line w80" />
          </span>
          <span className="job-side">
            <span className="shimmer line w20" />
          </span>
        </div>
      ))}
    </div>
  )
}
