import { useState } from 'react'
import { API, api, describeError, type Profile, type ProfileInput } from '../api'
import { Loading } from '../components/States'
import { invalidate, useQuery } from '../hooks'
import { enablePush, type PushState } from '../push'

const splitList = (raw: string) =>
  raw.split(',').map((v) => v.trim()).filter(Boolean)

export function Settings({ push, setPush }: { push: PushState; setPush: (s: PushState) => void }) {
  const profiles = useQuery('profiles', api.profiles)
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
        <button className="icon-btn" title="New profile" onClick={() => setEditing('new')}>+</button>
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
      <button className="icon-btn" title="Delete" onClick={remove}>✕</button>
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
          <small>Comma separated. Any one of them in the title is a match.</small>
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
