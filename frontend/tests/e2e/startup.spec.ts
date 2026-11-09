import { test, expect } from '@playwright/test'

test('local startup button makes Docker and the real load tester ready', async ({
  page,
  request,
}) => {
  test.setTimeout(240000)
  await page.goto('/system-design/load-tester')
  await expect(page.getByText('Connecting…', { exact: true })).not.toBeVisible()
  const connected = page.getByText('Load tester connected', { exact: true })
  if (!(await connected.isVisible())) {
    const start = page.getByRole('button', {
      name: 'Start load tester',
      exact: true,
    })
    await expect(start).toBeEnabled({ timeout: 30000 })
    await start.click()
    await expect(
      page.getByRole('heading', { name: 'Getting ready to test', exact: true }),
    ).toBeVisible()
    await expect(connected).toBeVisible({ timeout: 210000 })
  }
  expect((await request.get('/api/load-tester/ready')).status()).toBe(200)
  const control = await request.get('/api/load-tester/control')
  expect((await control.json()).phase).toBe('ready')
  await expect(
    page.getByRole('button', { name: 'Test my server', exact: true }),
  ).toBeEnabled()
})

test('startup error stays retryable and missing Docker can be checked again', async ({
  page,
}) => {
  await page.route('**/api/load-tester/ready', (route) =>
    route.fulfill({
      status: 503,
      contentType: 'application/json',
      body: '{"error":"Offline"}',
    }),
  )
  await page.route('**/api/load-tester/control', (route) =>
    route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify(
        route.request().method() === 'POST'
          ? {
              phase: 'error',
              message: 'Docker needs attention. Try again.',
              canStart: true,
            }
          : { phase: 'offline', message: 'Docker is stopped.', canStart: true },
      ),
    }),
  )
  await page.goto('/system-design/load-tester')
  const start = page.getByRole('button', {
    name: 'Start load tester',
    exact: true,
  })
  await expect(start).toBeEnabled()
  await start.click()
  await expect(start).toBeEnabled()
  // The status request after starting supplies the current state; no command text is needed.
  await expect(
    page.getByRole('region', { name: 'Start load tester' }),
  ).not.toContainText('docker compose')
  await page.unroute('**/api/load-tester/control')
  await page.route('**/api/load-tester/control', (route) =>
    route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({
        phase: 'offline',
        message: 'Docker was not found on this computer.',
        canStart: false,
      }),
    }),
  )
  await page.reload()
  await expect(
    page.getByRole('button', { name: 'Check again', exact: true }),
  ).toBeEnabled()
})

test('Docker management rejects cross-origin mutations and nonlocal hosts', async ({
  request,
}) => {
  expect(
    (
      await request.post('/api/load-tester/control', {
        headers: { Origin: 'https://unrelated.example' },
      })
    ).status(),
  ).toBe(403)
  expect(
    (
      await request.get('/api/load-tester/control', {
        headers: { Host: 'remote.example' },
      })
    ).status(),
  ).toBe(403)
})
