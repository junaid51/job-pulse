import { useState } from 'react'
import { API, api, describeError, deviceId, type Profile, type ProfileInput } from '../api'
import { Loading } from '../components/States'
import { TermPicker } from '../components/TermPicker'
import { providerLabel, shortAgo } from '../format'
import { useQuery } from '@tanstack/react-query'
import { invalidate } from '../query'
import { enablePush, type PushState } from '../push'
import { showToast } from '../toast'

export function Settings({ push, setPush }: { push: PushState; setPush: (s: PushState) => void }) {
  const profiles = useQuery({ queryKey: ['profiles'], queryFn: api.profiles })
  // #/settings/new deep-links straight into the editor.
  const [advanced, setAdvanced] = useState(false)
  const [editing, setEditing] = useState<Profile | 'new' | null>(
    () => location.hash === '#/settings/new' ? 'new' : null)

  const afterSave = () => {
    setEditing(null)
    invalidate('profiles')
    invalidate('jobs')
  }

  return (
    <section>
      <header className="bar">
        <h1>Settings</h1>
      </header>

      <h2 className="section-h">Saved searches</h2>
      {profiles.error ? <p className="state-detail pad">{describeError(profiles.error)}</p> : null}
      {!profiles.data && !profiles.error && <Loading />}
      {profiles.data?.length === 0 && <p className="state-detail pad">No saved searches yet — the Jobs screen starts one.</p>}
      {profiles.data?.map((profile) => (
        <ProfileRow key={profile.id} profile={profile} onEdit={() => setEditing(profile)} onChanged={afterSave} />
      ))}
      {profiles.data && (
        <div className="pad">
          <button className="btn-tonal wide" onClick={() => setEditing('new')}>+ New search</button>
        </div>
      )}

      <h2 className="section-h">Notifications</h2>
      <PushHealth push={push} />
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

      {/* Two hundred board rows and a device UUID are diagnostics, not
          settings. They stay one tap away rather than in the way. */}
      <button className="disclosure" aria-expanded={advanced}
        onClick={() => setAdvanced((v) => !v)}>
        <span>Advanced</span>
        <span className="disclosure-mark">{advanced ? '−' : '+'}</span>
      </button>

      {advanced && (
        <>
          <h2 className="section-h">Delivery</h2>
          <LastDelivery />

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
        </>
      )}

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

/** Proof that the chain works, for when it seems not to. */
function LastDelivery() {
  const status = useQuery({ queryKey: ['push-status'], queryFn: api.pushStatus })
  const at = status.data?.last_notified_at
  return (
    <div className="kv">
      <span>Last alert delivered</span>
      <span className="kv-value">{at ? shortAgo(at) + ' ago' : 'never yet'}</span>
    </div>
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

/** When to stay silent, in this device's own timezone. Off by default: a
 *  posting that lands at 06:45 is exactly the one worth waking up for, and
 *  silence should be asked for rather than assumed. */
function QuietHours({ from, to, timezone }: { from: number; to: number; timezone?: string }) {
  const [saving, setSaving] = useState(false)
  const on = from !== to
  const hours = Array.from({ length: 24 }, (_, h) => h)
  const label = (h: number) => `${String(h).padStart(2, '0')}:00`

  const save = async (nextFrom: number, nextTo: number) => {
    setSaving(true)
    try {
      await api.setQuietHours(nextFrom, nextTo)
      invalidate('push-status')
      showToast(nextFrom === nextTo ? 'Quiet hours off' : `Silent ${label(nextFrom)}–${label(nextTo)}`)
    } catch (error) {
      showToast(describeError(error))
    } finally {
      setSaving(false)
    }
  }

  return (
    <>
      <label className="kv switch-row">
        <span>Quiet hours</span>
        <input type="checkbox" checked={on} disabled={saving}
          onChange={(e) => save(0, e.target.checked ? 8 : 0)} />
      </label>
      {on && (
        <div className="pad stack">
          <div className="quiet-range">
            <select value={from} disabled={saving} aria-label="Silent from"
              onChange={(e) => save(Number(e.target.value), to)}>
              {hours.map((h) => <option key={h} value={h}>{label(h)}</option>)}
            </select>
            <span>to</span>
            <select value={to} disabled={saving} aria-label="Silent until"
              onChange={(e) => save(from, Number(e.target.value))}>
              {hours.map((h) => <option key={h} value={h}>{label(h)}</option>)}
            </select>
          </div>
          <p className="state-detail">
            Alerts inside this window wait for it to end rather than being
            dropped{timezone ? `, on ${timezone} time` : ''}. The job still
            appears in the feed immediately.
          </p>
        </div>
      )}
    </>
  )
}

/** What the server actually knows, as opposed to what the browser reports.
 *  A token can expire silently, and until this existed the only symptom was
 *  alerts quietly never arriving. */
function PushHealth({ push }: { push: PushState }) {
  const status = useQuery({ queryKey: ['push-status'], queryFn: api.pushStatus })
  const [testing, setTesting] = useState(false)
  if (!status.data) return null
  const { registered, timezone } = status.data
  const enabled = push === 'on'
  return (
    <>
      <div className="kv">
        <span>Push</span>
        <span className="kv-value">{describePush(push, registered)}</span>
      </div>
      {registered && enabled && (
        <QuietHours from={status.data.quiet_from ?? 0} to={status.data.quiet_to ?? 0}
          timezone={timezone} />
      )}
      {registered && enabled && (
        <div className="pad">
          <button
            className="btn-tonal wide"
            disabled={testing}
            onClick={async () => {
              setTesting(true)
              try {
                await api.testPush()
                showToast('Test sent — it should arrive within a second or two')
                invalidate('push-status')
              } catch (error) {
                showToast(describeError(error))
              } finally {
                setTesting(false)
              }
            }}
          >
            {testing ? 'Sending…' : 'Send a test notification'}
          </button>
        </div>
      )}
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

/** The browser's permission and the server's token are two halves of one
 *  answer, and either half alone was misleading: permission granted with no
 *  token on the server means alerts silently never arrive. */
function describePush(state: PushState, registered: boolean): string {
  switch (state) {
    case 'on': return registered ? 'On' : 'On here, but the server has no token'
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

// Suggestions, not the vocabulary: these ride the same alias dictionary the
// matcher uses (tapping "frontend" also finds React and Angular titles), and
// anything not offered here can be typed and works identically. Twenty-one
// chips made the form taller than the phone; these are the ones actually
// picked, engineering and otherwise.
const ROLES = [
  'frontend', 'backend', 'full stack', 'mobile', 'devops', 'platform',
  'data', 'qa', 'design', 'product', 'engineering manager', 'business analyst',
]
const PLACES = ['dubai', 'abu dhabi', 'uae', 'saudi', 'qatar', 'gulf', 'india',
  'uk', 'usa', 'worldwide']
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
  // A new search is two questions. Naming it and excluding words are
  // refinements, and an existing search has already made them.
  const [more, setMore] = useState(existing !== null)

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

        <div className="sheet-body">
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

        {more ? (
          <>
            <div className="f-group">
              <span className="f-label">Skip jobs mentioning</span>
              <TermPicker value={avoid} onChange={setAvoid}
                suggestions={NOISE} placeholder="nothing — or add a word…" tone="danger" />
            </div>

            <label>Name
              <input value={name} onChange={(e) => setName(e.target.value)}
                placeholder={autoName} autoCapitalize="words" />
            </label>
          </>
        ) : (
          <button type="button" className="more-link" onClick={() => setMore(true)}>
            Name it, or skip certain words
          </button>
        )}

        </div>

        {error && <p className="error-text">{error}</p>}
        <button className="btn-filled wide" disabled={saving} type="submit">
          {saving ? 'Saving…' : existing ? 'Save changes' : `Watch for ${autoName.toLowerCase()}`}
        </button>
      </form>
    </div>
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
