// The one service worker: shows push notifications that arrive while no
// JobPulse window is focused, and keeps the app shell cached so it opens
// instantly (and offline, showing whatever it last knew).
//
// The Firebase config below is the project's public client identifiers,
// mirrored from src/push.ts — a service worker cannot import app modules.
importScripts('https://www.gstatic.com/firebasejs/11.6.0/firebase-app-compat.js');
importScripts('https://www.gstatic.com/firebasejs/11.6.0/firebase-messaging-compat.js');

firebase.initializeApp({
  apiKey: 'AIzaSyBwFRURB92XTXgPgPXjUc-nhmSGHCAAjRc',
  appId: '1:1079027519074:web:fda43881298dad3a648b3b',
  messagingSenderId: '1079027519074',
  projectId: 'jobpulse-junaid',
  authDomain: 'jobpulse-junaid.firebaseapp.com',
  storageBucket: 'jobpulse-junaid.firebasestorage.app',
});
// Messages carry a notification payload, so the browser displays them without
// handler code here; initializing messaging is still required.
firebase.messaging();

// --- app shell cache: network-first, fall back to cache when offline ---
const CACHE = 'jobpulse-shell-v1';

self.addEventListener('install', (event) => {
  event.waitUntil(caches.open(CACHE).then((c) => c.addAll(['/'])));
  self.skipWaiting();
});

self.addEventListener('activate', (event) => {
  event.waitUntil(
    caches.keys().then((keys) =>
      Promise.all(keys.filter((k) => k !== CACHE).map((k) => caches.delete(k)))),
  );
});

self.addEventListener('fetch', (event) => {
  const url = new URL(event.request.url);
  // Only the app's own static files; API calls always go to the network.
  if (url.origin !== location.origin || event.request.method !== 'GET') return;
  event.respondWith(
    fetch(event.request)
      .then((response) => {
        const copy = response.clone();
        caches.open(CACHE).then((c) => c.put(event.request, copy));
        return response;
      })
      .catch(() => caches.match(event.request, { ignoreSearch: false })
        .then((hit) => hit ?? caches.match('/'))),
  );
});
