import { memo, useState } from 'react'
import { api, type Job } from '../api'
import { providerLabel, shortAgo } from '../format'
import { invalidate } from '../query'
import { showToast } from '../toast'

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
  highlight?: string[]
  ageOf?: 'posted' | 'applied' | 'matched'
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

  // Each view ages by its own event: the Applied view by when you applied, the
  // notifications feed by when the match landed, the jobs list by the posting.
  const posted = (props.ageOf === 'applied' && job.applied_at)
    || (props.ageOf === 'matched' && job.matched_at)
    || job.posted_at || job.matched_at
  const isFresh = props.ageOf !== 'applied' &&
    Date.now() - new Date(posted).getTime() < DAY_MS

  const hide = (event: React.MouseEvent) => {
    event.preventDefault()
    event.stopPropagation()
    api.hideJob(job.id)
      .then(() => {
        invalidate('jobs')
        invalidate('notifications')
        showToast('Hidden', {
          label: 'Undo',
          run: () => api.unhideJob(job.id)
            .then(() => { invalidate('jobs'); invalidate('notifications') })
            .catch(() => showToast('Could not undo')),
        })
      })
      .catch(() => { /* it stays visible; nothing to clean up */ })
  }

  const share = (event: React.MouseEvent) => {
    event.preventDefault()
    event.stopPropagation()
    const payload = { title: job.title, text: `${job.title} — ${job.company}`, url: job.url }
    if (navigator.share) {
      navigator.share(payload).catch(() => { /* user closed the sheet */ })
    } else {
      navigator.clipboard?.writeText(job.url)
        .then(() => showToast('Link copied'))
        .catch(() => { /* clipboard denied; nothing useful left to try */ })
    }
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
          <Highlighted text={job.title} terms={props.highlight} />
          {isFresh && <span className="fresh">NEW</span>}
          {applied && <span className="applied-tag">APPLIED</span>}
        </span>
        <span className="job-meta">{meta}</span>
        {job.salary && <span className="job-salary">{job.salary}</span>}
      </span>
      <span className="job-side">
        <span className="job-age" title={new Date(posted).toLocaleString()}>
          {props.ageOf === 'applied' ? `✓ ${shortAgo(posted)}` : shortAgo(posted)}
        </span>
        <span className="apply">Apply</span>
        {props.actions && (
          <span className="row-actions">
            <button title={applied ? 'Applied — tap to undo' : 'Mark applied'}
              className={applied ? 'on' : ''} onClick={toggleApplied}>
              <CheckIcon />
            </button>
            <button title="Share" onClick={share}>
              <ShareIcon />
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

/** Marks where a search term or profile keyword literally appears in the
 *  title — the "why is this here" at a glance. Alias-driven matches (the
 *  dictionary lives server-side) simply go unmarked. */
function Highlighted({ text, terms }: { text: string; terms?: string[] }) {
  const needles = (terms ?? []).map((t) => t.trim().toLowerCase()).filter(Boolean)
  if (needles.length === 0) return <>{text}</>
  const lower = text.toLowerCase()
  let earliest = -1
  let length = 0
  for (const needle of needles) {
    const at = lower.indexOf(needle)
    if (at !== -1 && (earliest === -1 || at < earliest)) {
      earliest = at
      length = needle.length
    }
  }
  if (earliest === -1) return <>{text}</>
  return (
    <>
      {text.slice(0, earliest)}
      <mark className="hit">{text.slice(earliest, earliest + length)}</mark>
      {text.slice(earliest + length)}
    </>
  )
}

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

function ShareIcon() {
  return (
    <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor"
      strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden>
      <path d="M12 3v12M12 3 8 7M12 3l4 4M5 13v6a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2v-6" />
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
