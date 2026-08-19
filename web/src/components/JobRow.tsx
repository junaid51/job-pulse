import type { Job } from '../api'
import { providerLabel, shortAgo } from '../format'

const DAY_MS = 24 * 60 * 60 * 1000

/** One row of the Jobs and Notifications lists. The whole row opens the
 *  official posting — applying means forms and logins, which belong in the
 *  real page, not in this app. */
export function JobRow(props: { job: Job; label?: string; showUnread?: boolean }) {
  const { job } = props
  const locationSaysRemote = job.location.toLowerCase().includes('remote')
  const meta = [
    job.company,
    job.location || null,
    job.remote && !locationSaysRemote ? 'Remote' : null,
    providerLabel(job.provider),
  ].filter(Boolean).join('  ·  ')

  const posted = job.posted_at ?? job.matched_at
  const isFresh = Date.now() - new Date(posted).getTime() < DAY_MS

  return (
    <a className="job-row" href={job.url} target="_blank" rel="noopener noreferrer">
      {props.showUnread && <span className={`dot ${job.seen_at ? '' : 'unread'}`} />}
      <CompanyMark name={job.company} />
      <span className="job-main">
        {props.label && <span className="job-label">{props.label}</span>}
        <span className="job-title">
          {job.title}
          {isFresh && <span className="fresh">NEW</span>}
        </span>
        <span className="job-meta">{meta}</span>
      </span>
      <span className="job-side">
        <span className="job-age" title={new Date(posted).toLocaleString()}>
          {shortAgo(posted)}
        </span>
        <span className="apply">Apply</span>
      </span>
    </a>
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
