import type { Metadata } from 'next'
import Link from 'next/link'
import { ArrowLeft, ArrowRight, Code2 } from 'lucide-react'
import { DocsIndex } from '../_components/DocsIndex'
import { SDKFlowDiagram } from '../_components/SDKFlowDiagram'

export const metadata: Metadata = {
  title: 'Rate Limiter | SDK Mode',
  description:
    'Embed the rate limiter directly in your Go service for in-process, service-level enforcement.',
}

const sections = [
  ['request-flow', 'Request flow'],
  ['overview', 'Overview'],
  ['integration', 'Integration points'],
  ['storage', 'Storage options'],
  ['tradeoffs', 'Tradeoffs'],
  ['when-to-use', 'When to use'],
] as const

export default function SDKModePage() {
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
          <Code2 className="size-4" /> SDK Mode
        </div>
        <h1 className="text-3xl font-semibold tracking-tight md:text-4xl">
          SDK Mode
        </h1>
        <p className="mt-4 max-w-2xl text-base leading-7 text-muted-foreground">
          Import the rate limiter as a Go library and call it inline in your
          service. Enforcement runs in the same process — no extra hop, no extra
          container.
        </p>
      </header>

      <div className="mt-10 space-y-14">
        <section id="request-flow" className="scroll-mt-20 space-y-4">
          <h2 className="text-xl font-semibold tracking-tight">Request flow</h2>
          <SDKFlowDiagram />
        </section>
        {/* Overview */}
        <section id="overview" className="scroll-mt-20 space-y-4">
          <h2 className="text-xl font-semibold tracking-tight">Overview</h2>
          <p className="text-sm leading-7 text-muted-foreground">
            SDK Mode embeds the limiter engine directly into your Go
            application. You call <code>limiter.Allow(ctx, key)</code> in your
            middleware or handler; the library handles the counter logic and
            returns a decision with the remaining allowance. No sidecar, no
            proxy — just a function call.
          </p>
          <div className="grid gap-4 sm:grid-cols-3">
            {[
              {
                label: 'In-process speed',
                body: 'No network hop for the limit check. RAM mode decisions happen in nanoseconds.',
              },
              {
                label: 'Fine-grained control',
                body: 'Apply different policies per route, per tenant, or per user inside a single handler.',
              },
              {
                label: 'Composable',
                body: 'Chain multiple limiters — global IP limit + per-user endpoint limit — with ordinary Go code.',
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

        {/* Integration points */}
        <section id="integration" className="scroll-mt-20 space-y-4">
          <h2 className="text-xl font-semibold tracking-tight">
            Integration points
          </h2>
          <p className="text-sm leading-7 text-muted-foreground">
            The SDK can be called at any layer of a Go HTTP stack:
          </p>
          <div className="grid gap-3 sm:grid-cols-2">
            {[
              {
                title: 'HTTP middleware',
                body: 'Wrap your router with a single middleware that enforces a global or per-route limit before any handler runs. Best for uniform policies across all endpoints.',
              },
              {
                title: 'Handler-level call',
                body: 'Call the limiter directly inside a specific handler for endpoint-specific limits, e.g. a stricter quota on a password-reset or export endpoint.',
              },
              {
                title: 'gRPC interceptor',
                body: 'Use the limiter inside a unary or streaming interceptor to gate RPC calls by caller identity or method name.',
              },
              {
                title: 'Background worker',
                body: 'Throttle outgoing API calls to third-party services by checking the limiter before each dispatch, turning it into an egress rate limiter.',
              },
            ].map((item) => (
              <div
                key={item.title}
                className="rounded-xl border border-border p-5"
              >
                <h3 className="text-sm font-semibold">{item.title}</h3>
                <p className="mt-2 text-sm leading-6 text-muted-foreground">
                  {item.body}
                </p>
              </div>
            ))}
          </div>
        </section>

        {/* Storage options */}
        <section id="storage" className="scroll-mt-20 space-y-4">
          <h2 className="text-xl font-semibold tracking-tight">
            Storage options
          </h2>
          <div className="grid gap-4 sm:grid-cols-2">
            {[
              {
                title: 'RAM (in-process map)',
                badge: 'Single instance',
                desc: 'Counters live in a mutex-protected map inside your service. Zero external dependencies. Restarting the process resets all counters. Use when you run one instance or when per-instance limits are acceptable.',
              },
              {
                title: 'Redis (shared counters)',
                badge: 'Horizontally scaled',
                desc: 'All replicas of your service share one counter per identity via an atomic Lua script. Requires a Redis connection but gives consistent limits regardless of how many instances are running.',
              },
            ].map((s) => (
              <div
                key={s.title}
                className="rounded-xl border border-border bg-card p-5"
              >
                <span className="mb-3 inline-block rounded-full bg-primary/10 px-2.5 py-0.5 text-[11px] font-medium text-primary">
                  {s.badge}
                </span>
                <h3 className="text-sm font-semibold">{s.title}</h3>
                <p className="mt-2 text-sm leading-6 text-muted-foreground">
                  {s.desc}
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
                <li>No proxy hop — lowest possible decision latency</li>
                <li>
                  Full access to request context, auth claims, and business
                  logic
                </li>
                <li>Compose multiple policies in code without config files</li>
                <li>Easier local development — no extra container to run</li>
              </ul>
            </div>
            <div className="rounded-xl border border-border bg-muted/20 p-5">
              <h3 className="mb-3 text-xs font-semibold tracking-wider text-destructive uppercase">
                Limitations
              </h3>
              <ul className="space-y-2 text-sm leading-6 text-muted-foreground">
                <li>Go only — no polyglot support</li>
                <li>Policy changes require a service redeploy</li>
                <li>
                  RAM counters are per-replica; each instance enforces its own
                  limit
                </li>
                <li>
                  Rejected requests still consume a small amount of service CPU
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
              SDK Mode is the right choice for Go services where you need
              context-aware limits (e.g. different quotas per subscription
              tier), want the absolute minimum latency overhead, or are
              enforcing egress limits on outgoing calls rather than incoming
              traffic.
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
                Need language-agnostic enforcement with no code changes?
              </p>
            </Link>
            <Link
              href="/system-design/rate-limiter/gateway-mode"
              className="group rounded-xl border border-border p-5 transition-colors hover:bg-accent focus-visible:outline-2 focus-visible:outline-ring"
            >
              <h3 className="flex items-center justify-between gap-3 text-sm font-medium">
                Gateway Mode
                <ArrowRight className="size-4 text-muted-foreground group-hover:text-foreground" />
              </h3>
              <p className="mt-2 text-sm leading-6 text-muted-foreground">
                Enforce limits centrally across many services via an API
                gateway.
              </p>
            </Link>
          </div>
        </section>
      </div>
    </div>
  )
}
