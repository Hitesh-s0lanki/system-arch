import { test, expect } from '@playwright/test'
import { createServer, type Server } from 'node:http'
import { execFileSync } from 'node:child_process'

let fixture: Server
let target: string
const arrivals: number[] = []

test.beforeAll(async () => {
  fixture = createServer((_req, res) => {
    const now = Date.now()
    while (arrivals.length && arrivals[0] < now - 1000) arrivals.shift()
    const limited = arrivals.length >= 7
    if (!limited) arrivals.push(now)
    setTimeout(() => {
      res.writeHead(limited ? 429 : 200, { 'Content-Type': 'text/plain' })
      res.end('ok')
    }, 5)
  })
  await new Promise<void>((resolve) => fixture.listen(0, '0.0.0.0', resolve))
  const address = fixture.address()
  if (!address || typeof address === 'string')
    throw new Error('Fixture unavailable')
  target = `http://host.docker.internal:${address.port}`
})

test.afterAll(async () => {
  fixture?.closeAllConnections()
  await new Promise<void>((resolve) => fixture?.close(() => resolve()))
})

test('sidebar → real Docker test → suggestion → restore → cancel → compact layout', async ({
  page,
}) => {
  const errors: string[] = []
  page.on('pageerror', (error) => errors.push(error.message))
  await page.goto('/')
  await page.getByRole('link', { name: 'Load Tester', exact: true }).click()
  await expect(page).toHaveURL(/system-design\/load-tester/)
  await expect(
    page.getByRole('link', { name: 'Load Tester', exact: true }),
  ).toHaveAttribute('aria-current', 'page')
  await expect(
    page.getByText('Load tester connected', { exact: true }),
  ).toBeVisible()

  await page
    .getByLabel('Website or server address', { exact: true })
    .fill(target)
  await expect(page.getByLabel('RPS steps', { exact: true })).toHaveCount(0)
  await page
    .getByLabel('Website or server address', { exact: true })
    .fill('ftp://example.com')
  await page
    .getByRole('button', { name: 'Test my server', exact: true })
    .click()
  await expect(
    page.getByRole('alert').filter({ hasText: 'HTTP or HTTPS' }),
  ).toBeVisible()
  await page
    .getByLabel('Website or server address', { exact: true })
    .fill(target)
  await page.getByLabel('Maximum load', { exact: true }).fill('10')
  await page.getByText('Advanced settings', { exact: true }).click()
  await page.getByLabel('Timeout (s)', { exact: true }).fill('1')
  await page.getByLabel('Response time limit (ms)', { exact: true }).fill('500')
  await page.getByText('Advanced settings', { exact: true }).click()
  await page
    .getByRole('button', { name: 'Test my server', exact: true })
    .click()
  await expect(page).toHaveURL(/test=[a-f0-9]{32}/)
  const capacityID = new URL(page.url()).searchParams.get('test')!
  await expect(
    page.getByRole('button', { name: 'Cancel', exact: true }),
  ).toBeVisible()
  await page.reload()
  await expect(
    page.getByRole('button', { name: 'Cancel', exact: true }),
  ).toBeVisible()
  const results = page.getByRole('region', {
    name: 'Test results',
    exact: true,
  })
  await expect(
    results.getByText('Suggested limit', { exact: true }),
  ).toBeVisible({ timeout: 160_000 })
  await expect(results.locator('strong').first()).toHaveText('4')
  await expect(results.getByRole('table')).toContainText('passed')
  await expect(results.getByRole('table')).toContainText('failed')
  await page
    .getByText('Technical details & full report', { exact: true })
    .click()
  await page
    .getByRole('button', { name: 'Execution logs', exact: true })
    .click()
  await expect(results.locator('pre')).toContainText('scenarios')
  const downloadEvent = page.waitForEvent('download')
  await page.getByRole('link', { name: 'Download JSON', exact: true }).click()
  expect((await downloadEvent).suggestedFilename()).toContain(capacityID)
  expect(
    execFileSync(
      'docker',
      [
        'ps',
        '-a',
        '--filter',
        `label=local.load-tester.job=${capacityID}`,
        '--format',
        '{{.ID}}',
      ],
      { encoding: 'utf8' },
    ).trim(),
  ).toBe('')
  await page.screenshot({
    path: 'test-results/load-tester-desktop.png',
    fullPage: true,
  })

  await page.getByText('Advanced settings', { exact: true }).click()
  await page.getByLabel('Test approach', { exact: true }).selectOption('fixed')
  await page.getByLabel('Maximum load', { exact: true }).fill('3')
  await page
    .getByLabel('Website or server address', { exact: true })
    .fill(target)
  await page
    .getByRole('button', { name: 'Test my server', exact: true })
    .click()
  await expect(
    page.getByRole('button', { name: 'Cancel', exact: true }),
  ).toBeVisible()
  await page.getByRole('button', { name: 'Cancel', exact: true }).click()
  await expect(
    page.getByRole('heading', { name: 'Test cancelled', exact: true }),
  ).toBeVisible()
  const cancelledID = new URL(page.url()).searchParams.get('test')!
  expect(
    execFileSync(
      'docker',
      [
        'ps',
        '-a',
        '--filter',
        `label=local.load-tester.job=${cancelledID}`,
        '--format',
        '{{.ID}}',
      ],
      { encoding: 'utf8' },
    ).trim(),
  ).toBe('')

  await page.goto(`/system-design/load-tester?test=${capacityID}`)
  await expect(page.getByText('Suggested limit', { exact: true })).toBeVisible()
  await page.setViewportSize({ width: 390, height: 844 })
  await expect(
    page.getByRole('button', { name: 'Test my server', exact: true }),
  ).toBeVisible()
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth,
    ),
  ).toBe(true)
  await page.screenshot({
    path: 'test-results/load-tester-mobile.png',
    fullPage: true,
  })
  await page.setViewportSize({ width: 720, height: 1000 })
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth,
    ),
  ).toBe(true)
  await page
    .locator('html')
    .evaluate((element) => element.setAttribute('dir', 'rtl'))
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth,
    ),
  ).toBe(true)
  await page.locator('html').evaluate((element) => {
    element.removeAttribute('dir')
    element.style.zoom = '2'
  })
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth,
    ),
  ).toBe(true)
  expect(errors).toEqual([])
})

test('proxy preserves validation and blocks unsupported paths and cross-origin mutations', async ({
  request,
}) => {
  expect((await request.get('/api/load-tester/ready')).status()).toBe(200)
  const invalid = await request.post('/api/load-tester/tests', {
    data: { targetUrl: 'file:///etc/passwd' },
  })
  expect(invalid.status()).toBe(400)
  expect((await request.get('/api/load-tester/arbitrary')).status()).toBe(404)
  expect(
    (
      await request.post('/api/load-tester/tests', {
        headers: { Origin: 'https://unrelated.example' },
        data: {},
      })
    ).status(),
  ).toBe(403)
})

test('short API run still displays insufficient evidence instead of a limit', async ({
  page,
  request,
}) => {
  const response = await request.post('/api/load-tester/tests', {
    data: {
      targetUrl: target,
      requestsPerSecond: 3,
      durationSeconds: 3,
      p95ThresholdMs: 1000,
      requestTimeoutSeconds: 1,
    },
  })
  expect(response.status()).toBe(202)
  const job = await response.json()
  await page.goto(`/system-design/load-tester?test=${job.id}`)
  const results = page.getByRole('region', {
    name: 'Test results',
    exact: true,
  })
  await expect(
    results.getByText('Suggested limit', { exact: true }),
  ).toBeVisible({ timeout: 30000 })
  await expect(results.locator('strong').first()).toHaveText('—')
  await expect(results).toContainText('Not enough passing evidence')
  await expect(results.getByRole('table')).toContainText('inconclusive')
})

test('unavailable service explains recovery and disables starting a test', async ({
  page,
}) => {
  await page.route('**/api/load-tester/ready', (route) =>
    route.fulfill({
      status: 503,
      contentType: 'application/json',
      body: JSON.stringify({ error: 'Docker unavailable' }),
    }),
  )
  await page.goto('/system-design/load-tester')
  await expect(page.getByText('Service offline', { exact: true })).toBeVisible()
  await expect(
    page.getByRole('button', { name: 'Test my server', exact: true }),
  ).toBeDisabled()
  await expect(
    page.getByRole('region', { name: 'Start load tester', exact: true }),
  ).toBeVisible()
})
