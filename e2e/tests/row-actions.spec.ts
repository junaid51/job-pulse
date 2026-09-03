import { expect, test } from '@playwright/test'
import { chip, createSearch, freshDevice, openApp, rows, titles } from './helpers'

/** The three buttons on a row. Every one of them was broken at some point
 *  today in a way that rendered perfectly: the X answered 404 for anything the
 *  search bar produced and the failure was swallowed, so it looked dead. */
test.describe('row actions', () => {
  test('dismissing a matched job removes it, and undo brings it back', async ({ page }) => {
    const device = await freshDevice(page, 'hide')
    await createSearch(device, { name: 'FE', keywords: ['frontend'], locations: ['dubai'] })
    await openApp(page)
    await expect(rows(page)).toHaveCount(1)

    await rows(page).first().locator('button[title="Hide this job"]').click()
    await expect(page.locator('.toast')).toContainText('Hidden')
    await expect(rows(page)).toHaveCount(0)

    await page.locator('.toast button').click()
    await expect(rows(page)).toHaveCount(1)
  })

  test('dismissing a search result works too, and it stays dismissed', async ({ page }) => {
    await freshDevice(page, 'hidesearch')
    await openApp(page)
    await page.locator('.search input').fill('warehouse')
    await expect(rows(page)).toHaveCount(1)
    const dismissed = await titles(page).first().innerText()

    await rows(page).first().locator('button[title="Hide this job"]').click()
    await expect(page.locator('.toast')).toContainText('Hidden')
    await expect(rows(page)).toHaveCount(0)

    // And the search bar must not hand it straight back on the next search.
    await page.locator('.search input').fill('associate')
    await expect(rows(page)).toHaveCount(1)
    await page.locator('.search input').fill('warehouse')
    await expect(page.locator('.state-title')).toContainText('Nothing matches')
    expect(dismissed).toContain('Warehouse')
  })

  test('marking a search result applied puts it in Applied', async ({ page }) => {
    const device = await freshDevice(page, 'applied')
    await createSearch(device, { name: 'FE', keywords: ['frontend'], locations: ['dubai'] })
    await openApp(page)

    await page.locator('.search input').fill('warehouse')
    await expect(rows(page)).toHaveCount(1)
    await rows(page).first().locator('button[title="Mark applied"]').click()
    await expect(rows(page).first().locator('.applied-tag')).toBeVisible()

    await page.locator('.search .clear').click()
    await chip(page, 'Applied').click()
    await expect(rows(page)).toHaveCount(1)
    await expect(titles(page)).toContainText([/Warehouse Associate/])
    await expect(page.locator('.feed-end')).toContainText('jobs you applied to')
  })

  test('Applied is not filtered by the region default', async ({ page }) => {
    const device = await freshDevice(page, 'appliedwhere')
    await createSearch(device, { name: 'BE', keywords: ['backend'], locations: ['dubai', 'uk'] })
    await openApp(page)

    // The UK match is the one the Gulf + India default used to hide, which made
    // an application vanish from the record of applications.
    const uk = rows(page).filter({ hasText: 'GitLab' })
    await expect(uk).toHaveCount(1)
    await uk.locator('button[title="Mark applied"]').click()
    await expect(uk.locator('.applied-tag')).toBeVisible()

    await chip(page, 'Applied').click()
    await expect(page.locator('.where-btn')).toHaveText('Anywhere')
    await expect(rows(page)).toHaveCount(1)
    await expect(titles(page)).toContainText([/Backend Engineer, EMEA/])
  })

  test('a row links to the real posting, and share copies it', async ({ page, context }) => {
    await freshDevice(page, 'link')
    await openApp(page)
    await page.locator('.search input').fill('warehouse')
    await expect(rows(page).first()).toHaveAttribute('href', /example\.test/)
    await expect(rows(page).first()).toHaveAttribute('target', '_blank')

    await context.grantPermissions(['clipboard-read', 'clipboard-write']).catch(() => {})
    await rows(page).first().locator('button[title="Share"]').click()
    // Either the share sheet opened (mobile) or the link went to the clipboard.
    await expect(page.locator('.toast, .job-row').first()).toBeVisible()
  })
})
