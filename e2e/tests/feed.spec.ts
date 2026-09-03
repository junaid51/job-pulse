import { expect, test } from '@playwright/test'
import { chip, createSearch, freshDevice, openApp, rows, titles, whereButton } from './helpers'

test.describe('the feed and its filters', () => {
  test('a saved search answers "where" itself, and widening asks the corpus', async ({ page }) => {
    const device = await freshDevice(page, 'where')
    await createSearch(device, {
      name: 'Frontend', keywords: ['frontend'],
      locations: ['dubai', 'abu dhabi', 'gulf', 'uae', 'saudi', 'qatar'],
    })
    await openApp(page)

    // Its own places, not the global Gulf + India default: that default used to
    // overrule a saved search and hide matches it had legitimately caught.
    await expect(whereButton(page)).toHaveText(/dubai \+5/)
    await expect(rows(page)).toHaveCount(2) // Dubai and Riyadh; not Bengaluru
    await expect(page.locator('.feed-end')).toContainText('your “Frontend” matches')

    await whereButton(page).click()
    // Nothing may look selected while the search owns the answer.
    await expect(page.locator('.sheet .pick.on')).toHaveCount(0)
    await expect(page.locator('.sheet small').first()).toContainText('“Frontend” watches dubai')

    await page.locator('.sheet .pick', { hasText: 'Anywhere' }).click()
    await page.locator('.sheet .btn-filled').click()

    // A saved search's match list holds nothing outside its places, so widening
    // has to ask the whole corpus for the same keywords — otherwise "Anywhere"
    // changes nothing, which is what it used to do.
    await expect(page.locator('.notice.quiet')).toContainText('every job for “Frontend”')
    await expect(page.locator('.feed-end')).toContainText('every job matching “Frontend”')
    await expect(rows(page)).toHaveCount(4)
    await expect(titles(page)).toContainText([/Frontend Engineer/])
  })

  test('the remote switch does not quietly reassign where', async ({ page }) => {
    const device = await freshDevice(page, 'remote')
    await createSearch(device, {
      name: 'Frontend', keywords: ['frontend'], locations: ['dubai', 'saudi'],
    })
    await openApp(page)
    await expect(whereButton(page)).toHaveText(/dubai · saudi/)

    await whereButton(page).click()
    await page.locator('.sheet .switch-row input').click()
    await expect(whereButton(page)).toHaveText(/dubai · saudi · remote/)
    await page.locator('.sheet .switch-row input').click()
    await expect(whereButton(page)).toHaveText(/^dubai · saudi$/)
  })

  test('a place is a whole word, in the filter and in the search bar', async ({ page }) => {
    await freshDevice(page, 'boundaries')
    await openApp(page)

    // "Indianapolis, Indiana" is in the corpus and must not answer for India.
    await page.locator('.search input').fill('frontend @india')
    await expect(rows(page)).toHaveCount(1)
    await expect(titles(page)).toHaveText([/Frontend Engineer/])
    await expect(rows(page).locator('.job-meta')).toContainText('Bengaluru')

    // "oci" inside Associate and "product" inside Production are the same
    // mistake in a title.
    await page.locator('.search input').fill('oci')
    await expect(page.locator('.state-title')).toContainText('Nothing matches')
    await page.locator('.search input').fill('product')
    await expect(page.locator('.state-title')).toContainText('Nothing matches')

    // But a plural is the same word.
    await page.locator('.search input').fill('platform')
    await expect(rows(page)).toHaveCount(2)
  })

  test('a Gulf search never answers with Romania', async ({ page }) => {
    const device = await freshDevice(page, 'romania')
    await createSearch(device, { name: 'Devops · Gulf', keywords: ['devops'], locations: ['gulf'] })
    await openApp(page)
    // "gulf" expands to "oman", and R-oman-ia contains it. This search used to
    // notify its owner about Romania.
    await expect(page.locator('.state-title')).toContainText('Nothing matched')
  })

  test('the search bar names every filter it applied when nothing matches', async ({ page }) => {
    await freshDevice(page, 'empty')
    await openApp(page)
    await page.locator('.search input').fill('kubernetes @dubai')
    await expect(page.locator('.state-title')).toContainText('in dubai')
    await page.locator('.search input').fill('frontend devops')
    await expect(page.locator('.state-title')).toContainText('No job matches all of those words')
    await page.locator('.state .btn-tonal').click() // "Search … instead" drops the last word
    await expect(rows(page)).toHaveCount(3)
  })

  test('exclusions, and the keyboard shortcut', async ({ page }) => {
    await freshDevice(page, 'exclude')
    await openApp(page)
    await page.locator('.search input').fill('frontend')
    await expect(rows(page)).toHaveCount(3)
    await page.locator('.search input').fill('frontend -senior')
    await expect(rows(page)).toHaveCount(2)

    await page.locator('.search .clear').click()
    await page.locator('body').press('/')
    await expect(page.locator('.search input')).toBeFocused()
  })

  test('chips switch scope, and each hands "where" back to its own search', async ({ page }) => {
    const device = await freshDevice(page, 'chips')
    await createSearch(device, { name: 'Gulf FE', keywords: ['frontend'], locations: ['dubai'] })
    await createSearch(device, { name: 'India FE', keywords: ['frontend'], locations: ['india'] })
    await openApp(page)

    await expect(chip(page, 'All searches')).toBeVisible()
    await chip(page, 'Gulf FE').click()
    await expect(whereButton(page)).toHaveText(/dubai/)
    await expect(rows(page)).toHaveCount(1)

    await chip(page, 'India FE').click()
    await expect(whereButton(page)).toHaveText(/india/)
    await expect(rows(page)).toHaveCount(1)
    await expect(rows(page).locator('.job-meta')).toContainText('Bengaluru')

    await chip(page, 'All searches').click()
    await expect(rows(page)).toHaveCount(2)
  })
})
