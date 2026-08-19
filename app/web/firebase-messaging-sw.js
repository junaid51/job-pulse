// Service worker for web push. The FCM SDK registers this file by its fixed
// name at the site root; a push that arrives while no JobPulse tab is focused
// is shown by the browser from here.
//
// Config values are the public client identifiers from lib/firebase_options.dart
// (a service worker cannot import Dart). Re-run `flutterfire configure` and
// update these if the Firebase project ever changes.
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
// any handler code here. Initializing messaging is still required.
firebase.messaging();
