import { expect, test } from '@playwright/test'

// Plan §9 (minus studio hours, removed by the owner on 2026-10-31): add 3
// pieces → check-in → trim one, then undo → the trim card shows a visit.
test('daily loop on a phone', async ({ page }) => {
  await page.goto('/')
  await expect(page.getByTestId('trim-card')).toHaveAttribute('data-kind', 'empty')
  await expect(page.getByTestId('studio-banner')).toHaveCount(0)

  // Time zone (the only schedule-ish setting left)
  await page.goto('/settings')
  await page.getByTestId('tz-select').selectOption('Europe/London')
  await page.getByTestId('save-tz').click()
  await expect(page.getByText('Time zone saved')).toBeVisible()

  // Add 3 pieces
  await page.goto('/')
  await page.getByTestId('fab-add').click()
  await page.getByRole('dialog', { name: 'Add pieces' }).waitFor()
  await page.getByRole('button', { name: 'One more' }).click()
  await page.getByRole('button', { name: 'One more' }).click()
  await page.getByTestId('add-submit').click()
  await expect(page.getByText('#1 #2 #3')).toBeVisible()
  await expect(page.getByRole('dialog')).toHaveCount(0)

  // Check-in: save the first piece unchanged, then trim the next one and undo
  await page.getByTestId('check-pieces').click()
  await expect(page.getByTestId('check-card')).toBeVisible()
  await expect(page.getByText('same as last time?')).toBeVisible()
  await page.getByTestId('save-next').click()
  await expect(page.getByText(/^2 of 3$/)).toBeVisible()
  await page.getByRole('button', { name: 'Trim', exact: true }).click()
  await page.getByTestId('confirm-trim').click()
  const toast = page.getByRole('status').filter({ hasText: 'trimmed' })
  await expect(toast).toBeVisible()
  await toast.getByRole('button', { name: 'Undo' }).click()
  await expect(page.getByText('Undone')).toBeVisible()

  // Back home: the trim card recommends a visit
  await page.getByRole('button', { name: 'End check-in' }).click()
  await page.goto('/')
  await expect(page.getByTestId('trim-card')).toHaveAttribute('data-kind', /next_visit|ready_now/)
  await expect(page.getByTestId('trim-card')).toContainText(/Come|ready now/)
})

test('slider: keyboard and tap', async ({ page }) => {
  await page.goto('/dev/slider')
  const track = page.getByRole('slider', { name: 'Test slider' })
  await track.scrollIntoViewIfNeeded()
  const box = (await track.boundingBox())!
  const before = await track.getAttribute('aria-valuenow')
  await track.focus()
  await page.keyboard.press('ArrowRight')
  await expect(track).toHaveAttribute('aria-valuenow', String(Math.round((Number(before) + 0.1) * 10) / 10))
  await page.mouse.click(box.x + box.width * 0.8, box.y + box.height / 2)
  expect(Number(await track.getAttribute('aria-valuenow'))).toBeGreaterThan(3.5)
})

test('slider: a vertical touch swipe scrolls the page and never changes the value; a sideways one does', async ({ page }) => {
  await page.goto('/dev/slider')
  const track = page.getByRole('slider', { name: 'Test slider' })
  await track.scrollIntoViewIfNeeded()
  const box = (await track.boundingBox())!
  const cdp = await page.context().newCDPSession(page)
  const touch = async (type: 'touchStart' | 'touchMove' | 'touchEnd', x: number, y: number) =>
    cdp.send('Input.dispatchTouchEvent', { type, touchPoints: type === 'touchEnd' ? [] : [{ x, y }] })
  const cx = box.x + box.width * 0.7
  const cy = box.y + box.height / 2
  const scrollBefore = await page.evaluate(() => window.scrollY)

  // Vertical swipe starting on the track, with a little sideways wobble.
  await touch('touchStart', cx, cy)
  for (let i = 1; i <= 10; i++) await touch('touchMove', cx + (i % 2) * 3, cy - i * 15)
  await touch('touchEnd', cx, cy - 150)
  await page.waitForTimeout(300)
  await expect(page.getByTestId('dev-changes')).toHaveText('0')
  expect(await page.evaluate(() => window.scrollY)).toBeGreaterThan(scrollBefore)

  // Sideways drag moves it.
  const b2 = (await track.boundingBox())!
  const y2 = b2.y + b2.height / 2
  await touch('touchStart', b2.x + b2.width * 0.3, y2)
  for (let i = 1; i <= 10; i++) await touch('touchMove', b2.x + b2.width * 0.3 + i * 12, y2 + 1)
  await touch('touchEnd', b2.x + b2.width * 0.3 + 120, y2)
  await expect(page.getByTestId('dev-changes')).not.toHaveText('0')
})
