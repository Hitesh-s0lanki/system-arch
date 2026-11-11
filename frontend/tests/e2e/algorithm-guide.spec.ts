import { expect, test } from '@playwright/test'

const path = '/system-design/rate-limiter/algorithms'

test('visual guide has short steps, working algorithm tabs and an expandable diagram', async ({
  page,
}, testInfo) => {
  const errors: string[] = []
  page.on('pageerror', (error) => errors.push(error.message))
  await page.goto(path)
  const article = page.getByRole('article')
  await expect(
    article.getByRole('heading', { name: 'Fixed Window Counter', exact: true }),
  ).toBeVisible()
  expect((await article.innerText()).split(/\s+/).length).toBeLessThan(240)
  await expect(
    page.getByRole('heading', { name: 'Measured latency', exact: true }),
  ).toHaveCount(0)
  await page
    .getByRole('tab', { name: 'Sliding Window Counter', exact: true })
    .click()
  await expect(article).toContainText('80 × 0.75 + 30 = 90')
  await expect(article.getByRole('listitem')).toHaveCount(3)
  await article
    .getByRole('button', { name: 'Expand Sliding Window Counter diagram' })
    .click()
  const dialog = page.getByRole('dialog')
  await expect(dialog).toBeVisible()
  await expect(dialog.getByRole('img')).toHaveAttribute(
    'src',
    '/algorithms/sliding-counter.svg',
  )
  await page.keyboard.press('Escape')
  await expect(dialog).toHaveCount(0)
  await expect(
    article.getByRole('button', {
      name: 'Expand Sliding Window Counter diagram',
    }),
  ).toBeFocused()
  await page.getByRole('tab', { name: 'Token Bucket', exact: true }).focus()
  await page.keyboard.press('ArrowRight')
  await page.keyboard.press('Enter')
  await expect(
    page.getByRole('tab', { name: 'Leaky Bucket Meter', exact: true }),
  ).toHaveAttribute('aria-selected', 'true')
  await expect(article).toContainText('does not queue')
  await page.getByRole('tab', { name: 'Compare', exact: true }).click()
  await page.getByRole('button', { name: /Sliding log/ }).click()
  await expect(
    article.getByRole('heading', { name: 'Sliding Window Log', exact: true }),
  ).toBeVisible()
  await expect(article.getByRole('img')).toHaveJSProperty('complete', true)
  expect(
    await article
      .getByRole('img')
      .evaluate((img: HTMLImageElement) => img.naturalWidth),
  ).toBeGreaterThan(0)
  await page.screenshot({
    path: testInfo.outputPath('guide-desktop.png'),
    fullPage: true,
  })
  expect(errors).toEqual([])
})

test('guide and expanded image stay usable at 320px', async ({
  page,
}, testInfo) => {
  await page.setViewportSize({ width: 320, height: 844 })
  await page.goto(path)
  await page
    .getByRole('tab', { name: 'Sliding Window Counter', exact: true })
    .click()
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true)
  await page
    .getByRole('button', { name: 'Expand Sliding Window Counter diagram' })
    .click()
  const dialog = page.getByRole('dialog')
  await expect(dialog).toBeVisible()
  const imageRegion = dialog.getByRole('region', { name: 'Full size diagram' })
  expect(
    await imageRegion.evaluate((el) => el.scrollWidth > el.clientWidth),
  ).toBe(true)
  await dialog.getByRole('button', { name: 'Close', exact: true }).click()
  await expect(dialog).toHaveCount(0)
  await page.screenshot({
    path: testInfo.outputPath('guide-mobile.png'),
    fullPage: true,
  })
})

test('index navigates across views, settings match Docker and Go, and files download', async ({
  page,
}, testInfo) => {
  await page.goto(path)
  const index = page.getByRole('navigation', {
    name: 'On this page',
    exact: true,
  })
  await expect(index).toBeVisible()
  await page
    .getByRole('tab', { name: 'Sliding Window Counter', exact: true })
    .click()
  await index
    .getByRole('link', { name: 'Docker configuration', exact: true })
    .click()
  const docker = page.getByRole('region', {
    name: 'Docker configuration',
    exact: true,
  })
  await expect(
    docker.getByRole('heading', { name: 'Run the Docker image' }),
  ).toBeVisible()
  await expect(page).toHaveURL(/#docker$/)
  await expect(
    index.getByRole('link', { name: 'Docker configuration', exact: true }),
  ).toHaveAttribute('aria-current', 'location')
  await docker.getByLabel('Example storage').selectOption('redis')
  await docker.getByLabel('Requests per window', { exact: true }).fill('25')
  await docker.getByLabel('Window (seconds)', { exact: true }).fill('30')
  await docker.getByLabel('Maximum identities', { exact: true }).fill('250')
  const downloading = page.waitForEvent('download')
  await docker
    .getByRole('button', { name: 'Download proxy JSON', exact: true })
    .click()
  const download = await downloading
  const output = testInfo.outputPath('proxy.algorithm.json')
  await download.saveAs(output)
  const { readFile } = await import('node:fs/promises')
  const config = JSON.parse(await readFile(output, 'utf8'))
  expect(config.storage.mode).toBe('redis')
  expect(config.routes[0]).toMatchObject({
    algorithm: 'sliding_window_counter',
    limit: 25,
    window: '30s',
    max_keys: 250,
  })
  expect(config.routes[0]).not.toHaveProperty('capacity')
  await docker.getByRole('tab', { name: 'Compose YAML', exact: true }).click()
  await expect(
    docker.getByLabel('compose.algorithm.yaml', { exact: true }),
  ).toContainText('image: rate-limiter:local')
  await expect(
    docker.getByLabel('compose.algorithm.yaml', { exact: true }),
  ).toContainText('127.0.0.1:6380:6379')
  await index.getByRole('link', { name: 'Go SDK', exact: true }).click()
  const sdk = page.getByRole('region', { name: 'Go SDK example', exact: true })
  await expect(sdk.getByLabel('main.go', { exact: true })).toContainText(
    'sdk.NewSlidingCounterRedis',
  )
  await expect(sdk.getByLabel('main.go', { exact: true })).toContainText(
    'Limit: 25, Window: 30*time.Second, MaxKeys: 250',
  )
  await expect(sdk.getByLabel('main.go', { exact: true })).toContainText(
    'options.MaxRetries = -1',
  )
  await page.getByRole('tab', { name: 'Token Bucket', exact: true }).click()
  await sdk.getByLabel('Burst capacity', { exact: true }).fill('7')
  await expect(sdk.getByLabel('main.go', { exact: true })).toContainText(
    'Capacity: 7',
  )
  await expect(sdk.getByLabel('main.go', { exact: true })).toContainText(
    'sdk.NewTokenBucketRedis',
  )
  await index
    .getByRole('link', { name: 'Docker configuration', exact: true })
    .click()
  await expect(
    docker.getByLabel('proxy.algorithm.json', { exact: true }),
  ).toContainText('"capacity": 7')
  await docker.getByLabel('Rate per window', { exact: true }).fill('0')
  await expect(docker.getByRole('alert')).toContainText('whole number')
  await expect(
    docker.getByRole('button', { name: 'Download proxy JSON', exact: true }),
  ).toHaveCount(0)
  await docker.getByLabel('Rate per window', { exact: true }).fill('25')
  await expect(
    index.getByRole('link', { name: 'Docker configuration', exact: true }),
  ).toHaveAttribute('aria-current', 'location')
  await page.screenshot({
    path: testInfo.outputPath('docker-desktop.png'),
    fullPage: true,
  })
})

test('mobile index and direct links open the right view without page overflow', async ({
  page,
}, testInfo) => {
  await page.setViewportSize({ width: 320, height: 844 })
  await page.goto(`${path}#sdk`)
  await expect(
    page.getByRole('heading', { name: 'Use the Go SDK', exact: true }),
  ).toBeVisible()
  await page.locator('summary').filter({ hasText: 'On this page' }).click()
  const index = page.getByRole('navigation', {
    name: 'On this page mobile',
    exact: true,
  })
  await index
    .getByRole('link', { name: 'Docker configuration', exact: true })
    .click()
  await expect(
    page.getByRole('heading', { name: 'Run the Docker image', exact: true }),
  ).toBeVisible()
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true)
  await page.screenshot({
    path: testInfo.outputPath('docker-mobile.png'),
    fullPage: true,
  })
})
