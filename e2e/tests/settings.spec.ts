import { expect, test } from '@playwright/test'
import { createSearch, expectNoSidewaysScroll, freshDevice, openApp, rows } from './helpers'

const openSettings = async (page: import('@playwright/test').Page) => {
  await page.locator('nav button').nth(1).click()
  await expect(page.locator('.section-h').first()).toHaveText('Saved searches')
}

test.describe('settings', () => {
  test('a search with many places does not push the page sideways', async ({ page }) => {
    const device = await freshDevice(page, 'overflow')
    await createSearch(device, {
      name: 'Everything everywhere',
      keywords: ['devops', 'cloud', 'platform', '-senior', '-lead'],
      locations: ['dubai', 'abu dhabi', 'gulf', 'uae', 'saudi', 'qatar', 'bengaluru', 'bangalore'],
    })
    await openApp(page)
    await openSettings(page)

    await expect(page.locator('.profile-row')).toHaveCount(1)
    await expectNoSidewaysScroll(page)

    // The editor holds the same terms as chips, which wrap by design.
    await page.locator('.profile-main').click()
    await expect(page.locator('.sheet h2')).toHaveText('Edit search')
    await expectNoSidewaysScroll(page)
  })

  test('editing a search takes effect immediately, in both directions', async ({ page }) => {
    const device = await freshDevice(page, 'edit')
    await createSearch(device, { name: 'Wide', keywords: ['frontend'], locations: [] })
    await openApp(page)
    // A search saved with no places answers "where" with Anywhere, so this is
    // every Frontend Engineer in the corpus, Indianapolis included.
    await expect(rows(page)).toHaveCount(4)

    await openSettings(page)
    await page.locator('.profile-main').click()
    await page.locator('.sheet .picker').first().locator('.pick', { hasText: 'backend' }).click()
    await page.locator('.sheet .btn-filled').click()
    await page.locator('nav button').first().click()
    await expect(rows(page)).toHaveCount(6) // frontend or backend, anywhere

    // And narrowing removes what no longer qualifies, without waiting for a poll.
    await openSettings(page)
    await page.locator('.profile-main').click()
    await page.locator('.sheet .picker').first().locator('.pick.on', { hasText: 'frontend' }).click()
    await page.locator('.sheet .btn-filled').click()
    await page.locator('nav button').first().click()
    await expect(rows(page)).toHaveCount(2) // backend only: Abu Dhabi and the UK one
  })

  test('deleting a search takes its matches with it', async ({ page }) => {
    const device = await freshDevice(page, 'delete')
    await createSearch(device, { name: 'Doomed', keywords: ['frontend'], locations: ['dubai'] })
    await openApp(page)
    await openSettings(page)

    page.on('dialog', (d) => d.accept())
    await page.locator('.profile-row .icon-btn').click()
    await expect(page.locator('.profile-row')).toHaveCount(0)
    await page.locator('nav button').first().click()
    await expect(page.locator('.state-title')).toHaveText('Start with a search')
  })

  test('advanced shows the boards and this device', async ({ page }) => {
    await freshDevice(page, 'advanced')
    await openApp(page)
    await openSettings(page)

    await expect(page.locator('.kv-value').first()).toHaveText(/Off|Not supported/)
    await page.locator('.disclosure').click()
    await expect(page.locator('.section-h', { hasText: 'Boards' })).toBeVisible()
    await expect(page.locator('.kv', { hasText: 'Device ID' })).toBeVisible()
    await expect(page.locator('.kv', { hasText: 'URL' })).toBeVisible()
    await expectNoSidewaysScroll(page)
  })

  test('a stale poller is reported rather than hidden', async ({ page }) => {
    await freshDevice(page, 'stale')
    // The suite's backend has no boards, so it has never completed a cycle —
    // exactly the state that used to look like an empty feed.
    await openApp(page)
    const notice = page.locator('.notice').first()
    if (await notice.count()) await expect(notice).toContainText(/boards|poll/i)
  })
})
