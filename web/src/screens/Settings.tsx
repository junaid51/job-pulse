import { useState } from 'react'
import { API, api, describeError, deviceId, type Profile, type ProfileInput } from '../api'
import { Loading } from '../components/States'
import { TermPicker } from '../components/TermPicker'
import { providerLabel, shortAgo } from '../format'
import { useQuery } from '@tanstack/react-query'
import { invalidate } from '../query'
import { enablePush, type PushState } from '../push'

export function Settings({ push, setPush }: { push: PushState; setPush: (s: PushState) => void }) {
  const profiles = useQuery({ queryKey: ['profiles'], queryFn: api.profiles })
  // #/settings/new deep-links straight into the editor.
  const [editing, setEditing] = useState<Profile | 'new' | null>(
    () => location.hash === '#/settings/new' ? 'new' : null)

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
      {profiles.data?.length === 0 && <p className="state-detail pad">Nothing watching the boards yet.</p>}
      {profiles.data?.map((profile) => (
        <ProfileRow key={profile.id} profile={profile} onEdit={() => setEditing(profile)} onChanged={afterSave} />
      ))}
      {profiles.data && (
        <div className="pad">
          <button className="btn-tonal wide" onClick={() => setEditing('new')}>+ New search</button>
        </div>
      )}

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

// The roles here ride the same alias dictionary the matcher uses, so tapping
// "frontend" also finds React and Angular titles.
const ROLES = [
  'frontend', 'backend', 'full stack', 'mobile', 'devops', 'data', 'design', 'product', 'qa',
  // The company is not only its engineers: a business analyst opening this
  // screen should find her own work in it.
  'product owner', 'program manager', 'engineering manager', 'business analyst',
  'project manager', 'finance', 'marketing', 'operations', 'sales',
  'human resources', 'customer support',
]
const PLACES = ['dubai', 'abu dhabi', 'uae', 'saudi', 'qatar', 'gulf', 'india', 'uk', 'usa']
const NOISE = ['senior', 'lead', 'principal', 'manager', 'intern']

const titleCase = (s: string) => s.replace(/\b\w/g, (c) => c.toUpperCase())

function Editor(props: { existing: Profile | null; onClose: () => void; onSaved: () => void }) {
  const { existing } = props
  const [name, setName] = useState(existing?.name ?? '')
  // Stored keywords fold the exclusions in as "-term"; the editor keeps the
  // two apart so nobody has to know that syntax exists.
  const [keywords, setKeywords] = useState<string[]>(
    existing?.keywords.filter((k) => !k.startsWith('-')) ?? [])
  const [avoid, setAvoid] = useState<string[]>(
    existing?.keywords.filter((k) => k.startsWith('-')).map((k) => k.slice(1)) ?? [])
  const [locations, setLocations] = useState<string[]>(existing?.locations ?? [])
  const [remoteOnly, setRemoteOnly] = useState(existing?.remote_only ?? false)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const autoName =
    [keywords[0], locations[0]].filter(Boolean).map((t) => titleCase(t!)).join(' · ')
    || (remoteOnly ? 'Remote' : 'Everything')

  const save = async () => {
    setSaving(true)
    setError(null)
    const input: ProfileInput = {
      name: name.trim() || autoName,
      keywords: [...keywords, ...avoid.map((t) => `-${t}`)],
      locations,
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
        <h2>{existing ? 'Edit search' : 'New search'}</h2>

        <div className="f-group">
          <span className="f-label">What are you looking for?</span>
          <TermPicker value={keywords} onChange={setKeywords}
            suggestions={ROLES} placeholder="type a role or skill…" />
          <small>Any one matches. Roles find related titles too — frontend covers React and Angular, business analyst covers BI and systems analyst.</small>
        </div>

        <div className="f-group">
          <span className="f-label">Where?</span>
          <TermPicker value={locations} onChange={setLocations}
            suggestions={PLACES} placeholder="anywhere — or add a place…" />
          <label className="switch-row">
            <span>Remote roles only</span>
            <input type="checkbox" checked={remoteOnly}
              onChange={(e) => setRemoteOnly(e.target.checked)} />
          </label>
        </div>

        <div className="f-group">
          <span className="f-label">Skip jobs mentioning</span>
          <TermPicker value={avoid} onChange={setAvoid}
            suggestions={NOISE} placeholder="nothing — or add a word…" tone="danger" />
        </div>

        <label>Name
          <input value={name} onChange={(e) => setName(e.target.value)}
            placeholder={autoName} autoCapitalize="words" />
        </label>

        {error && <p className="error-text">{error}</p>}
        <button className="btn-filled wide" disabled={saving} type="submit">
          {saving ? 'Saving…' : existing ? 'Save changes' : `Watch for ${autoName.toLowerCase()}`}
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
