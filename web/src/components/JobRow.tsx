import type { Job } from '../api'
import { providerLabel, shortAgo } from '../format'

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

  return (
    <a className="job-row" href={job.url} target="_blank" rel="noopener noreferrer">
      {props.showUnread && <span className={`dot ${job.seen_at ? '' : 'unread'}`} />}
      <span className="job-main">
        {props.label && <span className="job-label">{props.label}</span>}
        <span className="job-title">{job.title}</span>
        <span className="job-meta">{meta}</span>
      </span>
      <span className="job-side">
        <span className="job-age">{shortAgo(job.posted_at ?? job.matched_at)}</span>
        <span className="apply">Apply</span>
      </span>
    </a>
  )
}
