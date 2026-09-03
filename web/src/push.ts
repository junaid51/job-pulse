// Push via Firebase Cloud Messaging. Everything degrades to "no push" rather
// than failing: denied permission, an unsupported browser, or a fork not yet
// pointed at its own Firebase project all leave the rest of the app working.
//
// Firebase is the heaviest dependency in the bundle and most sessions never
// touch push, so it is imported lazily: the chunk downloads only once
// permission is granted (or being asked for), never on a plain page load.
import { api } from './api'
import { invalidate } from './query'

// Public client identifiers (also mirrored in the service worker). A fork
// points these at its own project.
const firebaseConfig = {
  apiKey: 'AIzaSyBwFRURB92XTXgPgPXjUc-nhmSGHCAAjRc',
  appId: '1:1079027519074:web:fda43881298dad3a648b3b',
  messagingSenderId: '1079027519074',
  projectId: 'jobpulse-junaid',
  authDomain: 'jobpulse-junaid.firebaseapp.com',
  storageBucket: 'jobpulse-junaid.firebasestorage.app',
}

// The VAPID key is public by design — it ships to every browser.
const VAPID = import.meta.env.VITE_JOBPULSE_VAPID ?? ''

export type PushState = 'unsupported' | 'off' | 'pending' | 'on'

// Why the last attempt failed, for the one screen that can act on it. A push
// setup that is broken and silent is the failure mode this whole app cannot
// afford: the notification *is* the product.
let lastError: string | null = null
export const pushError = () => lastError

// The cheap pre-check that needs no Firebase: can this browser do push at all?
const nativeSupport = () =>
  'Notification' in window && 'serviceWorker' in navigator && 'PushManager' in window

async function connect(): Promise<PushState> {
  const [{ initializeApp }, messagingModule] = await Promise.all([
    import('firebase/app'),
    import('firebase/messaging'),
  ])
  const { getMessaging, getToken, isSupported, onMessage } = messagingModule
  if (!(await isSupported().catch(() => false))) return 'unsupported'
  const app = initializeApp(firebaseConfig)
  const messaging = getMessaging(app)
  const registration = await navigator.serviceWorker.ready
  lastError = null
  const token = await getToken(messaging, {
    vapidKey: VAPID || undefined,
    serviceWorkerRegistration: registration,
  })
  if (!token) return 'off'
  await api.registerDevice(token)
  // A push while the app is open refreshes the feeds instead of showing a
  // banner: the new match appearing is the notification.
  onMessage(messaging, () => {
    invalidate('profiles')
    invalidate('profiles') // the chips carry unread counts
  })
  return 'on'
}

/** Called on startup: connects only if permission was already granted —
 *  browsers (iOS especially) only honor a request made from a tap. */
export async function initPush(): Promise<PushState> {
  if (!nativeSupport()) return 'unsupported'
  if (Notification.permission !== 'granted') return 'off'
  return connect().catch((error) => {
    lastError = String(error?.message ?? error)
    console.warn('push: token unavailable:', error)
    return 'off'
  })
}

/** The tap: ask for permission, then connect. */
export async function enablePush(): Promise<PushState> {
  if (!nativeSupport()) return 'unsupported'
  const permission = await Notification.requestPermission()
  if (permission !== 'granted') {
    lastError = 'This browser refused notification permission.'
    return 'off'
  }
  return connect().catch((error) => {
    lastError = String(error?.message ?? error)
    console.warn('push: token unavailable:', error)
    return 'off'
  })
}
