import { useQuery } from '@tanstack/react-query'
import React, { useEffect, useLayoutEffect, useRef, useState } from 'react'
import { api } from './api'
import { initPush, type PushState } from './push'
import { Toasts } from './toast'
import { Jobs } from './screens/Jobs'
import { Settings } from './screens/Settings'

// Two destinations: the feed, and the things that configure it. What used to be
// a third — Notifications — was the same list of matched jobs the feed already
// shows, and the two disagreed with each other often enough to be a bug. It is
// now the "New for me" chip on the feed itself.
type Tab = 'jobs' | 'settings'

/** #/notifications is what a tapped push points at, and older installs may
 *  still hold that URL — it lands on the feed, which is where the matches are. */
function tabFromHash(): Tab {
  return location.hash.startsWith('#/settings') ? 'settings' : 'jobs'
}

export function App() {
  // The tab lives in the URL hash so reload and back both behave.
  const [tab, setTab] = useState<Tab>(() => tabFromHash())
  useEffect(() => { location.hash = `#/${tab}` }, [tab])

  // A tapped notification points at #/notifications. If the app was closed the
  // initial state above reads it; if it was already open the service worker
  // just changes the hash on the live window, and only this listener notices.
  useEffect(() => {
    const onHashChange = () => {
      const next = tabFromHash()
      if (next !== tab) setTab(next)
    }
    window.addEventListener('hashchange', onHashChange)
    return () => window.removeEventListener('hashchange', onHashChange)
  }, [tab])

  const [push, setPush] = useState<PushState>('off')
  useEffect(() => { initPush().then(setPush) }, [])

  // Each tab keeps its place in the shared scroller, and re-selecting the
  // current tab is "take me to the top".
  const mainRef = useRef<HTMLElement>(null)
  const scrollMemory = useRef<Record<Tab, number>>({ jobs: 0, settings: 0 })
  const select = (next: Tab) => {
    const main = mainRef.current
    if (next === tab) {
      main?.scrollTo({ top: 0, behavior: 'smooth' })
      return
    }
    if (main) scrollMemory.current[tab] = main.scrollTop
    setTab(next)
  }
  useLayoutEffect(() => {
    const main = mainRef.current
    if (!main) return
    const top = scrollMemory.current[tab]
    main.scrollTop = top
    if (top === 0) return
    // The virtualized list re-measures a beat after being unhidden; until it
    // does, the content is short and the first restore clamps to 0.
    const raf = requestAnimationFrame(() => { main.scrollTop = top })
    const timer = setTimeout(() => {
      if (Math.abs(main.scrollTop - top) > 2) main.scrollTop = top
    }, 150)
    return () => { cancelAnimationFrame(raf); clearTimeout(timer) }
  }, [tab])

  // The per-search unread counts the feed's chips already show, summed.
  const profiles = useQuery({ queryKey: ['profiles'], queryFn: api.profiles })
  const unread = (profiles.data ?? []).reduce((n, profile) => n + profile.unread, 0)

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
      <Toasts />
      <main ref={mainRef}>
        {/* Jobs stays mounted: mid-scroll research and a typed search must
            survive a hop to Settings and back. */}
        <div className="screen" style={{ display: tab === 'jobs' ? undefined : 'none' }}>
          <Jobs goToSettings={() => select('settings')} />
        </div>
        {tab === 'settings' && (
          <div className="screen"><Settings push={push} setPush={setPush} /></div>
        )}
      </main>
      <nav>
        <div className="brand" aria-hidden>
          <PulseMark />
          <span>JobPulse</span>
        </div>
        <TabButton current={tab} tab="jobs" label="Jobs" glyph={<BriefcaseIcon />}
          badge={unread} onSelect={select} />
        <TabButton current={tab} tab="settings" label="Settings" glyph={<GearIcon />} onSelect={select} />
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
