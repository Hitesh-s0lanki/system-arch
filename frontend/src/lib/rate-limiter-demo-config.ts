import {
  proxyConfiguration,
  validateConfiguration,
  type IdentityKind,
  type Mode,
} from '@/app/(public)/system-design/rate-limiter/_components/configuration'

export const algorithms = {
  fixed_window: 'Fixed window',
  sliding_window_log: 'Sliding window log',
  sliding_window_counter: 'Sliding window counter',
  token_bucket: 'Token bucket',
  leaky_bucket: 'Leaky bucket',
} as const

export type DemoConfig = {
  algorithm: keyof typeof algorithms
  storage: Mode
  target: 'demo' | 'website' | 'server'
  upstream: string
  identity: IdentityKind
  limit: number
  windowSeconds: number
  capacity: number
}

export const defaultDemoConfig: DemoConfig = {
  algorithm: 'fixed_window',
  storage: 'ram',
  target: 'demo',
  upstream: 'https://example.com',
  identity: 'ip',
  limit: 3,
  windowSeconds: 10,
  capacity: 3,
}

export function demoProxyConfig(config: DemoConfig, revision = 'v1') {
  const proxy = proxyConfiguration({
    upstream:
      config.target === 'demo' ? 'http://backend:8080' : config.upstream,
    mode: config.storage,
    namespace: 'local-demo',
    rules: [
      {
        id: 'demo',
        pattern: '/',
        identity: config.identity,
        limit: config.limit,
        windowSeconds: config.windowSeconds,
        maxKeys: 10000,
        perRoute: false,
      },
    ],
  })
  return {
    ...proxy,
    routes: proxy.routes.map((route) => ({
      ...route,
      algorithm: config.algorithm,
      revision,
      ...(['token_bucket', 'leaky_bucket'].includes(config.algorithm)
        ? { capacity: config.capacity }
        : {}),
    })),
  }
}

export function validateDemoConfig(value: unknown): string | null {
  if (!value || typeof value !== 'object') return 'Invalid configuration.'
  const config = value as DemoConfig
  if (!Object.hasOwn(algorithms, config.algorithm))
    return 'Choose an algorithm.'
  if (!['demo', 'website', 'server'].includes(config.target))
    return 'Choose a server.'
  if (
    typeof config.upstream !== 'string' ||
    typeof config.identity !== 'string'
  )
    return 'Invalid server or identity.'
  if (
    !Number.isInteger(config.capacity) ||
    config.capacity < 1 ||
    config.capacity > 100000
  )
    return 'Capacity must be between 1 and 100,000.'
  if (
    !Number.isInteger(config.limit) ||
    config.limit < 1 ||
    config.limit > 100000
  )
    return 'Requests must be between 1 and 100,000.'
  return validateConfiguration({
    upstream:
      config.target === 'demo' ? 'http://backend:8080' : config.upstream,
    mode: config.storage,
    namespace: 'local-demo',
    rules: [
      {
        id: 'demo',
        pattern: '/',
        identity: config.identity,
        limit: config.limit,
        windowSeconds: config.windowSeconds,
        maxKeys: 10000,
        perRoute: false,
      },
    ],
  })
}
