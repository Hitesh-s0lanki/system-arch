import type { Metadata } from 'next'
import Link from 'next/link'
import { ArrowLeft, ArrowRight, Shield } from 'lucide-react'
import { LocalRateLimiterDemo } from '../_components/LocalRateLimiterDemo'
import { DocsIndex } from '../_components/DocsIndex'
import { ProxyFlowDiagram } from '../_components/ProxyFlowDiagram'

export const metadata: Metadata = {
  title: 'Rate Limiter | Proxy Mode',
  description:
    'Run the rate limiter as a reverse proxy between your clients and backend service.',
}

const sections = [
  ['request-flow', 'Request flow'],
  ['local-playground', 'Try locally'],
  ['overview', 'Overview'],
  ['how-it-works', 'How it works'],
  ['topology', 'Deployment topology'],
  ['tradeoffs', 'Tradeoffs'],
  ['when-to-use', 'When to use'],
] as const

export default function ProxyModePage() {
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
          <Shield className="size-4" /> Proxy Mode
        </div>
        <h1 className="text-3xl font-semibold tracking-tight md:text-4xl">
          Proxy Mode
        </h1>
        <p className="mt-4 max-w-2xl text-base leading-7 text-muted-foreground">
          Deploy the rate limiter as a standalone reverse proxy. All traffic
          passes through it before reaching your backend — no code changes
          required.
        </p>
      </header>

      <div className="mt-10 space-y-14">
        <section id="request-flow" className="scroll-mt-20 space-y-4">
          <h2 className="text-xl font-semibold tracking-tight">Request flow</h2>
          <ProxyFlowDiagram />
        </section>
        <section id="local-playground" className="scroll-mt-20">
          <LocalRateLimiterDemo hosted={Boolean(process.env.VERCEL)} embedded />
        </section>
        {/* Overview */}
        <section id="overview" className="scroll-mt-20 space-y-4">
          <h2 className="text-xl font-semibold tracking-tight">Overview</h2>
          <p className="text-sm leading-7 text-muted-foreground">
            In Proxy Mode the rate limiter process sits between your load
            balancer (or the internet) and your origin service. It inspects
            every inbound HTTP request, checks the caller&apos;s allowance, and
            either forwards the request upstream or returns{' '}
            <code>429 Too Many Requests</code> immediately — the backend never
            sees rejected traffic.
          </p>
          <div className="grid gap-4 sm:grid-cols-3">
            {[
              {
                label: 'Zero backend changes',
                body: 'Enforcement lives entirely in the proxy process. Your service receives clean forwarded requests.',
              },
              {
                label: 'Language-agnostic',
                body: 'Works in front of any HTTP backend regardless of runtime — Go, Node, Python, JVM.',
              },
              {
                label: 'Centralised policy',
                body: 'All limit rules live in one proxy.json config, versioned and reloaded without restarting the backend.',
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

        {/* How it works */}
        <section id="how-it-works" className="scroll-mt-20 space-y-4">
          <h2 className="text-xl font-semibold tracking-tight">How it works</h2>
          <p className="text-sm leading-7 text-muted-foreground">
            Each request goes through a fixed decision pipeline inside the proxy
            process:
          </p>

          <ol className="grid gap-3 sm:grid-cols-4">
            {[
              [
                '01',
                'Route match',
                'The incoming path is matched against ordered route patterns in the config.',
              ],
              [
                '02',
                'Identity resolve',
                'The caller is identified by IP, verified user ID, or API credential.',
              ],
              [
                '03',
                'Counter check',
                'The allowance is read from RAM or Redis atomically.',
              ],
              [
                '04',
                'Forward or reject',
                'Under-limit requests are proxied; over-limit gets 429 + Retry-After.',
              ],
            ].map(([n, title, body]) => (
              <li key={n} className="rounded-xl border border-border p-5">
                <span className="text-xs font-medium text-primary">{n}</span>
                <h3 className="mt-2 text-sm font-medium">{title}</h3>
                <p className="mt-2 text-sm leading-6 text-muted-foreground">
                  {body}
                </p>
              </li>
            ))}
          </ol>
          <p className="text-sm leading-7 text-muted-foreground">
            The proxy preserves the original method, path, query string,
            headers, and body when forwarding. Response headers and status from
            the upstream are returned verbatim to the caller.
          </p>
        </section>

        {/* Topology */}
        <section id="topology" className="scroll-mt-20 space-y-4">
          <h2 className="text-xl font-semibold tracking-tight">
            Deployment topology
          </h2>
          <p className="text-sm leading-7 text-muted-foreground">
            The proxy runs as a lightweight Docker container with no persistent
            disk state (RAM mode) or as a stateless sidecar backed by Redis for
            shared quotas across replicas.
          </p>
          <div className="grid gap-4 sm:grid-cols-2">
            {[
              {
                title: 'Single-node (RAM)',
                desc: 'One proxy container in front of one backend. Counters live in process memory — zero infrastructure overhead. Restart clears all counters.',
                badge: 'Development · small services',
              },
              {
                title: 'Multi-replica (Redis)',
                desc: 'Many proxy replicas share a single Redis instance. Atomic Lua scripts ensure one unified quota across all nodes. Redis failure returns 503.',
                badge: 'Production · horizontally scaled APIs',
              },
            ].map((t) => (
              <div
                key={t.title}
                className="rounded-xl border border-border bg-card p-5"
              >
                <span className="mb-3 inline-block rounded-full bg-primary/10 px-2.5 py-0.5 text-[11px] font-medium text-primary">
                  {t.badge}
                </span>
                <h3 className="text-sm font-semibold">{t.title}</h3>
                <p className="mt-2 text-sm leading-6 text-muted-foreground">
                  {t.desc}
                </p>
              </div>
            ))}
          </div>
        </section>

        {/* Tradeoffs */}
        <section id="tradeoffs" className="scroll-mt-20 space-y-4">
          <h2 className="text-xl font-semibold tracking-tight">Tradeoffs</h2>
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="rounded-xl border border-border bg-muted/20 p-5">
              <h3 className="mb-3 text-xs font-semibold tracking-wider text-emerald-600 uppercase dark:text-emerald-400">
                Advantages
              </h3>
              <ul className="space-y-2 text-sm leading-6 text-muted-foreground">
                <li>No application code touched — drop-in for any stack</li>
                <li>Rejected requests never consume backend resources</li>
                <li>Config changes don&apos;t require service redeployment</li>
                <li>Easy to audit: all enforcement in one binary</li>
              </ul>
            </div>
            <div className="rounded-xl border border-border bg-muted/20 p-5">
              <h3 className="mb-3 text-xs font-semibold tracking-wider text-destructive uppercase">
                Limitations
              </h3>
              <ul className="space-y-2 text-sm leading-6 text-muted-foreground">
                <li>Extra network hop adds a few ms of latency</li>
                <li>Proxy is a single point of failure without redundancy</li>
                <li>
                  Cannot enforce limits on backend-to-backend calls that bypass
                  the proxy
                </li>
                <li>
                  TLS termination must be handled upstream (ingress, load
                  balancer)
                </li>
              </ul>
            </div>
          </div>
        </section>

        {/* When to use */}
        <section id="when-to-use" className="scroll-mt-20 space-y-6 pb-6">
          <h2 className="text-xl font-semibold tracking-tight">When to use</h2>
          <div className="rounded-xl border border-border bg-muted/20 p-5">
            <p className="text-sm leading-7 text-muted-foreground">
              Proxy Mode is the right choice when you want enforcement
              completely outside your application, need to protect a service you
              don&apos;t own the source of, or want to trial rate limiting
              without any deploy risk to your backend.
            </p>
          </div>
          <div className="grid gap-4 sm:grid-cols-2">
            <Link
              href="/system-design/rate-limiter/docs"
              className="group rounded-xl border border-border p-5 transition-colors hover:bg-accent focus-visible:outline-2 focus-visible:outline-ring"
            >
              <h3 className="flex items-center justify-between gap-3 text-sm font-medium">
                Installation guide
                <ArrowRight className="size-4 text-muted-foreground group-hover:text-foreground" />
              </h3>
              <p className="mt-2 text-sm leading-6 text-muted-foreground">
                Run the proxy locally with Docker Compose in under 5 minutes.
              </p>
            </Link>
            <Link
              href="/system-design/rate-limiter/distributed-mode"
              className="group rounded-xl border border-border p-5 transition-colors hover:bg-accent focus-visible:outline-2 focus-visible:outline-ring"
            >
              <h3 className="flex items-center justify-between gap-3 text-sm font-medium">
                Distributed Mode
                <ArrowRight className="size-4 text-muted-foreground group-hover:text-foreground" />
              </h3>
              <p className="mt-2 text-sm leading-6 text-muted-foreground">
                Scale to multiple Redis-backed proxy replicas sharing one quota.
              </p>
            </Link>
          </div>
        </section>
      </div>
    </div>
  )
}
