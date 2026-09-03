import { type Page, expect } from '@playwright/test'

export const API = process.env.E2E_API ?? 'http://localhost:8091'

/** A device id nothing else has used, so every test starts with no saved
 *  searches, nothing seen, nothing hidden. It has to be in place before the
 *  bundle runs: api.ts mints one at module load. */
export async function freshDevice(page: Page, name: string): Promise<string> {
  const device = `e2e-${name}-${Date.now()}-${Math.random().toString(36).slice(2, 7)}`
  await page.addInitScript((id) => {
    localStorage.clear()
    localStorage.setItem('jobpulse-device', id)
  }, device)
  return device
}

export async function openApp(page: Page) {
  await page.goto('/')
  // The splash covers the app for ~600ms by design, and the first paint may be
  // skeleton rows. first() matters: several of these can be on screen at once,
  // and a locator that resolves to more than one element is an error, not a
  // wait.
  await expect(page.locator('.chips, .state-title, .job-row').first())
    .toBeVisible({ timeout: 20_000 })
}

/** Saved searches are created through the API rather than the editor, except in
 *  the test that is about the editor: a flow should fail for its own reason. */
export async function createSearch(device: string, body: {
  name: string; keywords: string[]; locations: string[]; remote_only?: boolean
}) {
  const response = await fetch(`${API}/api/profiles`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', 'X-Device': device },
    body: JSON.stringify({ remote_only: false, ...body }),
  })
  if (!response.ok) throw new Error(`creating "${body.name}": ${response.status}`)
  return (await response.json()).profile.id as number
}

export const rows = (page: Page) => page.locator('.job-row')
export const titles = (page: Page) => rows(page).locator('.job-title')
export const chip = (page: Page, label: string) =>
  page.locator('.chip', { hasText: label }).first()
export const whereButton = (page: Page) => page.locator('.where-btn')

/** No screen of a phone-first app may scroll sideways. This is the assertion
 *  the Settings list failed: a saved search with eight places rendered one
 *  unbroken line. */
export async function expectNoSidewaysScroll(page: Page) {
  const overflow = await page.evaluate(() => {
    const d = document.documentElement
    const widest = [...document.querySelectorAll('body *')]
      .filter((el) => {
        const box = el.getBoundingClientRect()
        return box.width > 0 && box.right > d.clientWidth + 1
      })
      .map((el) => `${el.tagName.toLowerCase()}.${String(el.className).trim()}`)
    return { scroll: d.scrollWidth, client: d.clientWidth, widest: widest.slice(0, 5) }
  })
  expect(overflow.scroll, `page scrolls sideways; past the right edge: ${overflow.widest.join(', ')}`)
    .toBeLessThanOrEqual(overflow.client + 1)
}
