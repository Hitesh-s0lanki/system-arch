import { expect, test } from '@playwright/test'
import { readFile } from 'node:fs/promises'
import { readFileSync } from 'node:fs'

const reportPath = 'public/benchmarks/rate-limiter.json'
const report = JSON.parse(readFileSync(reportPath, 'utf8'))
const path = '/system-design/rate-limiter/algorithms'
const formatLatency = (us: number) =>
  us >= 1000 ? `${(us / 1000).toFixed(2)} ms` : `${us.toFixed(3)} µs`

test('saved measurements match the page, filters, selected algorithm and downloaded JSON', async ({
  page,
}, testInfo) => {
  const errors: string[] = []
  page.on('pageerror', (error) => errors.push(error.message))
  await page.goto(path)
  await page.getByRole('tab', { name: 'Benchmarks', exact: true }).click()
  const section = page.locator('#latency')
  await expect(section.getByRole('table')).toBeVisible()
  await expect(section.getByRole('row')).toHaveCount(11)
  const initial = report.results.find(
    (row: {
      algorithm: string
      backend: string
      scenario: string
      workers: number
    }) =>
      row.algorithm === 'fixed_window' &&
      row.backend === 'redis' &&
      row.scenario === 'allowed-hot' &&
      row.workers === 1,
  )
  const initialRow = section
    .getByRole('row')
    .filter({ hasText: 'Fixed window' })
    .filter({ hasText: 'Redis' })
  await expect(initialRow).toContainText(formatLatency(initial.p95_us))
  await expect(section.getByLabel('Selected algorithm latency')).toContainText(
    'Fixed window · Redis',
  )
  await page
    .getByRole('tab', { name: 'Sliding Window Counter', exact: true })
    .click()
  await expect(section.getByLabel('Selected algorithm latency')).toContainText(
    'Sliding counter · RAM',
  )
  await section
    .getByLabel('Workload', { exact: true })
    .selectOption('http-denied')
  await section
    .getByLabel('Concurrent workers', { exact: true })
    .selectOption('16')
  await expect(section).toContainText('rejected requests stop at the proxy')
  const filtered = report.results.find(
    (row: {
      algorithm: string
      backend: string
      scenario: string
      workers: number
    }) =>
      row.algorithm === 'sliding_window_counter' &&
      row.backend === 'redis' &&
      row.scenario === 'http-denied' &&
      row.workers === 16,
  )
  const filteredRow = section
    .getByRole('row')
    .filter({ hasText: 'Sliding counter' })
    .filter({ hasText: 'Redis' })
  await expect(filteredRow).toContainText(formatLatency(filtered.p99_us))
  await section
    .getByText('Benchmark method and settings', { exact: true })
    .click()
  await expect(section).toContainText(report.methodology)
  const downloading = page.waitForEvent('download')
  await section.getByRole('link', { name: 'Download results JSON' }).click()
  const download = await downloading
  const output = testInfo.outputPath('measurements.json')
  await download.saveAs(output)
  expect(JSON.parse(await readFile(output, 'utf8'))).toEqual(report)
  await section.screenshot({ path: testInfo.outputPath('latency-desktop.png') })
  expect(errors).toEqual([])
})

test('unavailable or malformed reports show recovery rather than fabricated metrics', async ({
  page,
}) => {
  let responseMode = 'unavailable'
  await page.route('**/benchmarks/rate-limiter.json', async (route) => {
    if (responseMode === 'unavailable')
      return route.fulfill({ status: 503, body: 'Unavailable' })
    if (responseMode === 'malformed')
      return route.fulfill({ json: { schema_version: 1, results: [] } })
    return route.fulfill({ json: report })
  })
  await page.goto(path)
  await page.getByRole('tab', { name: 'Benchmarks', exact: true }).click()
  const section = page.locator('#latency')
  await expect(section.getByRole('alert')).toBeVisible()
  await expect(section.getByRole('table')).toHaveCount(0)
  responseMode = 'malformed'
  await section.getByRole('button', { name: 'Retry loading results' }).click()
  await expect(section.getByRole('alert')).toBeVisible()
  await expect(section.getByRole('table')).toHaveCount(0)
  responseMode = 'valid'
  await section.getByRole('button', { name: 'Retry loading results' }).click()
  await expect(section.getByRole('table')).toBeVisible()
})

test('latency controls and comparison fit narrow screens and work with a keyboard', async ({
  page,
}, testInfo) => {
  await page.setViewportSize({ width: 320, height: 844 })
  await page.goto(path)
  await page.getByRole('tab', { name: 'Benchmarks', exact: true }).click()
  const section = page.locator('#latency')
  await expect(section.getByRole('table')).toBeVisible()
  const workers = section.getByLabel('Concurrent workers', { exact: true })
  await workers.selectOption('16')
  await expect(workers).toHaveValue('16')
  await expect(section.getByRole('table')).toContainText(
    '16 concurrent workers',
  )
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth,
    ),
  ).toBe(true)
  const scrollRegion = section.getByRole('region', {
    name: 'Latency comparison table',
  })
  await workers.focus()
  await page.keyboard.press('Tab')
  await expect(
    section.getByRole('button', { name: 'Show all metrics' }),
  ).toBeFocused()
  await page.keyboard.press('Enter')
  await expect(
    section.getByRole('columnheader', { name: 'Maximum', exact: true }),
  ).toBeVisible()
  await page.keyboard.press('Tab')
  await expect(scrollRegion).toBeFocused()
  await page.keyboard.press('End')
  await page.keyboard.press('Tab')
  const method = section.getByText('Benchmark method and settings', {
    exact: true,
  })
  await expect(method).toBeFocused()
  await page.keyboard.press('Enter')
  await expect(
    section.getByText(report.methodology, { exact: true }),
  ).toBeVisible()
  await section.screenshot({ path: testInfo.outputPath('latency-mobile.png') })
})
