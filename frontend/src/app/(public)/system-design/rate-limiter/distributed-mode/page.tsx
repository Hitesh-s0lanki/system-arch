import type { Metadata } from 'next'
import Link from 'next/link'
import { ArrowLeft, ArrowRight, Network } from 'lucide-react'
import { DocsIndex } from '../_components/DocsIndex'
import { ModeFlowDiagram } from '../_components/ModeFlowDiagram'

export const metadata: Metadata = {
  title: 'Rate Limiter | Distributed Mode',
  description:
    'Test and operate the rate limiter across multiple Redis instances and replicated gateway nodes.',
}

const sections = [
  ['request-flow', 'Request flow'],
  ['overview', 'Overview'],
  ['topology', 'Topology'],
  ['consistency', 'Consistency model'],
  ['failure-modes', 'Failure modes'],
  ['when-to-use', 'When to use'],
] as const

export default function DistributedModePage() {
  return (
    <div className="mx-auto w-full max-w-6xl min-w-0 px-5 py-8 md:px-10 md:py-12 lg:pr-64">
      <DocsIndex sections={sections} />

      <Link
        href="/system-design/rate-limiter"
        className="inline-flex items-center gap-2 text-sm text-muted-foreground hover:text-foreground"
      >
        <ArrowLeft className="size-3.5" /> Rate Limiter overview
      </Link>

      <header className="mt-7 border-b border-border pb-8">
        <div className="mb-4 flex items-center gap-2 text-xs font-medium tracking-widest text-muted-foreground uppercase">
          <Network className="size-4" /> Distributed Mode
        </div>
        <h1 className="text-3xl font-semibold tracking-tight md:text-4xl">
          Distributed Mode
        </h1>
        <p className="mt-4 max-w-2xl text-base leading-7 text-muted-foreground">
          Run multiple rate-limiter replicas sharing quota state across one or
          more Redis instances — and understand what happens when parts of that
          infrastructure fail.
        </p>
      </header>

      <div className="mt-10 space-y-14">
        <section id="request-flow" className="scroll-mt-20 space-y-4">
          <h2 className="text-xl font-semibold tracking-tight">Request flow</h2>
          <ModeFlowDiagram mode="distributed" />
        </section>
        {/* Overview */}
        <section id="overview" className="scroll-mt-20 space-y-4">
          <h2 className="text-xl font-semibold tracking-tight">Overview</h2>
          <p className="text-sm leading-7 text-muted-foreground">
            Distributed Mode is not a separate binary — it is what happens when
            you configure multiple gateway or proxy replicas to point at the
            same Redis storage backend. Each replica is stateless; all quota
            state lives in Redis. The result is a horizontally scalable rate
            limiter where adding more gateway nodes does not fragment the
            allowance.
          </p>
          <div className="grid gap-4 sm:grid-cols-3">
            {[
              {
                label: 'Shared quota',
                body: 'All replicas read and write the same counter. 100 req/min is 100 across the whole cluster, not 100 per node.',
              },
              {
                label: 'Stateless gateways',
                body: 'Any replica can handle any request. Load balancers can round-robin without sticky sessions.',
              },
              {
                label: 'Atomic decisions',
                body: 'A Lua script on Redis checks capacity, increments the counter, and sets expiry in a single atomic operation.',
              },
            ].map((c) => (
              <div
                key={c.label}
                className="rounded-xl border border-border bg-card p-5"
              >
                <h3 className="text-sm font-semibold">{c.label}</h3>
                <p className="mt-2 text-sm leading-6 text-muted-foreground">
                  {c.body}
                </p>
              </div>
            ))}
          </div>
        </section>

        {/* Topology */}
        <section id="topology" className="scroll-mt-20 space-y-4">
          <h2 className="text-xl font-semibold tracking-tight">Topology</h2>
          <p className="text-sm leading-7 text-muted-foreground">
            Three common distributed topologies, in order of complexity:
          </p>
          <div className="space-y-3">
            {[
              {
                n: '01',
                title: 'Single Redis, multiple gateway replicas',
                badge: 'Most common',
                desc: 'All gateway containers share one Redis instance (or Redis Sentinel pair). Simple, low-ops. Redis is the single point of failure — mitigated with Sentinel or Redis Cluster.',
              },
              {
                n: '02',
                title: 'Redis Cluster (sharded)',
                badge: 'High throughput',
                desc: 'Redis Cluster shards keys across multiple primary nodes. The Lua script runs on the shard that owns the key. Quota for each identity lands on exactly one shard — no cross-slot coordination needed.',
              },
              {
                n: '03',
                title: 'Multi-region with per-region Redis',
                badge: 'Global deployment',
                desc: 'Each region has its own Redis; gateways in that region write locally. Quotas are per-region, not global. Use this when cross-region latency makes a single Redis impractical and per-region limits are acceptable.',
              },
            ].map((t) => (
              <div key={t.n} className="rounded-xl border border-border p-5">
                <div className="flex items-start justify-between gap-4">
                  <div>
                    <span className="text-xs font-medium text-primary">
                      {t.n}
                    </span>
                    <h3 className="mt-1 text-sm font-semibold">{t.title}</h3>
                  </div>
                  <span className="inline-block shrink-0 rounded-full bg-primary/10 px-2.5 py-0.5 text-[11px] font-medium text-primary">
                    {t.badge}
                  </span>
                </div>
                <p className="mt-2 text-sm leading-6 text-muted-foreground">
                  {t.desc}
                </p>
              </div>
            ))}
          </div>
        </section>

        {/* Consistency model */}
        <section id="consistency" className="scroll-mt-20 space-y-4">
          <h2 className="text-xl font-semibold tracking-tight">
            Consistency model
          </h2>
          <p className="text-sm leading-7 text-muted-foreground">
            The distributed limiter uses a single-writer consistency model per
            key, enforced by Redis:
          </p>
          <div className="overflow-x-auto rounded-xl border border-border">
            <table className="w-full text-left text-sm">
              <thead className="bg-muted/40">
                <tr>
                  <th className="px-4 py-3 font-semibold">Property</th>
                  <th className="px-4 py-3 font-semibold">Behaviour</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-border">
                {[
                  [
                    'Atomicity',
                    'INCR + EXPIRE execute inside a single Lua script — no partial updates.',
                  ],
                  [
                    'Ordering',
                    'Redis serialises all Lua scripts per key; no race between replicas.',
                  ],
                  [
                    'Accuracy',
                    'Exact fixed-window counter — not approximate. Over-limit requests are always rejected.',
                  ],
                  [
                    'Clock skew',
                    'Expiry is set by Redis server time. Gateway system clocks are irrelevant.',
                  ],
                  [
                    'Replication lag',
                    'Writes go to the Redis primary. Replicas are for read-only failover, not quota reads.',
                  ],
                ].map(([prop, behaviour]) => (
                  <tr key={prop}>
                    <td className="px-4 py-3 font-medium">{prop}</td>
                    <td className="px-4 py-3 text-muted-foreground">
                      {behaviour}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </section>

        {/* Failure modes */}
        <section id="failure-modes" className="scroll-mt-20 space-y-4">
          <h2 className="text-xl font-semibold tracking-tight">
            Failure modes
          </h2>
          <div className="space-y-3">
            {[
              {
                title: 'Redis unavailable',
                impact: '503 to all callers',
                detail:
                  'The gateway fails closed — it returns 503 and does not forward any request. There is no silent fallback to RAM. This prevents quota bypass during an outage.',
                severity: 'critical',
              },
              {
                title: 'Redis replication lag / split-brain',
                impact: 'Brief over-admission',
                detail:
                  'If a gateway reads from a stale replica during failover, it may allow slightly more than the limit for one window. Use Sentinel or Cluster to minimise failover duration.',
                severity: 'warning',
              },
              {
                title: 'Gateway replica crash',
                impact: 'No quota effect',
                detail:
                  'The crashed replica is removed by the load balancer. Its in-flight requests are lost (client retries). Redis state is unaffected; other replicas continue normally.',
                severity: 'low',
              },
              {
                title: 'Key capacity exhausted',
                impact: '503 for new identities',
                detail:
                  'Each policy has a configured maximum number of tracked identities. When capacity is full, new identities receive 503. Existing identities are unaffected. Tune maxKeys in the policy config.',
                severity: 'warning',
              },
            ].map((f) => (
              <div
                key={f.title}
                className={`rounded-xl border p-5 ${
                  f.severity === 'critical'
                    ? 'border-destructive/30 bg-destructive/5'
                    : f.severity === 'warning'
                      ? 'border-amber-500/30 bg-amber-500/5'
                      : 'border-border'
                }`}
              >
                <div className="flex items-start gap-4">
                  <div className="flex-1">
                    <h3 className="text-sm font-semibold">{f.title}</h3>
                    <span
                      className={`mt-1 inline-block text-xs font-medium ${
                        f.severity === 'critical'
                          ? 'text-destructive'
                          : f.severity === 'warning'
                            ? 'text-amber-600 dark:text-amber-400'
                            : 'text-muted-foreground'
                      }`}
                    >
                      Impact: {f.impact}
                    </span>
                    <p className="mt-2 text-sm leading-6 text-muted-foreground">
                      {f.detail}
                    </p>
                  </div>
                </div>
              </div>
            ))}
          </div>
        </section>

        {/* When to use */}
        <section id="when-to-use" className="scroll-mt-20 space-y-6 pb-6">
          <h2 className="text-xl font-semibold tracking-tight">When to use</h2>
          <div className="rounded-xl border border-border bg-muted/20 p-5">
            <p className="text-sm leading-7 text-muted-foreground">
              Use Distributed Mode any time you run more than one gateway or
              proxy replica and need a single unified quota per caller — which
              is essentially all production deployments. RAM mode is only
              appropriate for development or for use cases where per-instance
              limits are explicitly acceptable.
            </p>
          </div>
          <div className="grid gap-4 sm:grid-cols-2">
            <Link
              href="/system-design/rate-limiter/proxy-mode"
              className="group rounded-xl border border-border p-5 transition-colors hover:bg-accent focus-visible:outline-2 focus-visible:outline-ring"
            >
              <h3 className="flex items-center justify-between gap-3 text-sm font-medium">
                Proxy Mode
                <ArrowRight className="size-4 text-muted-foreground group-hover:text-foreground" />
              </h3>
              <p className="mt-2 text-sm leading-6 text-muted-foreground">
                Set up the proxy before scaling to multiple replicas.
              </p>
            </Link>
          </div>
        </section>
      </div>
    </div>
  )
}
