'use client'

import { useEffect, useState } from 'react'
import Link from 'next/link'
import {
  ArrowRight,
  Container,
  Download,
  Globe,
  Loader2,
  Play,
  ShieldCheck,
  Square,
  UserRound,
} from 'lucide-react'
import { Input } from '@/components/ui/input'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import {
  algorithms,
  defaultDemoConfig,
  demoProxyConfig,
  validateDemoConfig,
  type DemoConfig,
} from '@/lib/rate-limiter-demo-config'

type Service = {
  phase: 'stopped' | 'starting' | 'running' | 'stopping' | 'error'
  config?: DemoConfig
  error?: string
  message?: string
  gateway?: string
}
type Result = {
  status: number
  durationMs: number
  headers: Record<string, string>
  body: string
  truncated: boolean
}
const field =
  'h-10 w-full min-w-0 rounded-lg border border-input bg-background px-3 text-sm focus-visible:outline-2 focus-visible:outline-ring'
const selectField =
  'w-full min-w-0 [&>select]:h-10 [&>select]:bg-background [&>select]:pr-10 [&>select]:pl-3 [&>select]:font-normal [&>svg]:right-3'
const button =
  'inline-flex min-h-10 items-center justify-center gap-2 rounded-lg border border-border px-4 text-sm font-medium focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring disabled:cursor-not-allowed disabled:opacity-50'

export function LocalRateLimiterDemo({
  hosted,
  embedded = false,
}: {
  hosted: boolean
  embedded?: boolean
}) {
  const [config, setConfig] = useState(defaultDemoConfig)
  const [available, setAvailable] = useState(false)
  const [service, setService] = useState<Service>({ phase: 'stopped' })
  const [error, setError] = useState('')
  const [sending, setSending] = useState(false)
  const [pending, setPending] = useState(false)
  const [result, setResult] = useState<Result | null>(null)
  const [method, setMethod] = useState('GET')
  const [requestPath, setRequestPath] = useState('/')
  const [headers, setHeaders] = useState('{\n  "Accept": "application/json"\n}')
  const [body, setBody] = useState('')
  const [client, setClient] = useState(0)
  const busy = pending || ['starting', 'stopping'].includes(service.phase)
  const dirty = JSON.stringify(config) !== JSON.stringify(service.config)
  const ready = available && service.phase === 'running' && !dirty && !busy
  const Title = embedded ? 'h2' : 'h1'
  const bucket = ['token_bucket', 'leaky_bucket'].includes(config.algorithm)

  useEffect(() => {
    if (
      hosted ||
      !['localhost', '127.0.0.1', '[::1]'].includes(window.location.hostname)
    )
      return
    let active = true
    let initialized = false
    let timer: ReturnType<typeof setTimeout>
    async function refresh() {
      try {
        const response = await fetch('/api/rate-limiter/control', {
          cache: 'no-store',
        })
        const data = await response.json()
        if (!active) return
        setAvailable(response.ok && data.available)
        if (response.ok) {
          setService(data)
          if (!initialized && data.config) setConfig(data.config)
          initialized = true
        }
      } catch {
        if (active) setError('Cannot connect to the local demo controls.')
      }
      if (active) timer = setTimeout(refresh, 1500)
    }
    void refresh()
    return () => {
      active = false
      clearTimeout(timer)
    }
  }, [hosted])

  function update(change: Partial<DemoConfig>) {
    setConfig((current) => ({ ...current, ...change }))
    if (change.identity) setClient(0)
    setResult(null)
    setError('')
  }

  async function control(action: 'start' | 'stop') {
    const next = action === 'stop' ? service.config || config : config
    const issue = validateDemoConfig(next)
    if (issue) {
      setError(issue)
      return
    }
    setError('')
    setPending(true)
    setResult(null)
    try {
      const response = await fetch('/api/rate-limiter/control', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ action, config: next }),
      })
      const data = await response.json()
      if (!response.ok) throw new Error(data.error)
      setService(data)
    } catch (failure) {
      setError(
        failure instanceof Error ? failure.message : 'Docker command failed.',
      )
    } finally {
      setPending(false)
    }
  }

  async function send() {
    setError('')
    setSending(true)
    try {
      let parsedHeaders
      try {
        parsedHeaders = JSON.parse(headers)
      } catch {
        throw new Error('Enter headers as a JSON object.')
      }
      const response = await fetch('/api/rate-limiter/request', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          method,
          path: requestPath,
          headers: parsedHeaders,
          body,
          client,
        }),
      })
      const data = await response.json()
      if (!response.ok) throw new Error(data.error)
      setResult(data)
    } catch (failure) {
      setError(failure instanceof Error ? failure.message : 'Request failed.')
    } finally {
      setSending(false)
    }
  }

  function download() {
    const issue = validateDemoConfig(config)
    if (issue) {
      setError(issue)
      return
    }
    const url = URL.createObjectURL(
      new Blob([JSON.stringify(demoProxyConfig(config), null, 2) + '\n'], {
        type: 'application/json',
      }),
    )
    const anchor = document.createElement('a')
    anchor.href = url
    anchor.download = 'proxy.json'
    anchor.click()
    setTimeout(() => URL.revokeObjectURL(url), 1000)
  }

  return (
    <div
      className={
        embedded
          ? 'min-w-0 space-y-6'
          : 'mx-auto w-full max-w-5xl min-w-0 space-y-6 px-4 py-7 sm:px-7'
      }
    >
      <header className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <p className="mb-2 text-xs font-medium text-primary">
            Proxy mode / Local playground
          </p>
          <Title>
            {embedded ? 'Try the proxy locally' : algorithms[config.algorithm]}
          </Title>
          <p className="mt-2 text-sm text-muted-foreground">
            Choose an algorithm, start Docker, and send a request. The sample
            backend and request are already configured.
          </p>
        </div>
        <Link
          href="/system-design/rate-limiter/algorithms"
          className={`${button} hover:bg-muted`}
        >
          Compare algorithms{' '}
          <ArrowRight className="size-4" aria-hidden="true" />
        </Link>
      </header>

      <section
        aria-label="Request flow"
        className="grid items-center gap-3 sm:grid-cols-[1fr_auto_1fr_auto_1fr]"
      >
        {[
          { title: 'You', detail: 'Client', icon: UserRound },
          {
            title: 'Rate limiter',
            detail: `${config.limit} requests / ${config.windowSeconds}s · ${config.storage.toUpperCase()}`,
            icon: ShieldCheck,
          },
          {
            title: 'End server',
            detail:
              config.target === 'demo'
                ? 'Docker demo backend'
                : config.upstream,
            icon: Globe,
          },
        ].map(({ title, detail, icon: Icon }, index) => (
          <div key={title} className="contents">
            {index > 0 && (
              <ArrowRight
                className="mx-auto size-4 rotate-90 text-muted-foreground sm:rotate-0"
                aria-hidden="true"
              />
            )}
            <div className="min-w-0 rounded-xl border border-border bg-card p-4">
              <Icon className="mb-3 size-5 text-primary" aria-hidden="true" />
              <h2 className="text-sm">{title}</h2>
              <p
                className="mt-1 truncate text-xs text-muted-foreground"
                title={detail}
              >
                {detail}
              </p>
            </div>
          </div>
        ))}
      </section>

      <section
        aria-labelledby="demo-config-title"
        className="rounded-xl border border-border bg-card p-5"
      >
        <h2 id="demo-config-title" className="mb-4 text-base">
          1. Choose an algorithm
        </h2>
        <form
          onSubmit={(event) => {
            event.preventDefault()
            void control('start')
          }}
          className="space-y-4"
        >
          <fieldset
            disabled={busy || sending}
            className="grid min-w-0 gap-4 disabled:opacity-60 sm:grid-cols-3"
          >
            <label className="flex min-w-0 flex-col gap-2 text-xs font-medium">
              Algorithm
              <NativeSelect
                aria-label="Algorithm"
                className={selectField}
                value={config.algorithm}
                onChange={(event) =>
                  update({
                    algorithm: event.target.value as DemoConfig['algorithm'],
                  })
                }
              >
                {Object.entries(algorithms).map(([value, label]) => (
                  <NativeSelectOption key={value} value={value}>
                    {label}
                  </NativeSelectOption>
                ))}
              </NativeSelect>
            </label>
            <details className="min-w-0 sm:col-span-3">
              <summary className="cursor-pointer py-2 text-sm font-medium">
                Customize settings
              </summary>
              <div className="mt-3 grid min-w-0 gap-4 sm:grid-cols-3">
                <label className="flex min-w-0 flex-col gap-2 text-xs font-medium">
                  Storage
                  <NativeSelect
                    aria-label="Storage"
                    className={selectField}
                    value={config.storage}
                    onChange={(event) =>
                      update({
                        storage: event.target.value as DemoConfig['storage'],
                      })
                    }
                  >
                    <NativeSelectOption value="ram">RAM</NativeSelectOption>
                    <NativeSelectOption value="redis">Redis</NativeSelectOption>
                  </NativeSelect>
                </label>
                <label className="flex min-w-0 flex-col gap-2 text-xs font-medium">
                  Limit by
                  <NativeSelect
                    aria-label="Limit by"
                    className={selectField}
                    value={config.identity}
                    onChange={(event) =>
                      update({
                        identity: event.target.value as DemoConfig['identity'],
                      })
                    }
                  >
                    <NativeSelectOption value="ip">
                      IP address
                    </NativeSelectOption>
                    <NativeSelectOption value="api_key">
                      API key
                    </NativeSelectOption>
                    <NativeSelectOption value="user">
                      User ID
                    </NativeSelectOption>
                  </NativeSelect>
                </label>
                <label className="flex min-w-0 flex-col gap-2 text-xs font-medium">
                  Requests
                  <Input
                    type="number"
                    min={1}
                    max={100000}
                    required
                    className="h-10"
                    value={config.limit || ''}
                    onChange={(event) =>
                      update({ limit: Number(event.target.value) })
                    }
                  />
                </label>
                <label className="flex min-w-0 flex-col gap-2 text-xs font-medium">
                  Window (seconds)
                  <Input
                    type="number"
                    min={1}
                    max={86400}
                    required
                    className="h-10"
                    value={config.windowSeconds || ''}
                    onChange={(event) =>
                      update({ windowSeconds: Number(event.target.value) })
                    }
                  />
                </label>
                {bucket && (
                  <label className="flex min-w-0 flex-col gap-2 text-xs font-medium">
                    {config.algorithm === 'token_bucket'
                      ? 'Burst capacity'
                      : 'Bucket capacity'}
                    <Input
                      type="number"
                      min={1}
                      max={100000}
                      required
                      className="h-10"
                      value={config.capacity || ''}
                      onChange={(event) =>
                        update({ capacity: Number(event.target.value) })
                      }
                    />
                  </label>
                )}
                <label className="flex min-w-0 flex-col gap-2 text-xs font-medium">
                  End server
                  <NativeSelect
                    aria-label="End server"
                    className={selectField}
                    value={config.target}
                    onChange={(event) =>
                      update({
                        target: event.target.value as DemoConfig['target'],
                      })
                    }
                  >
                    <NativeSelectOption value="demo">
                      Demo backend
                    </NativeSelectOption>
                    <NativeSelectOption value="website">
                      Website
                    </NativeSelectOption>
                    <NativeSelectOption value="server">
                      Your server
                    </NativeSelectOption>
                  </NativeSelect>
                </label>
                {config.target !== 'demo' && (
                  <label className="flex min-w-0 flex-col gap-2 text-xs font-medium sm:col-span-2">
                    {config.target === 'website' ? 'Website URL' : 'Server URL'}
                    <Input
                      type="url"
                      required
                      className="h-10"
                      value={config.upstream}
                      onChange={(event) =>
                        update({ upstream: event.target.value })
                      }
                      placeholder="http://host.docker.internal:4000"
                    />
                    {config.target === 'server' && (
                      <span className="block font-normal text-muted-foreground">
                        Use host.docker.internal to reach a server on your
                        computer.
                      </span>
                    )}
                  </label>
                )}
              </div>
            </details>
          </fieldset>
          <p className="text-xs text-muted-foreground">
            {algorithms[config.algorithm]} · {config.limit} requests /{' '}
            {config.windowSeconds}s · {config.storage.toUpperCase()}.{' '}
            {config.target === 'demo'
              ? 'Includes a local sample API. No URL or credentials to enter.'
              : `Upstream: ${config.upstream}`}
          </p>
          <div className="flex flex-wrap items-center gap-3 border-t border-border pt-4">
            <button
              type="submit"
              disabled={!available || busy || sending}
              className={`${button} bg-primary text-primary-foreground hover:opacity-90`}
            >
              {busy ? (
                <Loader2 className="size-4 animate-spin" aria-hidden="true" />
              ) : (
                <Container className="size-4" aria-hidden="true" />
              )}
              {service.phase === 'starting'
                ? 'Starting Docker…'
                : service.phase === 'stopping'
                  ? 'Stopping…'
                  : service.phase === 'running'
                    ? 'Apply & restart Docker'
                    : 'Start Docker'}
            </button>
            {service.config &&
              !['stopped', 'starting', 'stopping'].includes(service.phase) && (
                <button
                  type="button"
                  disabled={!available || busy || sending}
                  onClick={() => void control('stop')}
                  className={`${button} hover:bg-muted`}
                >
                  <Square className="size-3.5" aria-hidden="true" /> Stop
                </button>
              )}
            <button
              type="button"
              onClick={download}
              className={`${button} hover:bg-muted`}
            >
              <Download className="size-4" aria-hidden="true" /> Config
            </button>
            <span className="text-xs text-muted-foreground" aria-live="polite">
              {!available
                ? 'Local Docker only · disabled on hosted sites'
                : busy
                  ? service.message || 'First build may take a few minutes.'
                  : service.phase === 'running'
                    ? dirty
                      ? 'Settings changed · restart to apply'
                      : 'Docker is ready'
                    : 'Start Docker to launch the proxy and sample API.'}
            </span>
          </div>
        </form>
      </section>

      <section
        aria-labelledby="test-request-title"
        className="rounded-xl border border-border bg-card p-5"
      >
        <h2 id="test-request-title" className="mb-4 text-base">
          2. Test a request
        </h2>
        <div className="mb-4 space-y-2 rounded-lg border border-border bg-muted/30 p-3">
          <label className="block space-y-2 text-xs font-medium">
            Active request URL
            <Input
              readOnly
              className="h-10 font-mono text-xs"
              value={
                ready && service.gateway
                  ? `${service.gateway}${requestPath}`
                  : ''
              }
              placeholder="Available when Docker is ready"
            />
          </label>
          <p className="text-xs text-muted-foreground">
            {ready
              ? `${algorithms[config.algorithm]} is active. Requests go through the proxy to ${config.target === 'demo' ? 'the bundled sample API' : config.upstream}.`
              : 'Start Docker to activate the sample URL.'}
          </p>
          <button
            type="button"
            className={`${button} hover:bg-muted`}
            disabled={sending || busy}
            onClick={() => {
              setMethod('GET')
              setRequestPath('/')
              setHeaders('{\n  "Accept": "application/json"\n}')
              setBody('')
              setClient(0)
              setResult(null)
              setError('')
            }}
          >
            Use sample request
          </button>
        </div>
        <form
          className="space-y-4"
          onSubmit={(event) => {
            event.preventDefault()
            void send()
          }}
        >
          <div className="grid gap-3 sm:grid-cols-[110px_1fr_180px]">
            <label className="flex min-w-0 flex-col gap-2 text-xs font-medium">
              Method
              <NativeSelect
                aria-label="Method"
                className={selectField}
                value={method}
                onChange={(event) => setMethod(event.target.value)}
              >
                {[
                  'GET',
                  'POST',
                  'PUT',
                  'PATCH',
                  'DELETE',
                  'HEAD',
                  'OPTIONS',
                ].map((value) => (
                  <NativeSelectOption key={value}>{value}</NativeSelectOption>
                ))}
              </NativeSelect>
            </label>
            <label className="flex min-w-0 flex-col gap-2 text-xs font-medium">
              Request path
              <Input
                required
                className="h-10 font-mono"
                value={requestPath}
                onChange={(event) => setRequestPath(event.target.value)}
                placeholder="/api/orders"
              />
            </label>
            <label className="flex min-w-0 flex-col gap-2 text-xs font-medium">
              Client identity
              <NativeSelect
                aria-label="Client identity"
                className={selectField}
                value={client}
                disabled={config.identity === 'ip'}
                onChange={(event) => setClient(Number(event.target.value))}
              >
                <NativeSelectOption value={0}>
                  {config.identity === 'ip'
                    ? 'Connection IP'
                    : config.identity === 'user'
                      ? 'user-1'
                      : 'key-1'}
                </NativeSelectOption>
                {config.identity !== 'ip' && (
                  <NativeSelectOption value={1}>
                    {config.identity === 'user' ? 'user-2' : 'key-2'}
                  </NativeSelectOption>
                )}
              </NativeSelect>
            </label>
          </div>
          <p className="text-xs text-muted-foreground">
            {config.identity === 'ip'
              ? 'Requests share the local connection IP.'
              : 'Demo credentials are sent in the Authorization header.'}
          </p>
          <details className="rounded-lg border border-border px-3 py-2">
            <summary className="cursor-pointer py-1 text-xs font-medium">
              Headers & request body
            </summary>
            <label className="mt-3 block space-y-2 text-xs">
              Headers (JSON)
              <textarea
                className={`${field} h-24 py-2 font-mono text-xs`}
                value={headers}
                onChange={(event) => setHeaders(event.target.value)}
                spellCheck={false}
              />
            </label>
            {!['GET', 'HEAD'].includes(method) && (
              <label className="mt-3 block space-y-2 text-xs">
                Request body
                <textarea
                  className={`${field} h-24 py-2 font-mono text-xs`}
                  value={body}
                  onChange={(event) => setBody(event.target.value)}
                />
              </label>
            )}
          </details>
          <div className="flex flex-wrap items-center gap-3">
            <button
              disabled={!ready || sending}
              className={`${button} bg-primary text-primary-foreground hover:opacity-90`}
            >
              {sending ? (
                <Loader2 className="size-4 animate-spin" aria-hidden="true" />
              ) : (
                <Play className="size-4" aria-hidden="true" />
              )}{' '}
              Send request
            </button>
            <span className="text-xs text-muted-foreground">
              {!ready
                ? 'Start Docker with these settings to test.'
                : 'Send repeatedly to reach the limit.'}
            </span>
          </div>
        </form>
        {result && (
          <div className="mt-5 min-w-0 space-y-3 border-t border-border pt-4">
            <div
              role="status"
              className="flex flex-wrap items-center gap-3 text-sm"
            >
              <span
                className={`rounded-md px-2 py-1 font-mono font-semibold ${result.status === 429 ? 'bg-destructive/10 text-destructive' : 'bg-muted'}`}
              >
                {result.status}
                {result.status === 429 ? ' · Rate limited' : ''}
              </span>
              <span className="text-xs text-muted-foreground">
                {result.durationMs} ms
              </span>
              {result.headers['x-ratelimit-remaining'] && (
                <span className="text-xs">
                  {result.headers['x-ratelimit-remaining']} remaining
                </span>
              )}
              {result.headers['retry-after'] && (
                <span className="text-xs">
                  Retry after {result.headers['retry-after']}s
                </span>
              )}
            </div>
            <pre
              aria-label="Response body"
              className="max-h-64 overflow-auto rounded-lg bg-muted p-3 text-xs break-all whitespace-pre-wrap"
            >
              {result.body || '(empty response)'}
              {result.truncated && '\n[Preview truncated]'}
            </pre>
            <details className="text-xs">
              <summary className="cursor-pointer py-2">
                Response headers
              </summary>
              <pre className="overflow-auto rounded-lg bg-muted p-3 break-all whitespace-pre-wrap">
                {JSON.stringify(result.headers, null, 2)}
              </pre>
            </details>
          </div>
        )}
      </section>
      {(error || service.error) && (
        <pre
          role="alert"
          className="overflow-auto rounded-lg border border-destructive/30 bg-destructive/5 p-4 text-xs break-all whitespace-pre-wrap text-destructive"
        >
          {error || service.error}
        </pre>
      )}
    </div>
  )
}
