import { expect, test } from '@playwright/test'
import { expectNoSidewaysScroll, freshDevice, openApp, rows, titles } from './helpers'

test.describe('a device with no history', () => {
  test('offers a first search, and the search bar works before one exists', async ({ page }) => {
    await freshDevice(page, 'firstrun')
    await openApp(page)

    await expect(page.locator('.state-title')).toHaveText('Start with a search')
    await expect(page.locator('.chips')).toHaveCount(0)

    // The bar covers every board even with nothing saved — that is the promise
    // the empty state makes.
    await page.locator('.search input').fill('frontend')
    // Dubai, Riyadh and Bengaluru. The fourth "Frontend Engineer" in the corpus
    // is in Indianapolis, and the Gulf + India default is what leaves it out —
    // not a substring accident, which used to let it in.
    await expect(rows(page)).toHaveCount(3)
    await expect(page.locator('.save-search')).toBeVisible()
    await expectNoSidewaysScroll(page)
  })

  test('saves a search from the bar and lands on its feed', async ({ page }) => {
    await freshDevice(page, 'savebar')
    await openApp(page)

    await page.locator('.search input').fill('backend')
    await expect(rows(page)).toHaveCount(1) // the other one is UK-only
    await page.locator('.save-search').click()

    await expect(page.locator('.chip.selected')).toHaveText(/backend/i)
    await expect(page.locator('.toast')).toContainText('Saved')
    await expect(titles(page)).toContainText([/Backend Engineer/])
    // Two, not the one that was on screen: a search saved from the bar takes
    // the typed words and any @place, but not the Gulf + India region filter,
    // which is a global default rather than part of the query. So the saved
    // search watches everywhere and picks up the UK role as well. Worth
    // knowing — it means what you save is broader than what you were reading.
    await expect(rows(page)).toHaveCount(2)
  })

  test('the editor creates a search, and the feed frame names it', async ({ page }) => {
    await freshDevice(page, 'editor')
    await openApp(page)

    await page.locator('.state .btn-tonal, .chip-add').first().click()
    await expect(page.locator('.sheet h2')).toHaveText('New search')
    await page.locator('.sheet .pick', { hasText: 'frontend' }).first().click()
    await page.locator('.sheet .pick', { hasText: 'dubai' }).first().click()
    await page.locator('.sheet .btn-filled').click()

    await expect(page.locator('.chip.selected')).toHaveText(/frontend/i)
    await expect(rows(page)).toHaveCount(1)
    await expect(page.locator('.feed-end')).toContainText('matches')
    await expect(page.locator('.where-btn')).toHaveText(/dubai/)
  })
})
