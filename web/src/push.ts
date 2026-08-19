// Push via Firebase Cloud Messaging. Everything degrades to "no push" rather
// than failing: denied permission, an unsupported browser, or a fork not yet
// pointed at its own Firebase project all leave the rest of the app working.
import { initializeApp } from 'firebase/app'
import { getMessaging, getToken, isSupported, onMessage } from 'firebase/messaging'
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

async function connect(): Promise<PushState> {
  const app = initializeApp(firebaseConfig)
  const messaging = getMessaging(app)
  const registration = await navigator.serviceWorker.ready
  const token = await getToken(messaging, {
    vapidKey: VAPID || undefined,
    serviceWorkerRegistration: registration,
  })
  if (!token) return 'off'
  await api.registerDevice(token)
  // A push while the app is open refreshes the feeds instead of showing a
  // banner: the new match appearing is the notification.
  onMessage(messaging, () => invalidate('notifications'))
  return 'on'
}

/** Called on startup: connects only if permission was already granted —
 *  browsers (iOS especially) only honor a request made from a tap. */
export async function initPush(): Promise<PushState> {
  if (!(await isSupported().catch(() => false))) return 'unsupported'
  if (Notification.permission !== 'granted') return 'off'
  return connect().catch((error) => {
    console.warn('push: token unavailable:', error)
    return 'off'
  })
}

/** The tap: ask for permission, then connect. */
export async function enablePush(): Promise<PushState> {
  if (!(await isSupported().catch(() => false))) return 'unsupported'
  const permission = await Notification.requestPermission()
  if (permission !== 'granted') return 'off'
  return connect().catch((error) => {
    console.warn('push: token unavailable:', error)
    return 'off'
  })
}
