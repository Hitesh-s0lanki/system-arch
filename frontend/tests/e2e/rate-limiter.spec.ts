import { expect, test } from '@playwright/test'
import { readFile } from 'node:fs/promises'

const config = {
  algorithm: 'fixed_window',
  storage: 'ram',
  target: 'demo',
  upstream: 'https://example.com',
  identity: 'ip',
  limit: 2,
  windowSeconds: 60,
  capacity: 1,
}
const origin = process.env.FRONTEND_URL || 'http://localhost:3000'

test('hosted controls stay disabled, configuration exports, and mobile layout fits', async ({
  page,
}, testInfo) => {
  await page.route('**/api/rate-limiter/control', (route) =>
    route.fulfill({ status: 403, json: { available: false } }),
  )
  await page.setViewportSize({ width: 390, height: 844 })
  await page.goto('/system-design/rate-limiter/proxy-mode')
  await expect(
    page.getByRole('heading', { name: 'Try the proxy locally', exact: true }),
  ).toBeVisible()
  await expect(
    page.getByRole('button', { name: 'Start Docker' }),
  ).toBeDisabled()
  await expect(
    page.getByRole('button', { name: 'Send request' }),
  ).toBeDisabled()
  await page
    .getByLabel('Algorithm', { exact: true })
    .selectOption('token_bucket')
  await page.getByText('Customize settings', { exact: true }).click()
  await page.getByLabel('Burst capacity').fill('7')
  const downloading = page.waitForEvent('download')
  await page.getByRole('button', { name: 'Config', exact: true }).click()
  const download = await downloading
  const output = testInfo.outputPath('proxy.json')
  await download.saveAs(output)
  const exported = JSON.parse(await readFile(output, 'utf8'))
  expect(exported.routes[0]).toMatchObject({
    algorithm: 'token_bucket',
    capacity: 7,
  })
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true)
  await page.screenshot({
    path: testInfo.outputPath('demo-mobile.png'),
    fullPage: true,
  })
})

test('local endpoints reject remote hosts, cross-origin calls, and malformed configuration', async ({
  request,
}) => {
  expect(
    (
      await request.get('/api/rate-limiter/control', {
        headers: { Host: 'example.com' },
      })
    ).status(),
  ).toBe(403)
  expect(
    (
      await request.post('/api/rate-limiter/control', {
        headers: { Origin: 'https://example.com' },
        data: { action: 'start', config },
      })
    ).status(),
  ).toBe(403)
  expect(
    (
      await request.post('/api/rate-limiter/control', {
        data: { action: 'start', config },
      })
    ).status(),
  ).toBe(403)
  const bad = await request.post('/api/rate-limiter/control', {
    headers: { Origin: origin },
    data: { action: 'start', config: { ...config, algorithm: 'bogus' } },
  })
  expect(bad.status()).toBe(400)
  expect(await bad.json()).toMatchObject({ error: 'Choose an algorithm.' })
})

test('start Docker in the UI, forward requests, enforce limits, and isolate verified users', async ({
  page,
}, testInfo) => {
  const errors: string[] = []
  page.on('pageerror', (error) => errors.push(error.message))
  await page.goto('/system-design/rate-limiter/proxy-mode')
  await page.getByText('Customize settings', { exact: true }).click()
  await page.getByLabel('Requests', { exact: true }).fill('2')
  await page.getByLabel('Window (seconds)').fill('60')
  await page
    .getByRole('button', { name: /Start Docker|Apply & restart Docker/ })
    .click()
  try {
    await expect(
      page.getByText('Docker is ready', { exact: true }),
    ).toBeVisible({ timeout: 150000 })
    for (let i = 0; i < 2; i++) {
      await page.getByRole('button', { name: 'Send request' }).click()
      await expect(
        page.getByRole('button', { name: 'Send request' }),
      ).toBeEnabled()
      await expect(page.getByRole('status')).toContainText('200')
    }
    await page.getByRole('button', { name: 'Send request' }).click()
    await expect(page.getByRole('status')).toContainText('429 · Rate limited')
    await expect(page.getByRole('status')).toContainText('Retry after')
    await page.getByLabel('Limit by', { exact: true }).selectOption('user')
    await expect(
      page.getByRole('button', { name: 'Send request' }),
    ).toBeDisabled()
    await page.getByRole('button', { name: 'Apply & restart Docker' }).click()
    await expect(
      page.getByText('Docker is ready', { exact: true }),
    ).toBeVisible({ timeout: 90000 })
    await page.getByLabel('Method', { exact: true }).selectOption('POST')
    await page.getByLabel('Request path').fill('/orders?source=browser')
    await page.getByText('Headers & request body', { exact: true }).click()
    await page
      .getByLabel('Headers (JSON)')
      .fill('{"Content-Type":"text/plain","X-Demo":"hello"}')
    await page
      .getByLabel('Request body', { exact: true })
      .fill('hello from the browser')
    await page.getByRole('button', { name: 'Send request' }).click()
    await expect(page.getByLabel('Response body')).toContainText(
      'hello from the browser',
    )
    await expect(page.getByLabel('Response body')).toContainText('user-1')
    await expect(page.getByLabel('Response body')).toContainText(
      'source=browser',
    )
    await page.getByLabel('Client identity').selectOption('1')
    await page.getByRole('button', { name: 'Send request' }).click()
    await expect(page.getByLabel('Response body')).toContainText('user-2')
    await page.screenshot({
      path: testInfo.outputPath('demo-desktop.png'),
      fullPage: true,
    })
    expect(errors).toEqual([])
  } finally {
    await page.getByRole('button', { name: 'Stop', exact: true }).click()
    await expect(
      page.getByText('Start Docker to launch the proxy and sample API.', {
        exact: true,
      }),
    ).toBeVisible({ timeout: 30000 })
  }
})

test('each algorithm launches in RAM and Redis and enforces API-key quotas', async ({
  request,
}) => {
  test.setTimeout(360000)
  const headers = { Origin: origin }
  let current = config
  async function send(client = 0) {
    const response = await request.post('/api/rate-limiter/request', {
      headers,
      data: { method: 'GET', path: '/test', headers: {}, body: '', client },
    })
    expect(response.ok()).toBe(true)
    return response.json()
  }
  try {
    for (const storage of ['ram', 'redis']) {
      for (const algorithm of [
        'fixed_window',
        'sliding_window_log',
        'sliding_window_counter',
        'token_bucket',
        'leaky_bucket',
      ]) {
        current = {
          ...config,
          algorithm,
          storage,
          identity: 'api_key',
          limit: 1,
        }
        const start = await request.post('/api/rate-limiter/control', {
          headers,
          data: { action: 'start', config: current },
        })
        expect(start.status()).toBe(202)
        await expect
          .poll(
            async () => {
              const state = await (
                await request.get('/api/rate-limiter/control')
              ).json()
              if (state.phase === 'error') throw new Error(state.error)
              return state.phase
            },
            { timeout: 90000, intervals: [1000] },
          )
          .toBe('running')
        expect(
          (await send()).status,
          `${algorithm}/${storage} first request`,
        ).toBe(200)
        expect((await send()).status, `${algorithm}/${storage} limit`).toBe(429)
        expect(
          (await send(1)).status,
          `${algorithm}/${storage} separate key`,
        ).toBe(200)
      }
    }
    const reserved = await request.post('/api/rate-limiter/request', {
      headers,
      data: {
        method: 'GET',
        path: '/',
        headers: { 'X-Forwarded-For': '1.2.3.4' },
        body: '',
        client: 0,
      },
    })
    expect(reserved.status()).toBe(400)
    const escaped = await request.post('/api/rate-limiter/request', {
      headers,
      data: {
        method: 'GET',
        path: '//example.com',
        headers: {},
        body: '',
        client: 0,
      },
    })
    expect(escaped.status()).toBe(400)
  } finally {
    await request.post('/api/rate-limiter/control', {
      headers,
      data: { action: 'stop', config: current },
    })
    await expect
      .poll(
        async () =>
          (await (await request.get('/api/rate-limiter/control')).json()).phase,
        { timeout: 30000 },
      )
      .toBe('stopped')
  }
})

test('proxy mode runs with defaults and displays the active sample URL', async ({
  page,
}) => {
  await page.goto('/system-design/rate-limiter/proxy-mode')
  await expect(
    page.getByRole('heading', { name: 'Try the proxy locally' }),
  ).toBeVisible()
  await expect(page.getByLabel('Request path', { exact: true })).toHaveValue(
    '/',
  )
  await expect(page.getByLabel('Active request URL')).toHaveValue('')
  await page
    .getByLabel('Algorithm', { exact: true })
    .selectOption('token_bucket')
  await page.getByRole('button', { name: 'Start Docker', exact: true }).click()
  try {
    await expect(
      page.getByText('Docker is ready', { exact: true }),
    ).toBeVisible({ timeout: 150000 })
    await expect(page.getByLabel('Active request URL')).toHaveValue(
      /http:\/\/127\.0\.0\.1:\d+\/$/,
    )
    await page
      .getByRole('button', { name: 'Send request', exact: true })
      .click()
    await expect(page.getByRole('status')).toContainText('200')
    await expect(page.getByLabel('Response body')).toContainText('/')
    await page.getByLabel('Request path', { exact: true }).fill('/custom')
    await page.getByRole('button', { name: 'Use sample request' }).click()
    await expect(page.getByLabel('Request path', { exact: true })).toHaveValue(
      '/',
    )
    await page
      .getByLabel('Algorithm', { exact: true })
      .selectOption('fixed_window')
    await expect(
      page.getByRole('button', { name: 'Send request', exact: true }),
    ).toBeDisabled()
    await expect(page.getByLabel('Active request URL')).toHaveValue('')
  } finally {
    await page.getByRole('button', { name: 'Stop', exact: true }).click()
    await expect(
      page.getByText('Start Docker to launch the proxy and sample API.', {
        exact: true,
      }),
    ).toBeVisible({ timeout: 30000 })
  }
})
