import { expect, test } from '@playwright/test'
import { readFile } from 'node:fs/promises'

test('architecture explains RAM and Redis decisions without sending upstream traffic', async ({
  page,
}, testInfo) => {
  const upstreamRequests: string[] = []
  const errors: string[] = []
  page.on('request', (request) => {
    if (new URL(request.url()).hostname === 'hiteshsolanki.com')
      upstreamRequests.push(request.url())
  })
  page.on('pageerror', (error) => errors.push(error.message))
  await page.goto('/system-design/rate-limiter/fixed-window')
  await expect(
    page.getByRole('heading', { name: 'From client to backend' }),
  ).toBeVisible()
  await page.getByRole('button', { name: 'Inspect RAM counters' }).click()
  await expect(
    page.getByRole('heading', { name: 'RAM belongs to one gateway' }),
  ).toBeVisible()
  for (let i = 0; i < 3; i++)
    await page.getByRole('button', { name: 'Simulate request' }).click()
  await expect(page.getByRole('status')).toContainText('200 · Forwarded')
  await expect(page.getByTestId('remaining-allowance')).toHaveText(
    '0 / 3 remaining',
  )
  await page.getByRole('button', { name: 'Simulate request' }).click()
  await expect(page.getByRole('status')).toContainText('429 · Rate limited')
  await expect(page.getByRole('status')).toContainText(
    '4 attempted · 3 forwarded',
  )
  await expect(
    page.getByRole('button', { name: 'Inspect Backend service' }),
  ).toContainText('Not reached')
  await page.getByRole('button', { name: 'Reset window' }).click()
  await expect(page.getByTestId('remaining-allowance')).toHaveText(
    '3 / 3 remaining',
  )

  await page.getByText('Redis', { exact: true }).click()
  await expect(
    page.getByRole('radio', { name: 'Redis', exact: true }),
  ).toBeChecked()
  await page.getByRole('button', { name: 'Inspect Redis state' }).click()
  await expect(
    page.getByRole('heading', { name: 'Redis shares the allowance' }),
  ).toBeVisible()
  await page.getByRole('checkbox', { name: 'Redis available' }).uncheck()
  await page.getByRole('button', { name: 'Simulate request' }).click()
  await expect(page.getByRole('status')).toContainText(
    '503 · Redis unavailable',
  )
  await expect(page.getByTestId('remaining-allowance')).toHaveText(
    '3 / 3 remaining',
  )
  await expect(page.getByRole('status')).toContainText('0 forwarded')
  await page.evaluate(() => window.scrollTo(0, 0))
  await page.screenshot({
    path: testInfo.outputPath('redis-outage-desktop.png'),
    fullPage: true,
  })
  await page.getByRole('checkbox', { name: 'Redis available' }).check()
  await page.getByRole('button', { name: 'Simulate request' }).click()
  await expect(page.getByRole('status')).toContainText('200 · Forwarded')
  await expect(
    page.getByText('Historical test against hiteshsolanki.com', {
      exact: false,
    }),
  ).toBeVisible()
  expect(upstreamRequests).toEqual([])
  expect(errors).toEqual([])
})

test('architecture fits a mobile viewport and supports keyboard storage selection', async ({
  page,
}, testInfo) => {
  await page.setViewportSize({ width: 390, height: 844 })
  await page.goto('/system-design/rate-limiter/fixed-window')
  await page.getByRole('radio', { name: 'RAM', exact: true }).focus()
  await page.keyboard.press('ArrowRight')
  await expect(
    page.getByRole('radio', { name: 'Redis', exact: true }),
  ).toBeChecked()
  await expect(
    page.getByRole('button', { name: 'Inspect Redis state' }),
  ).toBeVisible()
  const fits = await page.evaluate(
    () => document.documentElement.scrollWidth <= window.innerWidth,
  )
  expect(fits).toBe(true)
  await page.evaluate(() => window.scrollTo(0, 0))
  await page.screenshot({
    path: testInfo.outputPath('architecture-mobile.png'),
    fullPage: true,
  })
})

test('custom website and multiple identity policies update the model and export deployable JSON', async ({
  page,
}, testInfo) => {
  await page.goto('/system-design/rate-limiter/fixed-window')
  await page.getByLabel('Backend website URL').fill('https://example.com/api')
  await page
    .getByRole('combobox', { name: 'Limit by · rule 1', exact: true })
    .selectOption('api_key')
  await page
    .getByLabel('Requests per window · rule 1', { exact: true })
    .fill('2')
  await page
    .getByLabel('Window in seconds · rule 1', { exact: true })
    .fill('60')
  await page.getByRole('button', { name: 'Add policy rule' }).click()
  await page
    .getByRole('combobox', { name: 'Limit by · rule 2', exact: true })
    .selectOption('user')
  await page
    .getByLabel('Requests per window · rule 2', { exact: true })
    .fill('1')
  await page.getByRole('button', { name: 'Apply to diagram' }).click()
  await expect(
    page.getByRole('button', { name: 'Inspect Backend service' }),
  ).toContainText('example.com/api')
  await expect(
    page.getByRole('button', { name: 'Inspect Rate limiter + proxy' }),
  ).toContainText('2 requests / 60 seconds')
  await page.getByRole('button', { name: 'Simulate request' }).click()
  await page.getByRole('button', { name: 'Simulate request' }).click()
  await page.getByRole('button', { name: 'Simulate request' }).click()
  await expect(page.getByRole('status')).toContainText('429 · Rate limited')
  await page
    .getByLabel('Example api credential id', { exact: true })
    .fill('credential-other')
  await page.getByRole('button', { name: 'Simulate request' }).click()
  await expect(page.getByRole('status')).toContainText('200 · Forwarded')
  await expect(page.getByTestId('remaining-allowance')).toHaveText(
    '1 / 2 remaining',
  )
  await page
    .getByLabel('Example api credential id', { exact: true })
    .fill('credential-7')
  await expect(page.getByTestId('remaining-allowance')).toHaveText(
    '0 / 2 remaining',
  )
  await page
    .getByRole('combobox', { name: 'Simulate policy', exact: true })
    .selectOption('1')
  await page.getByRole('button', { name: 'Simulate request' }).click()
  await expect(page.getByTestId('remaining-allowance')).toHaveText(
    '0 / 1 remaining',
  )
  await page
    .getByRole('combobox', { name: 'Simulate policy', exact: true })
    .selectOption('0')
  await expect(page.getByTestId('remaining-allowance')).toHaveText(
    '0 / 2 remaining',
  )

  const downloading = page.waitForEvent('download')
  await page.getByRole('button', { name: 'Download proxy.json' }).click()
  const download = await downloading
  const output = testInfo.outputPath('proxy.json')
  await download.saveAs(output)
  const config = JSON.parse(await readFile(output, 'utf8'))
  expect(config.version).toBe(1)
  expect(config.upstream).toBe('https://example.com/api')
  expect(config.storage.mode).toBe('ram')
  expect(config.routes).toHaveLength(2)
  expect(config.routes[0]).toMatchObject({
    limit: 2,
    window: '60s',
    policy: { identity: 'api_key' },
  })
  expect(config.routes[1].policy.identity).toBe('user')
  expect(config).not.toHaveProperty('credentials')
  await page.evaluate(() => window.scrollTo(0, 0))
  await page.screenshot({
    path: testInfo.outputPath('custom-policies-desktop.png'),
    fullPage: true,
  })
})

test('invalid configuration is rejected and identity capacity preserves existing counters', async ({
  page,
}) => {
  await page.goto('/system-design/rate-limiter/fixed-window')
  await page.getByLabel('Backend website URL').fill('javascript:alert(1)')
  await page.getByRole('button', { name: 'Apply to diagram' }).click()
  await expect(
    page
      .getByRole('region', { name: 'Your website & policies' })
      .getByRole('alert'),
  ).toContainText('HTTP or HTTPS')
  await expect(
    page.getByRole('button', { name: 'Inspect Backend service' }),
  ).toContainText('hiteshsolanki.com')
  await page.getByLabel('Backend website URL').fill('https://example.com')
  await page
    .getByLabel('Requests per window · rule 1', { exact: true })
    .fill('0')
  await page.getByRole('button', { name: 'Apply to diagram' }).click()
  await expect(
    page
      .getByRole('region', { name: 'Your website & policies' })
      .getByRole('alert'),
  ).toContainText('positive integer')
  await page
    .getByLabel('Requests per window · rule 1', { exact: true })
    .fill('2')
  await page
    .getByLabel('Maximum identities · rule 1', { exact: true })
    .fill('1')
  await page.getByRole('button', { name: 'Apply to diagram' }).click()
  await page.getByRole('button', { name: 'Simulate request' }).click()
  await page
    .getByLabel('Example ip address', { exact: true })
    .fill('203.0.113.11')
  await page.getByRole('button', { name: 'Simulate request' }).click()
  await expect(page.getByRole('status')).toContainText(
    '503 · Identity capacity reached',
  )
  await page
    .getByLabel('Example ip address', { exact: true })
    .fill('203.0.113.10')
  await expect(page.getByTestId('remaining-allowance')).toHaveText(
    '1 / 2 remaining',
  )
  await page.getByRole('button', { name: 'Simulate request' }).click()
  await expect(page.getByRole('status')).toContainText('200 · Forwarded')
  await page.getByRole('button', { name: 'Reset window' }).click()
  await page.getByLabel('Example ip address', { exact: true }).fill('invalid')
  await page.getByRole('button', { name: 'Simulate request' }).click()
  await expect(
    page
      .getByRole('region', { name: 'Walk through a request' })
      .getByRole('alert'),
  ).toContainText('valid example IPv4 or IPv6')
  await page
    .getByRole('combobox', { name: 'Storage', exact: true })
    .selectOption('redis')
  await expect(
    page.getByRole('radio', { name: 'Redis', exact: true }),
  ).toBeChecked()
  await page.getByRole('button', { name: 'Add policy rule' }).click()
  await page.getByRole('button', { name: 'Remove rule 2' }).click()
  await expect(page.getByRole('button', { name: 'Remove rule 2' })).toHaveCount(
    0,
  )
})
