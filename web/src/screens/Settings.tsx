import { useState } from 'react'
import { API, api, describeError, deviceId, type Profile, type ProfileInput } from '../api'
import { Loading } from '../components/States'
import { providerLabel, shortAgo } from '../format'
import { useQuery } from '@tanstack/react-query'
import { invalidate } from '../query'
import { enablePush, type PushState } from '../push'

const splitList = (raw: string) =>
  raw.split(',').map((v) => v.trim()).filter(Boolean)

export function Settings({ push, setPush }: { push: PushState; setPush: (s: PushState) => void }) {
  const profiles = useQuery({ queryKey: ['profiles'], queryFn: api.profiles })
  const [editing, setEditing] = useState<Profile | 'new' | null>(null)

  const afterSave = () => {
    setEditing(null)
    invalidate('profiles')
    invalidate('jobs')
    invalidate('notifications')
  }

  return (
    <section>
      <header className="bar">
        <h1>Settings</h1>
        <button className="icon-btn" title="New profile" onClick={() => setEditing('new')}>
          <PlusIcon />
        </button>
      </header>

      <h2 className="section-h">Search profiles</h2>
      {profiles.error ? <p className="state-detail pad">{describeError(profiles.error)}</p> : null}
      {!profiles.data && !profiles.error && <Loading />}
      {profiles.data?.length === 0 && <p className="state-detail pad">None yet. Use + to add one.</p>}
      {profiles.data?.map((profile) => (
        <ProfileRow key={profile.id} profile={profile} onEdit={() => setEditing(profile)} onChanged={afterSave} />
      ))}

      <h2 className="section-h">Notifications</h2>
      <div className="kv"><span>Push</span><span className="kv-value">{describePush(push)}</span></div>
      {push !== 'on' && (
        <div className="pad stack">
          <button
            className="btn-tonal wide"
            onClick={async () => { setPush('pending'); setPush(await enablePush()) }}
          >
            Enable push notifications
          </button>
          <p className="state-detail">
            On iPhone: add this app to your Home Screen first (Share → Add to Home
            Screen) and open it from there — iOS only allows notifications for
            installed web apps.
          </p>
        </div>
      )}

      <h2 className="section-h">Boards</h2>
      <Boards />

      <h2 className="section-h">This device</h2>
      <DeviceIdentity />

      <h2 className="section-h">Backend</h2>
      <div className="kv"><span>URL</span><span className="kv-value">{API}</span></div>
      <p className="state-detail pad">
        Fixed at build time. The boards are polled on a schedule; refresh any
        screen to pick up what the last poll found.
      </p>

      {editing && (
        <Editor
          existing={editing === 'new' ? null : editing}
          onClose={() => setEditing(null)}
          onSaved={afterSave}
        />
      )}
    </section>
  )
}

function Boards() {
  const boards = useQuery({ queryKey: ['boards'], queryFn: api.boards })
  if (boards.error) return <p className="state-detail pad">{describeError(boards.error)}</p>
  if (!boards.data) return <Loading />
  const failing = boards.data.filter((b) => b.last_error)
  return (
    <>
      <p className="state-detail pad">
        {boards.data.length} sources · {boards.data.reduce((n, b) => n + b.jobs, 0)} live jobs
        {failing.length > 0 && ` · ${failing.length} failing`}
      </p>
      {boards.data.map((b) => (
        <div className="board-row" key={`${b.provider}:${b.slug}`}>
          <span className={`board-dot ${b.last_error ? 'bad' : 'ok'}`} />
          <span className="board-main">
            <span className="board-name">{b.name || b.slug}</span>
            <span className="job-meta">
              {providerLabel(b.provider)} · {b.jobs} {b.jobs === 1 ? 'job' : 'jobs'}
              {b.last_error ? ` · ${b.last_error}` : ''}
            </span>
          </span>
          <span className="job-age">
            {b.last_polled_at ? shortAgo(b.last_polled_at) : 'never'}
          </span>
        </div>
      ))}
    </>
  )
}

/** The anonymous id everything this device owns is filed under. Losing it
 *  (reinstalling the Home-Screen app clears site data) orphans the profiles —
 *  so it can be copied out and pasted back in. */
function DeviceIdentity() {
  const [adopt, setAdopt] = useState('')
  const [copied, setCopied] = useState(false)
  return (
    <>
      <div className="kv">
        <span>Device ID</span>
        <span className="kv-value">
          {deviceId.slice(0, 13)}…{' '}
          <button
            className="linkish"
            onClick={() => {
              navigator.clipboard?.writeText(deviceId)
                .then(() => { setCopied(true); setTimeout(() => setCopied(false), 1500) })
            }}
          >
            {copied ? 'copied' : 'copy'}
          </button>
        </span>
      </div>
      <div className="pad stack">
        <p className="state-detail">
          Your searches belong to this ID. Keep a copy somewhere — reinstalling
          the app clears it, and pasting it back here reclaims everything.
        </p>
        <form
          className="adopt"
          onSubmit={(event) => {
            event.preventDefault()
            const id = adopt.trim()
            if (!id) return
            localStorage.setItem('jobpulse-device', id)
            location.reload()
          }}
        >
          <input
            value={adopt}
            onChange={(event) => setAdopt(event.target.value)}
            placeholder="Paste a device ID to restore it"
            aria-label="Device ID to adopt"
          />
          <button className="btn-tonal" type="submit" disabled={!adopt.trim()}>Use</button>
        </form>
      </div>
    </>
  )
}

function describePush(state: PushState): string {
  switch (state) {
    case 'on': return 'On'
    case 'pending': return 'Setting up…'
    case 'unsupported': return 'Not supported in this browser'
    default: return 'Off'
  }
}

function ProfileRow(props: { profile: Profile; onEdit: () => void; onChanged: () => void }) {
  const { profile } = props
  const terms = [
    ...profile.keywords,
    ...profile.locations.map((location) => `@${location}`),
    ...(profile.remote_only ? ['remote only'] : []),
  ]
  const remove = async () => {
    if (!confirm(`Delete "${profile.name}"? Its matched jobs disappear with it.`)) return
    try {
      await api.deleteProfile(profile.id)
      props.onChanged()
    } catch (error) {
      alert(describeError(error))
    }
  }
  return (
    <div className="profile-row">
      <button className="profile-main" onClick={props.onEdit}>
        <span className="job-title">{profile.name}</span>
        <span className="job-meta">{terms.length ? terms.join('  ·  ') : 'Everything'}</span>
      </button>
      <button className="icon-btn" title="Delete" onClick={remove}>
        <CrossIcon />
      </button>
    </div>
  )
}

function Editor(props: { existing: Profile | null; onClose: () => void; onSaved: () => void }) {
  const { existing } = props
  const [name, setName] = useState(existing?.name ?? '')
  const [keywords, setKeywords] = useState(existing?.keywords.join(', ') ?? '')
  const [locations, setLocations] = useState(existing?.locations.join(', ') ?? '')
  const [remoteOnly, setRemoteOnly] = useState(existing?.remote_only ?? false)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const save = async () => {
    setSaving(true)
    setError(null)
    const input: ProfileInput = {
      name: name.trim(),
      keywords: splitList(keywords),
      locations: splitList(locations),
      remote_only: remoteOnly,
    }
    try {
      if (existing) await api.updateProfile(existing.id, input)
      else await api.createProfile(input)
      props.onSaved()
    } catch (cause) {
      setError(describeError(cause))
      setSaving(false)
    }
  }

  return (
    <div className="sheet-backdrop" onClick={props.onClose}>
      <form
        className="sheet"
        onClick={(event) => event.stopPropagation()}
        onSubmit={(event) => { event.preventDefault(); save() }}
      >
        <h2>{existing ? 'Edit profile' : 'New profile'}</h2>
        <label>Name
          <input value={name} onChange={(e) => setName(e.target.value)}
            placeholder="Backend Go" autoFocus={!existing} required />
        </label>
        <label>Keywords
          <input value={keywords} onChange={(e) => setKeywords(e.target.value)}
            placeholder="go, backend, platform" />
          <small>Comma separated; any one matches. Prefix with - to exclude: designer, -senior</small>
        </label>
        <label>Locations
          <input value={locations} onChange={(e) => setLocations(e.target.value)}
            placeholder="dubai, uae, remote" />
          <small>Comma separated. Leave empty for anywhere.</small>
        </label>
        <label className="switch-row">
          <span>Remote only</span>
          <input type="checkbox" checked={remoteOnly}
            onChange={(e) => setRemoteOnly(e.target.checked)} />
        </label>
        {error && <p className="error-text">{error}</p>}
        <button className="btn-filled wide" disabled={saving} type="submit">
          {saving ? 'Saving…' : 'Save'}
        </button>
      </form>
    </div>
  )
}


function PlusIcon() {
  return (
    <svg width="17" height="17" viewBox="0 0 24 24" fill="none" stroke="currentColor"
      strokeWidth="2" strokeLinecap="round" aria-hidden>
      <path d="M12 5v14M5 12h14" />
    </svg>
  )
}

function CrossIcon() {
  return (
    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor"
      strokeWidth="2" strokeLinecap="round" aria-hidden>
      <path d="m6 6 12 12M18 6 6 18" />
    </svg>
  )
}
