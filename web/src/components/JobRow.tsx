import { memo, useState } from 'react'
import { api, describeError, type Job } from '../api'
import { providerLabel, shortAgo } from '../format'
import { invalidate } from '../query'
import { showToast } from '../toast'

/** One row of the Jobs and Notifications lists. The whole row opens the
 *  official posting — applying means forms and logins, which belong in the
 *  real page, not in this app. Memoized: the search box lives beside a
 *  fifty-row list, and without this every keystroke re-renders every row. */
export const JobRow = memo(function JobRow(props: {
  job: Job
  /** The saved search that caught this, when the feed spans several. */
  via?: string
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
    props.via ? `via ${props.via}` : providerLabel(job.provider),
  ].filter(Boolean).join('  ·  ')

  // Each view ages by its own event: the Applied view by when you applied, the
  // notifications feed by when the match landed, the jobs list by the posting.
  const posted = (props.ageOf === 'applied' && job.applied_at)
    || (props.ageOf === 'matched' && job.matched_at)
    || job.posted_at || job.matched_at

  const hide = (event: React.MouseEvent) => {
    event.preventDefault()
    event.stopPropagation()
    api.hideJob(job.id)
      .then(() => {
        invalidate('jobs')
        invalidate('profiles')
        showToast('Hidden', {
          label: 'Undo',
          run: () => api.unhideJob(job.id)
            .then(() => { invalidate('jobs'); invalidate('profiles') })
            .catch(() => showToast('Could not undo')),
        })
      })
      // A dead button is worse than a failed one: this silently did nothing
      // for every search result, because hunt state used to need a match row.
      .catch((error) => showToast(describeError(error)))
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
    api.toggleApplied(job.id)
      .then((r) => setApplied(r.applied))
      .catch((error) => { setApplied(job.applied); showToast(describeError(error)) })
  }

  return (
    <a className={`job-row ${applied ? 'applied' : ''}`} href={job.url}
      target="_blank" rel="noopener noreferrer">
      {props.showUnread && <span className={`dot ${job.seen_at ? '' : 'unread'}`} />}
      <CompanyMark name={job.company} />
      <span className="job-main">
        <span className="job-title">
          <Highlighted text={job.title} terms={props.highlight} />
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
  const needles = (terms ?? [])
    .map((t) => t.trim())
    .filter(Boolean)
    .map((t) => t.replace(/[.*+?^${}()|[\]\\]/g, '\\$&'))
  if (needles.length === 0) return <>{text}</>
  // Every word that earned the row, not just the first one: the search matches
  // words independently now, so a single underline would explain half of it.
  const pattern = new RegExp(`(${needles.join('|')})`, 'gi')
  const pieces = text.split(pattern)
  if (pieces.length === 1) return <>{text}</>
  return (
    <>
      {pieces.map((piece, i) =>
        i % 2 === 1
          ? <mark className="hit" key={i}>{piece}</mark>
          : piece,
      )}
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
