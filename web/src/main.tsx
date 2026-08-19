import { createSyncStoragePersister } from '@tanstack/query-sync-storage-persister'
import { PersistQueryClientProvider } from '@tanstack/react-query-persist-client'
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { App } from './App'
import { queryClient } from './query'
import { showToast } from './toast'
import './styles.css'

// The service worker handles push display and app-shell caching. It calls
// skipWaiting on install, so a deployed update activates while the page is
// open — controllerchange is the moment to offer the reload, and the end of
// "hard refresh to get the new version".
if ('serviceWorker' in navigator) {
  navigator.serviceWorker.register('/firebase-messaging-sw.js')
  navigator.serviceWorker.addEventListener('controllerchange', () => {
    showToast('A new version is ready', { label: 'Reload', run: () => location.reload() }, 0)
  })
}

// Query cache persisted to storage: a cold open renders the last-known jobs
// instantly while the free-tier backend takes its minute to wake.
const persister = createSyncStoragePersister({ storage: window.localStorage })

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
