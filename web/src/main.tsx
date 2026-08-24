import { createSyncStoragePersister } from '@tanstack/query-sync-storage-persister'
import { PersistQueryClientProvider } from '@tanstack/react-query-persist-client'
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { App } from './App'
import { queryClient } from './query'
import { showToast } from './toast'
import './styles.css'

const bootedAt = performance.now()

// The service worker handles push display and app-shell caching. It calls
// skipWaiting on install, so a deployed update activates while the page is
// open — controllerchange is the moment to offer the reload, and the end of
// "hard refresh to get the new version".
if ('serviceWorker' in navigator) {
  const loadedAt = Date.now()
  navigator.serviceWorker.register('/firebase-messaging-sw.js').then((registration) => {
    // iOS keeps an installed PWA warm for hours: resuming it is not a page
    // load, so nothing would ever notice a deploy. Foregrounding is the
    // moment to check (the worker is stamped per build, so a deploy always
    // changes its bytes).
    document.addEventListener('visibilitychange', () => {
      if (document.visibilityState === 'visible') registration.update().catch(() => {})
    })
  })
  navigator.serviceWorker.addEventListener('controllerchange', () => {
    // Right after a load the page already is the new version (the shell is
    // served no-cache); only a session that has been running needs the offer.
    if (Date.now() - loadedAt < 5000) return
    showToast('A new version is ready', { label: 'Reload', run: () => location.reload() }, 0)
  })
}

// Query cache persisted to storage: a cold open renders the last-known jobs
// instantly while the free-tier backend takes its minute to wake.
const persister = createSyncStoragePersister({ storage: window.localStorage })

/** The splash in index.html goes as soon as React has painted the real UI —
 *  but not before its own mark has finished drawing, because a splash that
 *  flashes for 80ms is worse than no splash at all. */
function dismissSplash() {
  const splash = document.getElementById('splash')
  if (!splash) return
  const wait = Math.max(0, 620 - (performance.now() - bootedAt))
  setTimeout(() => {
    splash.classList.add('gone')
    setTimeout(() => splash.remove(), 320)
  }, wait)
}

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <PersistQueryClientProvider
      client={queryClient}
      persistOptions={{ persister, maxAge: 24 * 60 * 60 * 1000 }}
    >
      <App />
    </PersistQueryClientProvider>
  </StrictMode>,
)

// Two frames: one for React to commit, one for the browser to paint it.
requestAnimationFrame(() => requestAnimationFrame(dismissSplash))
