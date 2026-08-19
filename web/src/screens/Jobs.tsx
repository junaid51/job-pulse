import { useState } from 'react'
import { api, describeError } from '../api'
import { JobRow } from '../components/JobRow'
import { Empty, ErrorState, Loading } from '../components/States'
import { invalidate, useQuery } from '../hooks'

async function refreshFeeds() {
  try { await api.poll() } catch { /* the cron's endpoint; refetch regardless */ }
  invalidate('jobs')
  invalidate('notifications')
}

export function Jobs({ goToSettings }: { goToSettings: () => void }) {
  const profiles = useQuery('profiles', api.profiles)
  const [selected, setSelected] = useState<number | null>(null)

  let body
  if (profiles.error) {
    body = <ErrorState message={describeError(profiles.error)} onRetry={profiles.refetch} />
  } else if (!profiles.data) {
    body = <Loading />
  } else if (profiles.data.length === 0) {
    body = (
      <Empty
        title="No search profiles yet"
        detail="A profile is what jobs get matched against: keywords, locations, remote or not."
        actionLabel="Create a profile"
        onAction={goToSettings}
      />
    )
  } else {
    const list = profiles.data
    const profile = list.find((candidate) => candidate.id === selected) ?? list[0]
    body = (
      <>
        {list.length > 1 && (
          <div className="chips">
            {list.map((candidate) => (
              <button
                key={candidate.id}
                className={`chip ${candidate.id === profile.id ? 'selected' : ''}`}
                onClick={() => setSelected(candidate.id)}
              >
                {candidate.name}
              </button>
            ))}
          </div>
        )}
        <JobList profileId={profile.id} />
      </>
    )
  }

  return (
    <section>
      <header className="bar">
        <h1>Jobs</h1>
        <button className="icon-btn" title="Refresh" onClick={refreshFeeds}>↻</button>
      </header>
      {body}
    </section>
  )
}

function JobList({ profileId }: { profileId: number }) {
  const jobs = useQuery(`jobs:${profileId}`, () => api.jobs(profileId))
  if (jobs.error) return <ErrorState message={describeError(jobs.error)} onRetry={jobs.refetch} />
  if (!jobs.data) return <Loading />
  if (jobs.data.length === 0) {
    return (
      <Empty
        title="Nothing matched yet"
        detail="Try broader keywords in Settings, or refresh — the boards are polled every half hour."
        actionLabel="Refresh"
        onAction={refreshFeeds}
      />
    )
  }
  return <div className="list">{jobs.data.map((job) => <JobRow key={job.id} job={job} />)}</div>
}
