'use client'

import { useState } from 'react'
import Link from 'next/link'
import {
  ArrowDown,
  ArrowDownUp,
  ArrowLeft,
  ArrowRight,
  Check,
  CircleAlert,
  Container,
  Database,
  Globe,
  Info,
  MemoryStick,
  MousePointer2,
  Play,
  RotateCcw,
  ShieldCheck,
  Terminal,
  Users,
  Zap,
  type LucideIcon,
} from 'lucide-react'
import { flowSnapshot } from './flow-snapshot'
import { ConfigurationEditor } from './ConfigurationEditor'
import { Input } from '@/components/ui/input'
import {
  defaultConfiguration,
  identityLabels,
  normalizeExampleIdentity,
  type IdentityKind,
  type LimiterConfiguration,
  type Mode,
} from './configuration'

type NodeId = 'client' | 'proxy' | 'storage' | 'backend'
type ResponseCode = 200 | 429 | 503

function ArchitectureNode({
  id,
  title,
  eyebrow,
  description,
  icon: Icon,
  selected,
  onSelect,
  status,
}: {
  id: NodeId
  title: string
  eyebrow: string
  description: string
  icon: LucideIcon
  selected: NodeId
  onSelect: (id: NodeId) => void
  status?: string
}) {
  return (
    <button
      type="button"
      aria-pressed={selected === id}
      aria-label={`Inspect ${title}`}
      onClick={() => onSelect(id)}
      className={`w-full rounded-xl border bg-card p-4 text-left shadow-sm transition-colors focus-visible:outline-2 focus-visible:outline-offset-4 focus-visible:outline-ring ${
        selected === id
          ? 'border-primary ring-1 ring-primary/20'
          : 'border-border hover:border-primary/50'
      }`}
    >
      <span className="mb-3 flex items-center justify-between gap-2">
        <span className="flex size-9 items-center justify-center rounded-lg border border-border bg-muted/60 text-primary">
          <Icon className="size-[18px]" aria-hidden="true" />
        </span>
        <span className="text-[10px] font-medium tracking-wider text-muted-foreground uppercase">
          {eyebrow}
        </span>
      </span>
      <span className="block text-sm font-semibold">{title}</span>
      <span className="mt-1 block text-xs leading-relaxed break-words text-muted-foreground">
        {description}
      </span>
      {status && (
        <span className="mt-3 inline-flex max-w-full rounded-md bg-muted px-2 py-1 font-mono text-[10px] break-all text-foreground">
          {status}
        </span>
      )}
    </button>
  )
}

function Connection({ label, active }: { label: string; active?: boolean }) {
  return (
    <div
      aria-hidden="true"
      className={`flex items-center justify-center gap-2 text-[10px] lg:mt-24 lg:flex-col ${active ? 'text-primary' : 'text-muted-foreground'}`}
    >
      <ArrowDown className="size-5 lg:hidden" />
      <ArrowRight className="hidden size-6 lg:block" />
      <span>{label}</span>
    </div>
  )
}

export function RateLimiterArchitecture() {
  const [configuration, setConfiguration] =
    useState<LimiterConfiguration>(defaultConfiguration)
  const [activeRuleIndex, setActiveRuleIndex] = useState(0)
  const [selected, setSelected] = useState<NodeId>('proxy')
  const [counters, setCounters] = useState<
    Record<string, Record<string, number>>
  >({})
  const [attemptTotals, setAttemptTotals] = useState<Record<string, number>>({})
  const [response, setResponse] = useState<ResponseCode | null>(null)
  const [redisDown, setRedisDown] = useState(false)
  const [capacityReached, setCapacityReached] = useState(false)
  const [identityError, setIdentityError] = useState('')
  const [examples, setExamples] = useState<Record<IdentityKind, string>>({
    ip: '203.0.113.10',
    user: 'user-42',
    api_key: 'credential-7',
  })
  const mode = configuration.mode
  const rule = configuration.rules[activeRuleIndex] || configuration.rules[0]
  const example = examples[rule.identity]
  const identity = normalizeExampleIdentity(rule.identity, example)
  const key = JSON.stringify([rule.identity, identity])
  const policyCounters = Object.hasOwn(counters, rule.id)
    ? counters[rule.id]
    : {}
  const used = policyCounters[key] || 0
  const remaining = rule.limit - used
  const attempts = Object.hasOwn(attemptTotals, rule.id)
    ? attemptTotals[rule.id]
    : 0
  const forwarded = Object.values(policyCounters).reduce(
    (total, count) => total + count,
    0,
  )
  const upstream = new URL(configuration.upstream)

  function clearResult() {
    setResponse(null)
    setCapacityReached(false)
    setIdentityError('')
  }

  function resetWindow() {
    setCounters((current) => ({ ...current, [rule.id]: {} }))
    setAttemptTotals((current) => ({ ...current, [rule.id]: 0 }))
    clearResult()
  }

  function resetAll() {
    setCounters({})
    setAttemptTotals({})
    clearResult()
  }

  function changeMode(next: Mode) {
    setConfiguration((current) => ({ ...current, mode: next }))
    setRedisDown(false)
    resetAll()
  }

  function applyConfiguration(next: LimiterConfiguration) {
    setConfiguration(next)
    setActiveRuleIndex(0)
    setRedisDown(false)
    resetAll()
  }

  function simulate() {
    if (!identity) {
      setIdentityError(
        rule.identity === 'ip'
          ? 'Enter a valid example IPv4 or IPv6 address.'
          : 'Enter a nonblank credential or user ID of at most 256 bytes.',
      )
      return
    }
    setIdentityError('')
    setAttemptTotals((current) => ({
      ...current,
      [rule.id]: (Object.hasOwn(current, rule.id) ? current[rule.id] : 0) + 1,
    }))
    setCapacityReached(false)
    if (mode === 'redis' && redisDown) {
      setResponse(503)
    } else if (
      !(key in policyCounters) &&
      Object.keys(policyCounters).length >= rule.maxKeys
    ) {
      setCapacityReached(true)
      setResponse(503)
    } else if (used >= rule.limit) {
      setResponse(429)
    } else {
      setCounters((current) => ({
        ...current,
        [rule.id]: { ...current[rule.id], [key]: used + 1 },
      }))
      setResponse(200)
    }
  }

  const details: Record<NodeId, { title: string; body: string }> = {
    client: {
      title:
        rule.identity === 'ip'
          ? 'One allowance per client IP'
          : `One allowance per ${identityLabels[rule.identity].toLowerCase()}`,
      body:
        rule.identity === 'ip'
          ? 'The gateway uses the socket address. Forwarding headers are considered only when the connecting proxy is explicitly trusted. Changing an untrusted X-Forwarded-For header does not create a new allowance.'
          : 'User and API credential IDs must come from verified authentication. Each tenant and policy gets independent counters. A credential ID identifies the verified key; never use an arbitrary client header as proof of identity.',
    },
    proxy: {
      title: 'Decide before forwarding',
      body: 'The Docker gateway matches a route policy, resolves the identity, and checks its allowance. Allowed requests retain their method, path, query, and body when forwarded. A rejection returns directly to the client.',
    },
    storage: {
      title:
        mode === 'ram'
          ? 'RAM belongs to one gateway'
          : 'Redis shares the allowance',
      body:
        mode === 'ram'
          ? 'Counters live inside this process and are protected by a mutex. Each gateway has its own counters; restarting clears them. Periodic cleanup removes expired entries.'
          : 'Gateway replicas with the same policy and namespace use one shared allowance. An atomic Lua operation checks the counter, capacity, and expiry. A Redis failure returns 503; the gateway does not fall back to RAM.',
    },
    backend: {
      title: 'Only admitted requests reach the origin',
      body: `The configured upstream is ${configuration.upstream}. The backend response returns through the gateway. Keep the backend behind the gateway in your deployment to prevent bypassing it. Applying this illustration does not change a running Docker service.`,
    },
  }

  const result =
    response === 200
      ? {
          title: '200 · Forwarded',
          body: 'The backend receives this request and returns its response.',
        }
      : response === 429
        ? {
            title: '429 · Rate limited',
            body: 'Allowance exhausted. Return Retry-After; the backend is not reached.',
          }
        : response === 503
          ? {
              title: capacityReached
                ? '503 · Identity capacity reached'
                : '503 · Redis unavailable',
              body: capacityReached
                ? 'This policy cannot track another identity. Existing identities keep their allowance; the backend is not reached.'
                : 'Fail closed. No RAM fallback and no request to the backend.',
            }
          : {
              title: 'Ready to simulate',
              body: `Try ${rule.limit + 1} requests for the same identity to see this policy limit in action.`,
            }

  return (
    <div className="mx-auto w-full max-w-6xl min-w-0 space-y-6 px-4 py-7 sm:px-7">
      <header className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <div className="mb-2 flex items-center gap-2 text-[11px] font-semibold tracking-wider text-primary uppercase">
            <Zap className="size-3.5" aria-hidden="true" /> Rate limiter
          </div>
          <h1>From client to backend</h1>
          <p className="mt-2 max-w-xl text-sm">
            Explore how the reverse proxy checks an allowance before a request
            reaches your service.
          </p>
        </div>
        <Link
          href="/system-design/load-tester"
          className="inline-flex min-h-10 items-center gap-2 rounded-lg border border-border bg-card px-3 text-xs font-medium hover:bg-muted focus-visible:outline-2 focus-visible:outline-ring"
        >
          Open Load Tester{' '}
          <ArrowRight className="size-3.5" aria-hidden="true" />
        </Link>
      </header>

      <ConfigurationEditor
        config={configuration}
        onApply={applyConfiguration}
        onModeChange={changeMode}
      />

      <section
        aria-labelledby="architecture-title"
        className="overflow-hidden rounded-2xl border border-border bg-card"
      >
        <div className="flex flex-wrap items-center justify-between gap-4 border-b border-border px-5 py-4">
          <div>
            <h2 id="architecture-title" className="text-sm">
              Request architecture
            </h2>
            <span className="mt-1 block text-xs text-muted-foreground">
              Fixed window · {identityLabels[rule.identity]} ·{' '}
              {configuration.rules.length}{' '}
              {configuration.rules.length === 1 ? 'policy' : 'policies'}
            </span>
          </div>
          <fieldset className="flex rounded-lg border border-border bg-muted/50 p-1">
            <legend className="sr-only">Storage mode</legend>
            {(['ram', 'redis'] as const).map((value) => (
              <label key={value} className="cursor-pointer">
                <input
                  type="radio"
                  name="storage-mode"
                  value={value}
                  checked={mode === value}
                  onChange={() => changeMode(value)}
                  className="peer sr-only"
                />
                <span
                  className={`flex min-h-9 items-center gap-2 rounded-md px-4 text-xs font-medium peer-focus-visible:outline-2 peer-focus-visible:outline-offset-2 peer-focus-visible:outline-ring ${mode === value ? 'bg-card text-primary shadow-sm' : 'text-muted-foreground'}`}
                >
                  {value === 'ram' ? (
                    <MemoryStick className="size-3.5" aria-hidden="true" />
                  ) : (
                    <Database className="size-3.5" aria-hidden="true" />
                  )}
                  {value === 'ram' ? 'RAM' : 'Redis'}
                </span>
              </label>
            ))}
          </fieldset>
        </div>

        <div
          className="px-5 py-6 sm:px-7"
          style={{
            backgroundImage:
              'radial-gradient(var(--border) 1px, transparent 1px)',
            backgroundSize: '20px 20px',
          }}
        >
          <div className="mb-5 flex items-center gap-1.5 text-[11px] text-muted-foreground">
            <MousePointer2 className="size-3" aria-hidden="true" /> Select a
            component to explore its role
          </div>
          <div className="grid items-start gap-4 lg:grid-cols-[minmax(0,1fr)_36px_minmax(0,1.3fr)_36px_minmax(0,1.1fr)] lg:gap-3">
            <div className="lg:mt-10">
              <ArchitectureNode
                id="client"
                title="Client"
                eyebrow="01 · Request"
                description="Browser or application"
                icon={Globe}
                selected={selected}
                onSelect={setSelected}
                status={`${identityLabels[rule.identity]} · ${example || 'Not set'}`}
              />
              <div className="mt-3 flex items-center gap-1.5 text-[11px] text-muted-foreground">
                <Users className="size-3" aria-hidden="true" /> Each identity
                gets its own allowance
              </div>
            </div>
            <Connection label="HTTP" active={response !== null} />
            <div
              className={
                mode === 'ram'
                  ? 'rounded-2xl border border-dashed border-primary/40 bg-background/80 p-3'
                  : ''
              }
            >
              <div
                className={
                  mode === 'redis'
                    ? 'rounded-2xl border border-dashed border-primary/40 bg-background/80 p-3'
                    : ''
                }
              >
                <div className="mb-3 flex items-center gap-2 text-[10px] font-medium tracking-wider text-primary uppercase">
                  <Container className="size-3.5" aria-hidden="true" /> Docker
                  gateway
                </div>
                <ArchitectureNode
                  id="proxy"
                  title="Rate limiter + proxy"
                  eyebrow="02 · Decide"
                  description={`Match policy → resolve ${rule.identity === 'ip' ? 'IP' : 'identity'} → check limit`}
                  icon={ShieldCheck}
                  selected={selected}
                  onSelect={setSelected}
                  status={`${rule.limit} requests / ${rule.windowSeconds} seconds`}
                />
              </div>
              <div className="flex items-center justify-center gap-2 py-3 text-[10px] text-muted-foreground">
                <ArrowDownUp className="size-4" aria-hidden="true" />{' '}
                {mode === 'ram'
                  ? 'Read / update counter'
                  : 'Atomic Lua decision'}
              </div>
              <ArchitectureNode
                id="storage"
                title={mode === 'ram' ? 'RAM counters' : 'Redis state'}
                eyebrow={mode === 'ram' ? 'Process-local' : 'Shared service'}
                description={
                  mode === 'ram'
                    ? 'Mutex-protected map · periodic cleanup'
                    : 'Shared across replicas · automatic expiry'
                }
                icon={mode === 'ram' ? MemoryStick : Database}
                selected={selected}
                onSelect={setSelected}
                status={
                  mode === 'redis' && redisDown
                    ? 'Unavailable'
                    : `${used} used · ${remaining} remaining`
                }
              />
              {mode === 'redis' && (
                <span className="mt-2 block text-center text-[10px] text-muted-foreground">
                  Redis runs separately from the gateway.
                </span>
              )}
            </div>
            <Connection label="Allowed" active={response === 200} />
            <div className="lg:mt-10">
              <ArchitectureNode
                id="backend"
                title="Backend service"
                eyebrow="03 · Forward"
                description={
                  upstream.host +
                  (upstream.pathname === '/' ? '' : upstream.pathname)
                }
                icon={Globe}
                selected={selected}
                onSelect={setSelected}
                status={
                  response === 200
                    ? `Receives request · ${upstream.protocol === 'https:' ? 'HTTPS' : 'HTTP'}`
                    : response
                      ? 'Not reached'
                      : `${upstream.protocol === 'https:' ? 'HTTPS' : 'HTTP'} upstream`
                }
              />
              <div className="mt-3 flex items-center gap-1.5 text-[11px] text-muted-foreground">
                <ArrowLeft className="size-3" aria-hidden="true" /> Response
                returns through proxy
              </div>
            </div>
          </div>
          <div
            className={`mt-5 flex items-start gap-3 rounded-lg border px-4 py-3 text-xs ${response === 429 || response === 503 ? 'border-destructive/30 bg-destructive/5 text-destructive' : 'border-border bg-card/90 text-muted-foreground'}`}
          >
            <ArrowLeft className="mt-0.5 size-4 shrink-0" aria-hidden="true" />
            <div>
              <span className="font-semibold">Rejected → back to client</span>
              <span className="mt-1 block leading-relaxed">
                429 when the allowance is exhausted. 503 when Redis or
                rate-limit capacity is unavailable. These requests stop at the
                gateway.
              </span>
            </div>
          </div>
        </div>

        <div className="flex items-start gap-3 border-t border-border bg-muted/25 px-5 py-4">
          <Info
            className="mt-0.5 size-4 shrink-0 text-primary"
            aria-hidden="true"
          />
          <div>
            <h3 className="text-xs">{details[selected].title}</h3>
            <p className="mt-1 text-xs leading-relaxed">
              {details[selected].body}
            </p>
          </div>
        </div>
      </section>

      <section
        aria-labelledby="simulation-title"
        className="rounded-xl border border-border bg-card p-5"
      >
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <h2 id="simulation-title" className="text-sm">
              Walk through a request
            </h2>
            <p className="mt-1 text-xs">
              Local illustration · no network traffic. Reset advances to a fresh
              window for the selected policy.
            </p>
          </div>
          {mode === 'redis' && (
            <label className="flex min-h-10 cursor-pointer items-center gap-2 text-xs">
              <input
                type="checkbox"
                checked={!redisDown}
                onChange={(event) => {
                  setRedisDown(!event.target.checked)
                  setResponse(null)
                }}
                className="size-4 accent-primary"
              />{' '}
              Redis available
            </label>
          )}
        </div>
        <div className="mt-4 grid gap-4 sm:grid-cols-2">
          <label className="space-y-1.5 text-xs font-medium">
            Simulate policy
            <select
              aria-label="Simulate policy"
              value={activeRuleIndex}
              onChange={(event) => {
                setActiveRuleIndex(Number(event.target.value))
                clearResult()
              }}
              className="h-9 w-full rounded-lg border border-input bg-background px-2 text-xs focus-visible:outline-2 focus-visible:outline-ring"
            >
              {configuration.rules.map((item, index) => (
                <option key={item.id} value={index}>
                  {item.id} · {item.pattern} · {identityLabels[item.identity]}
                </option>
              ))}
            </select>
          </label>
          <label className="space-y-1.5 text-xs font-medium">
            Example {identityLabels[rule.identity].toLowerCase()}
            <Input
              value={example}
              onChange={(event) => {
                setExamples((current) => ({
                  ...current,
                  [rule.identity]: event.target.value,
                }))
                clearResult()
              }}
              className="h-9 text-xs"
              autoComplete="off"
            />
          </label>
        </div>
        <p className="mt-2 text-[11px] text-muted-foreground">
          Changing the example identity gives it a separate counter. User and
          credential IDs here are illustrative, not secret API tokens.
        </p>
        {identityError && (
          <p role="alert" className="mt-2 text-xs text-destructive">
            {identityError}
          </p>
        )}
        <div className="mt-4 flex flex-wrap items-center gap-3">
          <button
            type="button"
            onClick={simulate}
            className="inline-flex min-h-10 items-center gap-2 rounded-lg bg-primary px-4 text-xs font-medium text-primary-foreground hover:opacity-90 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring"
          >
            <Play className="size-3.5" aria-hidden="true" /> Simulate request
          </button>
          <button
            type="button"
            onClick={resetWindow}
            className="inline-flex min-h-10 items-center gap-2 rounded-lg border border-border px-3 text-xs font-medium hover:bg-muted focus-visible:outline-2 focus-visible:outline-ring"
          >
            <RotateCcw className="size-3.5" aria-hidden="true" /> Reset window
          </button>
          <span
            className="font-mono text-xs text-muted-foreground"
            data-testid="remaining-allowance"
          >
            {remaining} / {rule.limit} remaining
          </span>
        </div>
        <div
          role="status"
          aria-live="polite"
          aria-atomic="true"
          className="mt-4 flex items-start gap-3 rounded-lg bg-muted/50 p-3"
        >
          {response === 200 ? (
            <Check
              className="mt-0.5 size-4 shrink-0 text-emerald-700 dark:text-emerald-400"
              aria-hidden="true"
            />
          ) : response ? (
            <CircleAlert
              className="mt-0.5 size-4 shrink-0 text-destructive"
              aria-hidden="true"
            />
          ) : (
            <Terminal
              className="mt-0.5 size-4 shrink-0 text-muted-foreground"
              aria-hidden="true"
            />
          )}
          <div className="min-w-0 flex-1">
            <span className="text-xs font-semibold">{result.title}</span>
            <p className="mt-1 text-xs">{result.body}</p>
            <span className="mt-2 block font-mono text-[10px] text-muted-foreground">
              {attempts} attempted · {forwarded} forwarded in this policy window
            </span>
          </div>
        </div>
      </section>

      <section aria-labelledby="verification-title" className="pb-4">
        <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
          <h2 id="verification-title" className="text-sm">
            Recorded hiteshsolanki.com verification
          </h2>
          <span className="text-[11px] text-muted-foreground">
            {flowSnapshot.date} · {flowSnapshot.checks} flow checks passed
          </span>
        </div>
        <div className="grid gap-3 sm:grid-cols-3">
          {[
            {
              label: 'Direct to website',
              value: flowSnapshot.publicMedianMs.direct,
            },
            {
              label: 'Through RAM proxy',
              value: flowSnapshot.publicMedianMs.ram,
            },
            {
              label: 'Through Redis proxy',
              value: flowSnapshot.publicMedianMs.redis,
            },
          ].map((item) => (
            <div
              key={item.label}
              className="rounded-xl border border-border bg-card p-4"
            >
              <span className="text-xs text-muted-foreground">
                {item.label}
              </span>
              <div className="mt-2 font-mono text-xl font-medium tracking-tight">
                {item.value.toFixed(2)}{' '}
                <span className="text-xs font-normal text-muted-foreground">
                  ms
                </span>
              </div>
              <span className="text-[10px] text-muted-foreground">
                Median full response · {flowSnapshot.publicSamples} warm GETs
              </span>
            </div>
          ))}
        </div>
        <p className="mt-3 text-xs leading-relaxed">
          Historical test against hiteshsolanki.com, not live telemetry. Public
          timings include network, TLS, and transfer variability. In a separate{' '}
          {flowSnapshot.controlledSamples}-sample controlled test, median added
          time was {flowSnapshot.addedMedianMs.ram.toFixed(2)} ms for RAM and{' '}
          {flowSnapshot.addedMedianMs.redis.toFixed(2)} ms for Redis, including
          Docker networking.
        </p>
      </section>
    </div>
  )
}
