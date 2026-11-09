'use client'

import { useEffect, useState, type FormEvent, type ReactNode } from 'react'
import {
  ArrowRight,
  CircleStop,
  Loader2,
  Play,
  RefreshCw,
  SlidersHorizontal,
  Zap,
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { activeStatus, type TestConfig } from './types'
import { ResultsPanel } from './ResultsPanel'
import { useLoadTest } from './use-load-test'
import { StartService } from './StartService'
import { automaticLoads, timeAtLoad } from './workload'

function Field({
  label,
  id,
  children,
  hint,
}: {
  label: string
  id: string
  children: ReactNode
  hint?: string
}) {
  return (
    <div className="min-w-0 space-y-1.5">
      <label className="block text-xs font-medium" htmlFor={id}>
        {label}
      </label>
      {children}
      {hint && <p className="text-xs">{hint}</p>}
    </div>
  )
}
export function LoadTesterWorkbench() {
  const tests = useLoadTest()
  const [mode, setMode] = useState<'capacity' | 'fixed'>('capacity')
  const [url, setUrl] = useState('')
  const [maximumLoad, setMaximumLoad] = useState(20)
  const [duration, setDuration] = useState(30)
  const [p95, setP95] = useState(1000)
  const [errorPercent, setErrorPercent] = useState(1)
  const [headroom, setHeadroom] = useState(20)
  const [method, setMethod] = useState('GET')
  const [headers, setHeaders] = useState('')
  const [body, setBody] = useState('')
  const [timeout, setTimeoutSeconds] = useState(5)
  const [maxVUs, setMaxVUs] = useState(50)
  const [formError, setFormError] = useState('')
  const [elapsed, setElapsed] = useState(0)
  const running = activeStatus(tests.job)
  const locked = running || tests.pending
  useEffect(() => {
    if (!running || !tests.job?.startedAt) return
    const started = Date.parse(tests.job.startedAt)
    const timer = setInterval(
      () => setElapsed(Math.max(0, Math.floor((Date.now() - started) / 1000))),
      1000,
    )
    return () => clearInterval(timer)
  }, [running, tests.job?.startedAt])
  const parsedRates =
    mode === 'capacity'
      ? automaticLoads(maximumLoad)
      : automaticLoads(maximumLoad).slice(-1)
  const estimated =
    parsedRates.reduce((total, rate) => total + timeAtLoad(rate, duration), 0) +
    Math.max(0, parsedRates.length - 1) * timeout
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setFormError('')
    try {
      const target = new URL(url)
      if (
        !['http:', 'https:'].includes(target.protocol) ||
        target.username ||
        target.password
      )
        throw new Error('Enter an HTTP or HTTPS URL without credentials.')
      if (
        !Number.isInteger(maximumLoad) ||
        maximumLoad < 1 ||
        maximumLoad > 1000
      )
        throw new Error(
          'Choose a maximum load between 1 and 1,000 requests per second.',
        )
      if (estimated + timeout > 300)
        throw new Error(
          'This test would take too long. Reduce the time or request timeout in advanced settings.',
        )
      let requestHeaders: Record<string, string> | undefined
      if (headers.trim()) {
        const parsed: unknown = JSON.parse(headers)
        if (
          !parsed ||
          Array.isArray(parsed) ||
          typeof parsed !== 'object' ||
          Object.values(parsed).some((value) => typeof value !== 'string')
        )
          throw new Error('Headers must be a JSON object with string values.')
        requestHeaders = parsed as Record<string, string>
      }
      const config: TestConfig = {
        targetUrl: url,
        method,
        requestTimeoutSeconds: timeout,
        p95ThresholdMs: p95,
        maxErrorRate: errorPercent / 100,
        headroomPercent: headroom,
        virtualUsers: Math.min(10, maxVUs),
        maxVirtualUsers: maxVUs,
        headers: requestHeaders,
        body: body || undefined,
        ...(mode === 'capacity'
          ? {
              rateSteps: parsedRates.map((rate) => ({
                requestsPerSecond: rate,
                durationSeconds: timeAtLoad(rate, duration),
              })),
            }
          : {
              requestsPerSecond: maximumLoad,
              durationSeconds: timeAtLoad(maximumLoad, duration),
            }),
      }
      await tests.start(config)
    } catch (e) {
      setFormError(
        e instanceof Error ? e.message : 'Check your workload settings.',
      )
    }
  }
  return (
    <div className="w-full min-w-0">
      <div className="mx-auto max-w-6xl space-y-6 px-4 py-6 sm:px-7 sm:py-8">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <div className="mb-2 flex items-center gap-2 text-xs font-medium text-primary">
              <Zap className="size-3.5" />
              LOAD TESTER
            </div>
            <h1>How much traffic can your server handle?</h1>
            <p className="mt-2 max-w-xl text-sm">
              Enter your server address and the maximum load to try. We’ll
              increase traffic for you and show what passed.
            </p>
          </div>
          <button
            type="button"
            onClick={() => {
              void tests.refresh()
              tests.retry()
            }}
            className="inline-flex min-h-8 items-center gap-2 rounded-full border border-border px-3 text-xs hover:bg-muted focus-visible:outline-2 focus-visible:outline-ring"
            aria-label="Refresh service connection"
          >
            <span
              className={`size-1.5 rounded-full ${tests.connection === 'online' ? 'bg-primary' : 'bg-muted-foreground'}`}
            />
            {tests.connection === 'online'
              ? 'Load tester connected'
              : tests.connection === 'checking'
                ? 'Connecting…'
                : 'Service offline'}
            <RefreshCw className="size-3" />
          </button>
        </div>
        {tests.connection === 'offline' && (
          <StartService
            onReady={() => {
              void tests.refresh()
              tests.retry()
            }}
          />
        )}
        <div className="grid items-start gap-5 min-[75rem]:grid-cols-[340px_minmax(0,1fr)]">
          <section
            className="rounded-xl border border-border bg-card p-4 sm:p-5"
            aria-label="Test configuration"
          >
            <form onSubmit={submit} className="space-y-4">
              <div className="flex items-center justify-between">
                <h2 className="text-sm">Test your server</h2>
                <SlidersHorizontal className="size-4 text-muted-foreground" />
              </div>
              <fieldset
                disabled={locked}
                className="space-y-4 disabled:opacity-65"
              >
                <Field label="Website or server address" id="target-url">
                  <Input
                    id="target-url"
                    type="url"
                    placeholder="https://your-website.com"
                    required
                    value={url}
                    onChange={(event) => setUrl(event.target.value)}
                    className="h-10"
                  />
                </Field>
                <Field
                  label="Maximum load"
                  id="maximum-load"
                  hint="Requests per second. For example, 20 means up to 20 requests every second."
                >
                  <Input
                    id="maximum-load"
                    type="number"
                    min={1}
                    max={1000}
                    required
                    value={Number.isFinite(maximumLoad) ? maximumLoad : ''}
                    onChange={(event) =>
                      setMaximumLoad(event.target.valueAsNumber)
                    }
                    className="h-10"
                  />
                </Field>
                <div className="rounded-lg bg-muted/70 px-3 py-3 text-xs text-muted-foreground">
                  {mode === 'capacity'
                    ? 'We’ll start with a smaller load and increase it automatically, up to your maximum.'
                    : 'We’ll keep traffic at the load you chose.'}
                  {Number.isFinite(estimated) && parsedRates.length > 0
                    ? ` Allow about ${Math.ceil(estimated / 60)} ${Math.ceil(estimated / 60) === 1 ? 'minute' : 'minutes'}.`
                    : ''}{' '}
                  The results will show the highest load that passed and a
                  suggested limit with spare capacity.
                </div>
                <details className="group">
                  <summary className="cursor-pointer text-xs font-medium text-muted-foreground hover:text-foreground">
                    Advanced settings
                  </summary>
                  <div className="mt-4 space-y-4">
                    <Field label="Test approach" id="test-approach">
                      <select
                        id="test-approach"
                        value={mode}
                        onChange={(event) =>
                          setMode(event.target.value as 'capacity' | 'fixed')
                        }
                        className="h-9 w-full rounded-lg border border-input bg-background px-2 text-sm focus-visible:outline-2 focus-visible:outline-ring"
                      >
                        <option value="capacity">
                          Increase traffic automatically
                        </option>
                        <option value="fixed">Keep traffic steady</option>
                      </select>
                    </Field>
                    <Field
                      label="Time at each load (seconds)"
                      id="duration"
                      hint="We extend short tests when needed to collect enough responses."
                    >
                      <Input
                        id="duration"
                        type="number"
                        min={1}
                        max={90}
                        required
                        value={Number.isFinite(duration) ? duration : ''}
                        onChange={(event) =>
                          setDuration(event.target.valueAsNumber)
                        }
                      />
                    </Field>
                    <p className="text-xs">
                      For a server on this computer, use
                      http://host.docker.internal:PORT instead of localhost in
                      the address.
                    </p>

                    <div className="grid grid-cols-2 gap-3">
                      <Field label="Response time limit (ms)" id="p95">
                        <Input
                          id="p95"
                          type="number"
                          min={1}
                          required
                          value={Number.isFinite(p95) ? p95 : ''}
                          onChange={(event) =>
                            setP95(event.target.valueAsNumber)
                          }
                        />
                      </Field>
                      <Field label="Allowed failures (%)" id="error-rate">
                        <Input
                          id="error-rate"
                          type="number"
                          min={0}
                          max={100}
                          step={0.1}
                          required
                          value={
                            Number.isFinite(errorPercent) ? errorPercent : ''
                          }
                          onChange={(event) =>
                            setErrorPercent(event.target.valueAsNumber)
                          }
                        />
                      </Field>
                    </div>
                    <Field
                      label="Spare capacity (%)"
                      id="headroom"
                      hint="Leave 20% unused to make room for traffic spikes."
                    >
                      <Input
                        id="headroom"
                        type="number"
                        min={1}
                        max={90}
                        required
                        value={Number.isFinite(headroom) ? headroom : ''}
                        onChange={(event) =>
                          setHeadroom(event.target.valueAsNumber)
                        }
                      />
                    </Field>
                    <div className="grid grid-cols-2 gap-3">
                      <Field label="Timeout (s)" id="timeout">
                        <Input
                          id="timeout"
                          type="number"
                          min={1}
                          max={30}
                          required
                          value={Number.isFinite(timeout) ? timeout : ''}
                          onChange={(event) =>
                            setTimeoutSeconds(event.target.valueAsNumber)
                          }
                        />
                      </Field>
                      <Field label="Load generator workers" id="workers">
                        <Input
                          id="workers"
                          type="number"
                          min={1}
                          max={200}
                          required
                          value={Number.isFinite(maxVUs) ? maxVUs : ''}
                          onChange={(event) =>
                            setMaxVUs(event.target.valueAsNumber)
                          }
                        />
                      </Field>
                    </div>
                    <Field label="HTTP method" id="method">
                      <select
                        id="method"
                        value={method}
                        onChange={(event) => setMethod(event.target.value)}
                        className="h-9 w-full rounded-lg border border-input bg-background px-2 text-sm focus-visible:outline-2 focus-visible:outline-ring"
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
                          <option key={value}>{value}</option>
                        ))}
                      </select>
                    </Field>
                    <Field label="Headers (JSON, optional)" id="headers">
                      <textarea
                        id="headers"
                        value={headers}
                        onChange={(event) => setHeaders(event.target.value)}
                        placeholder={'{"Authorization": "Bearer …"}'}
                        rows={2}
                        className="w-full rounded-lg border border-input bg-background p-2 font-mono text-xs focus-visible:outline-2 focus-visible:outline-ring"
                      />
                    </Field>
                    <Field label="Request body (optional)" id="request-body">
                      <textarea
                        id="request-body"
                        value={body}
                        onChange={(event) => setBody(event.target.value)}
                        rows={2}
                        className="w-full rounded-lg border border-input bg-background p-2 font-mono text-xs focus-visible:outline-2 focus-visible:outline-ring"
                      />
                    </Field>
                  </div>
                </details>
              </fieldset>
              {(formError || tests.error) && (
                <div
                  role="alert"
                  className="rounded-lg bg-destructive/10 p-3 text-xs text-destructive"
                >
                  {formError || tests.error}
                  {tests.error && (
                    <button
                      type="button"
                      onClick={tests.retry}
                      className="ms-2 font-medium underline"
                    >
                      Retry
                    </button>
                  )}
                </div>
              )}
              <Button
                type="submit"
                size="lg"
                className="w-full"
                disabled={locked || tests.connection !== 'online'}
              >
                {tests.pending ? (
                  <Loader2 className="animate-spin" />
                ) : (
                  <Play className="size-3.5" />
                )}
                {tests.pending
                  ? 'Starting…'
                  : running
                    ? 'Test in progress'
                    : 'Test my server'}
                {!locked && <ArrowRight className="ms-auto size-3.5" />}
              </Button>
            </form>
            {tests.job && (
              <div
                className="mt-4 border-t border-border pt-4"
                role="status"
                aria-live="polite"
              >
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <span className="inline-flex items-center gap-2 text-xs font-medium capitalize">
                    {running && <Loader2 className="size-3.5 animate-spin" />}
                    {tests.job.status}
                    {running && tests.job.startedAt
                      ? ` · ${elapsed}s elapsed`
                      : ''}
                  </span>
                  {running && (
                    <Button
                      variant="outline"
                      size="sm"
                      disabled={
                        tests.cancelling || tests.job.status === 'cancelling'
                      }
                      onClick={() => void tests.cancel()}
                    >
                      <CircleStop />
                      {tests.job.status === 'cancelling'
                        ? 'Stopping…'
                        : 'Cancel'}
                    </Button>
                  )}
                </div>
                <p className="mt-2 text-xs break-all">
                  {tests.job.config.targetUrl}
                </p>
                <p className="mt-1 text-xs">
                  {tests.job.config.rateSteps
                    ? tests.job.config.rateSteps
                        .map((step) => step.requestsPerSecond)
                        .join(' → ')
                    : tests.job.config.requestsPerSecond ||
                      `${tests.job.config.virtualUsers} virtual users`}
                  {tests.job.config.rateSteps ||
                  tests.job.config.requestsPerSecond
                    ? ' requests / second'
                    : ''}
                  {' · '}
                  {tests.job.config.rateSteps?.[0]?.durationSeconds ??
                    tests.job.config.durationSeconds}
                  s at each load
                </p>
                {running && (
                  <p className="mt-1 text-xs">
                    Status updates automatically. Results appear after
                    completion.
                  </p>
                )}
              </div>
            )}
          </section>
          <ResultsPanel
            key={tests.selectedId}
            job={tests.job}
            report={tests.report}
          />
        </div>
        <section
          className="border-t border-border pt-5"
          aria-label="Recent runs"
        >
          <div className="mb-3 flex items-center justify-between">
            <h2 className="text-xs font-medium text-muted-foreground">
              Recent runs
            </h2>
            <Button
              size="sm"
              variant="ghost"
              onClick={() => void tests.refresh()}
            >
              <RefreshCw />
              Refresh
            </Button>
          </div>
          {tests.history.length === 0 ? (
            <p className="text-xs">Your test history will appear here.</p>
          ) : (
            <div className="grid gap-2 sm:grid-cols-2 min-[68.75rem]:grid-cols-4">
              {tests.history.map((item) => (
                <button
                  type="button"
                  key={item.id}
                  onClick={() => tests.select(item.id)}
                  aria-pressed={item.id === tests.selectedId}
                  className={`min-w-0 rounded-lg border p-3 text-start transition-colors hover:bg-muted focus-visible:outline-2 focus-visible:outline-ring ${item.id === tests.selectedId ? 'border-primary/40 bg-primary/5' : 'border-border bg-card'}`}
                >
                  <div
                    className="truncate text-xs font-medium"
                    title={item.config.targetUrl}
                  >
                    {item.config.targetUrl.replace(/^https?:\/\//, '')}
                  </div>
                  <div className="mt-1 flex flex-wrap items-center justify-between gap-1 text-[11px] text-muted-foreground">
                    <span className="capitalize">{item.status}</span>
                    <time dateTime={item.createdAt} suppressHydrationWarning>
                      {new Date(item.createdAt).toLocaleString(undefined, {
                        month: 'short',
                        day: 'numeric',
                        hour: '2-digit',
                        minute: '2-digit',
                      })}
                    </time>
                  </div>
                </button>
              ))}
            </div>
          )}
        </section>
      </div>
    </div>
  )
}
