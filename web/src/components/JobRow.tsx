import { memo, useState } from 'react'
import { api, type Job } from '../api'
import { providerLabel, shortAgo } from '../format'
import { invalidate } from '../hooks'

const DAY_MS = 24 * 60 * 60 * 1000

/** One row of the Jobs and Notifications lists. The whole row opens the
 *  official posting — applying means forms and logins, which belong in the
 *  real page, not in this app. Memoized: the search box lives beside a
 *  fifty-row list, and without this every keystroke re-renders every row. */
export const JobRow = memo(function JobRow(props: {
  job: Job
  label?: string
  showUnread?: boolean
  actions?: boolean
}) {
  const { job } = props
  const [applied, setApplied] = useState(job.applied)
  const locationSaysRemote = job.location.toLowerCase().includes('remote')
  const meta = [
    job.company,
    job.location || null,
    job.remote && !locationSaysRemote ? 'Remote' : null,
    providerLabel(job.provider),
  ].filter(Boolean).join('  ·  ')

  const posted = job.posted_at ?? job.matched_at
  const isFresh = Date.now() - new Date(posted).getTime() < DAY_MS

  const hide = (event: React.MouseEvent) => {
    event.preventDefault()
    event.stopPropagation()
    api.hideJob(job.id).then(() => { invalidate('jobs'); invalidate('notifications') })
      .catch(() => { /* it stays visible; nothing to clean up */ })
  }
  const toggleApplied = (event: React.MouseEvent) => {
    event.preventDefault()
    event.stopPropagation()
    setApplied((v) => !v) // optimistic; the answer corrects it
    api.toggleApplied(job.id).then((r) => setApplied(r.applied)).catch(() => setApplied(job.applied))
  }

  return (
    <a className={`job-row ${applied ? 'applied' : ''}`} href={job.url}
      target="_blank" rel="noopener noreferrer">
      {props.showUnread && <span className={`dot ${job.seen_at ? '' : 'unread'}`} />}
      <CompanyMark name={job.company} />
      <span className="job-main">
        {props.label && <span className="job-label">{props.label}</span>}
        <span className="job-title">
          {job.title}
          {isFresh && <span className="fresh">NEW</span>}
          {applied && <span className="applied-tag">APPLIED</span>}
        </span>
        <span className="job-meta">{meta}</span>
        {job.salary && <span className="job-salary">{job.salary}</span>}
      </span>
      <span className="job-side">
        <span className="job-age" title={new Date(posted).toLocaleString()}>
          {shortAgo(posted)}
        </span>
        <span className="apply">Apply</span>
        {props.actions && (
          <span className="row-actions">
            <button title={applied ? 'Applied — tap to undo' : 'Mark applied'}
              className={applied ? 'on' : ''} onClick={toggleApplied}>
              <CheckIcon />
            </button>
            <button title="Hide this job" onClick={hide}>
              <HideIcon />
            </button>
          </span>
        )}
      </span>
    </a>
  )
})

/** A letter mark with a hue derived from the company name — stable identity
 *  without logos, which the boards do not reliably provide. */
function CompanyMark({ name }: { name: string }) {
  let hash = 0
  for (const ch of name) hash = (hash * 31 + ch.codePointAt(0)!) | 0
  const hue = ((hash % 360) + 360) % 360
  return (
    <span className="mark" style={{ ['--hue' as string]: hue }} aria-hidden>
      {(name[0] ?? '·').toUpperCase()}
    </span>
  )
}

function CheckIcon() {
  return (
    <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor"
      strokeWidth="2.4" strokeLinecap="round" strokeLinejoin="round" aria-hidden>
      <path d="m4.5 12.5 5 5 10-11" />
    </svg>
  )
}

function HideIcon() {
  return (
    <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor"
      strokeWidth="2.2" strokeLinecap="round" aria-hidden>
      <path d="m6 6 12 12M18 6 6 18" />
    </svg>
  )
}
