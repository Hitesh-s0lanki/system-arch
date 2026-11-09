export type IdentityKind = 'ip' | 'user' | 'api_key'
export type Mode = 'ram' | 'redis'

export type PolicyRule = {
  id: string
  pattern: string
  identity: IdentityKind
  limit: number
  windowSeconds: number
  maxKeys: number
  perRoute: boolean
}

export type LimiterConfiguration = {
  upstream: string
  mode: Mode
  namespace: string
  rules: PolicyRule[]
}

export const identityLabels: Record<IdentityKind, string> = {
  ip: 'IP address',
  user: 'User ID',
  api_key: 'API credential ID',
}

export const defaultConfiguration: LimiterConfiguration = {
  upstream: 'https://hiteshsolanki.com',
  mode: 'ram',
  namespace: 'demo',
  rules: [
    {
      id: 'default-ip',
      pattern: '/',
      identity: 'ip',
      limit: 3,
      windowSeconds: 10,
      maxKeys: 10000,
      perRoute: false,
    },
  ],
}

export function validateConfiguration(
  config: LimiterConfiguration,
): string | null {
  if (!['ram', 'redis'].includes(config.mode))
    return 'Choose RAM or Redis storage.'
  try {
    const url = new URL(config.upstream)
    if (
      !['http:', 'https:'].includes(url.protocol) ||
      !url.hostname ||
      url.username ||
      url.password ||
      url.search ||
      url.hash
    ) {
      return 'Enter an HTTP or HTTPS upstream without credentials, a query, or a fragment.'
    }
  } catch {
    return 'Enter a complete upstream URL, including http:// or https://.'
  }
  if (!/^[a-zA-Z0-9:_-]{1,128}$/.test(config.namespace))
    return 'Use 1–128 letters, numbers, colons, underscores, or hyphens for the namespace.'
  if (!config.rules.length || config.rules.length > 20)
    return 'Add between 1 and 20 policy rules.'
  const ids = new Set<string>()
  const patterns = new Set<string>()
  for (const [index, rule] of config.rules.entries()) {
    const prefix = `Rule ${index + 1}: `
    if (!/^[a-zA-Z0-9_-]{1,64}$/.test(rule.id) || ids.has(rule.id))
      return (
        prefix +
        'use a unique policy name with 1–64 letters, numbers, underscores, or hyphens.'
      )
    ids.add(rule.id)
    const parts = rule.pattern.trim().split(/\s+/)
    const path = parts.at(-1) || ''
    if (
      parts.length > 2 ||
      (parts.length === 2 &&
        !['GET', 'HEAD', 'POST', 'PUT', 'PATCH', 'DELETE', 'OPTIONS'].includes(
          parts[0],
        )) ||
      !path.startsWith('/') ||
      /[?#\s]/.test(path) ||
      new TextEncoder().encode(rule.pattern).length > 256
    )
      return prefix + 'use a route such as /, POST /orders, or GET /items/{id}.'
    if (path === '/healthz' || path === '/readyz')
      return prefix + 'health and readiness paths are reserved.'
    const wildcards = new Set<string>()
    for (const [segmentIndex, segment] of path.split('/').entries()) {
      if (!/[{}]/.test(segment)) continue
      if (
        !/^\{[a-zA-Z_][a-zA-Z0-9_]*(\.\.\.)?\}$/.test(segment) ||
        (segment.includes('...') && segmentIndex !== path.split('/').length - 1)
      )
        return (
          prefix +
          'use complete wildcard segments such as {id}, or a final {path...}.'
        )
      const wildcard = segment.replace(/[{}.]/g, '')
      if (wildcards.has(wildcard))
        return prefix + 'wildcard names must be unique within a route.'
      wildcards.add(wildcard)
    }
    const normalized = parts
      .join(' ')
      .replace(/\{[a-zA-Z_][a-zA-Z0-9_]*(\.\.\.)?\}/g, (_match, tail) =>
        tail ? '{...}' : '{}',
      )
    if (patterns.has(normalized))
      return prefix + 'each route pattern must be unique.'
    patterns.add(normalized)
    if (!['ip', 'user', 'api_key'].includes(rule.identity))
      return prefix + 'choose an IP, user, or API credential identity.'
    if (
      !Number.isInteger(rule.limit) ||
      rule.limit < 1 ||
      rule.limit > 2147483647
    )
      return (
        prefix + 'request limit must be a positive integer up to 2,147,483,647.'
      )
    if (
      !Number.isInteger(rule.windowSeconds) ||
      rule.windowSeconds < 1 ||
      rule.windowSeconds > 86400
    )
      return (
        prefix +
        'window must be a whole number of seconds between 1 and 86,400.'
      )
    if (
      !Number.isInteger(rule.maxKeys) ||
      rule.maxKeys < 1 ||
      rule.maxKeys > 2147483647
    )
      return (
        prefix + 'key capacity must be a positive integer up to 2,147,483,647.'
      )
  }
  return null
}

export function proxyConfiguration(config: LimiterConfiguration) {
  let upstream = config.upstream.trim()
  try {
    upstream = new URL(upstream).toString()
  } catch {
    /* Keep incomplete drafts visible in the preview. */
  }
  return {
    version: 1,
    listen: ':8080',
    upstream,
    storage: {
      mode: config.mode,
      namespace: config.namespace,
      timeout: '500ms',
    },
    cleanup_interval: '1s',
    upstream_timeout: '10s',
    anonymous_tenant: 'public',
    trusted_proxies: [],
    forward_authorization: false,
    routes: config.rules.map((rule) => ({
      pattern: rule.pattern.trim().replace(/\s+/g, ' '),
      policy: {
        id: rule.id,
        identity: rule.identity,
        per_route: rule.perRoute,
      },
      limit: rule.limit,
      window: `${rule.windowSeconds}s`,
      max_keys: rule.maxKeys,
    })),
  }
}

export function normalizeExampleIdentity(
  kind: IdentityKind,
  value: string,
): string | null {
  if (!value.trim() || new TextEncoder().encode(value).length > 256) return null
  if (kind !== 'ip') return value
  if (
    /^\d{1,3}(\.\d{1,3}){3}$/.test(value) &&
    value
      .split('.')
      .every(
        (part) =>
          Number(part) <= 255 && (part.length === 1 || !part.startsWith('0')),
      )
  )
    return value.split('.').map(Number).join('.')
  try {
    if (!value.includes(':') || value.includes('%')) return null
    const canonical = new URL(`http://[${value}]/`).hostname.slice(1, -1)
    const mapped = canonical.match(/^::ffff:([a-f0-9]{1,4}):([a-f0-9]{1,4})$/)
    if (mapped) {
      const high = parseInt(mapped[1], 16)
      const low = parseInt(mapped[2], 16)
      return [high >> 8, high & 255, low >> 8, low & 255].join('.')
    }
    return canonical
  } catch {
    return null
  }
}
