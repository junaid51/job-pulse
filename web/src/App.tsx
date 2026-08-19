import { useQuery } from '@tanstack/react-query'
import React, { useEffect, useState } from 'react'
import { api } from './api'
import { initPush, type PushState } from './push'
import { Jobs } from './screens/Jobs'
import { Notifications } from './screens/Notifications'
import { Settings } from './screens/Settings'

type Tab = 'jobs' | 'notifications' | 'settings'

export function App() {
  // The tab lives in the URL hash so reload and back both behave.
  const [tab, setTab] = useState<Tab>(() =>
    (['jobs', 'notifications', 'settings'] as const).find(
      (candidate) => location.hash === `#/${candidate}`) ?? 'jobs')
  useEffect(() => { location.hash = `#/${tab}` }, [tab])

  const [push, setPush] = useState<PushState>('off')
  useEffect(() => { initPush().then(setPush) }, [])

  const feed = useQuery({ queryKey: ['notifications'], queryFn: api.notifications })
  const unread = feed.data?.unread ?? 0

  // The count on the app icon itself — iOS 16.4+ Home-Screen apps and desktop
  // PWAs support the Badging API; everywhere else this is a silent no-op.
  useEffect(() => {
    const nav = navigator as Navigator & {
      setAppBadge?: (n: number) => Promise<void>
      clearAppBadge?: () => Promise<void>
    }
    if (unread > 0) nav.setAppBadge?.(unread)?.catch(() => {})
    else nav.clearAppBadge?.()?.catch(() => {})
  }, [unread])

  return (
    <div className="frame">
      <main>
        {tab === 'jobs' && <Jobs goToSettings={() => setTab('settings')} />}
        {tab === 'notifications' && <Notifications />}
        {tab === 'settings' && <Settings push={push} setPush={setPush} />}
      </main>
      <nav>
        <div className="brand" aria-hidden>
          <PulseMark />
          <span>JobPulse</span>
        </div>
        <TabButton current={tab} tab="jobs" label="Jobs" glyph={<BriefcaseIcon />} onSelect={setTab} />
        <TabButton current={tab} tab="notifications" label="Notifications" glyph={<BellIcon />}
          badge={unread} onSelect={setTab} />
        <TabButton current={tab} tab="settings" label="Settings" glyph={<GearIcon />} onSelect={setTab} />
      </nav>
    </div>
  )
}

function TabButton(props: {
  current: Tab; tab: Tab; label: string; glyph: React.ReactNode; badge?: number
  onSelect: (tab: Tab) => void
}) {
  return (
    <button
      className={`tab ${props.current === props.tab ? 'active' : ''}`}
      onClick={() => props.onSelect(props.tab)}
    >
      <span className="tab-glyph">
        {props.glyph}
        {(props.badge ?? 0) > 0 && <span className="badge">{props.badge}</span>}
      </span>
      {props.label}
    </button>
  )
}


// Inline icons: unicode glyphs render as tofu on iOS, and an icon font would be
// a dependency for three pictures.
const icon = { width: 20, height: 20, viewBox: '0 0 24 24', fill: 'none',
  stroke: 'currentColor', strokeWidth: 1.8, strokeLinecap: 'round', strokeLinejoin: 'round' } as const

function BriefcaseIcon() {
  return (
    <svg {...icon}>
      <rect x="3" y="7.5" width="18" height="12.5" rx="2.5" />
      <path d="M8.5 7.5V6a2 2 0 0 1 2-2h3a2 2 0 0 1 2 2v1.5M3 12.5h18" />
    </svg>
  )
}

function BellIcon() {
  return (
    <svg {...icon}>
      <path d="M18 9.5a6 6 0 1 0-12 0c0 5-2 6-2 6h16s-2-1-2-6" />
      <path d="M10.3 19.5a2 2 0 0 0 3.4 0" />
    </svg>
  )
}

function GearIcon() {
  return (
    <svg {...icon}>
      <circle cx="12" cy="12" r="3.2" />
      <path d="M12 2.8v2.4M12 18.8v2.4M21.2 12h-2.4M5.2 12H2.8M18.5 5.5l-1.7 1.7M7.2 16.8l-1.7 1.7M18.5 18.5l-1.7-1.7M7.2 7.2 5.5 5.5" />
    </svg>
  )
}


function PulseMark() {
  return (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor"
      strokeWidth="2.4" strokeLinecap="round" strokeLinejoin="round" aria-hidden>
      <path d="M3 12h4l2.5-5 4 10 2.5-5h5" />
    </svg>
  )
}
