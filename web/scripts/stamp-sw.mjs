// Stamps the built service worker with a hash of the bundle's file names, so
// every deploy that changes the app also changes the worker's bytes — the
// browser reinstalls it, skipWaiting hands over control, and running sessions
// get the "new version is ready" toast instead of staying stale for hours.
import { createHash } from 'node:crypto'
import { appendFileSync, readdirSync } from 'node:fs'

const assets = readdirSync('dist/assets').sort().join('\n')
const build = createHash('sha256').update(assets).digest('hex').slice(0, 12)
appendFileSync('dist/firebase-messaging-sw.js', `\n// build ${build}\n`)
console.log(`service worker stamped: ${build}`)
