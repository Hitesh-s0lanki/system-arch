import type { Metadata } from 'next'
import Link from 'next/link'
import { ArrowLeft, ArrowRight, Globe } from 'lucide-react'
import { DocsIndex } from '../_components/DocsIndex'
import { ModeFlowDiagram } from '../_components/ModeFlowDiagram'

export const metadata: Metadata = {
  title: 'Rate Limiter | Gateway Mode',
  description:
    'Integrate the rate limiter into an API gateway to enforce limits centrally across all upstream services.',
}

const sections = [
  ['request-flow', 'Request flow'],
  ['overview', 'Overview'],
  ['position', 'Gateway position'],
  ['policy-model', 'Policy model'],
  ['tradeoffs', 'Tradeoffs'],
  ['when-to-use', 'When to use'],
] as const

export default function GatewayModePage() {
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
          <Globe className="size-4" /> Gateway Mode
        </div>
        <h1 className="text-3xl font-semibold tracking-tight md:text-4xl">
          Gateway Mode
        </h1>
        <p className="mt-4 max-w-2xl text-base leading-7 text-muted-foreground">
          Position the rate limiter at the API gateway layer to enforce a single
          unified quota policy across every upstream service from one place.
        </p>
      </header>

      <div className="mt-10 space-y-14">
        <section id="request-flow" className="scroll-mt-20 space-y-4">
          <h2 className="text-xl font-semibold tracking-tight">Request flow</h2>
          <ModeFlowDiagram mode="gateway" />
        </section>
        {/* Overview */}
        <section id="overview" className="scroll-mt-20 space-y-4">
          <h2 className="text-xl font-semibold tracking-tight">Overview</h2>
          <p className="text-sm leading-7 text-muted-foreground">
            Gateway Mode places the rate limiter at the entry point of a
            microservice platform. Every external request hits the gateway
            first; the limiter checks the caller&apos;s shared quota before
            routing to any downstream service. Individual services have no
            rate-limiting logic and no knowledge of quotas.
          </p>
          <div className="grid gap-4 sm:grid-cols-3">
            {[
              {
                label: 'Single enforcement point',
                body: 'One gateway enforces limits for all services. No duplicated config, no policy drift between teams.',
              },
              {
                label: 'Cross-service quota',
                body: "A caller's allowance is consumed regardless of which service they hit — 100 req/min across the entire platform, not per service.",
              },
              {
                label: 'Owned by platform team',
                body: "Service teams don't manage rate limit config. The platform team controls quotas centrally via versioned policy files.",
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

        {/* Gateway position */}
        <section id="position" className="scroll-mt-20 space-y-4">
          <h2 className="text-xl font-semibold tracking-tight">
            Gateway position
          </h2>
          <p className="text-sm leading-7 text-muted-foreground">
            The rate limiter in Gateway Mode sits behind TLS termination and
            authentication, but in front of routing:
          </p>
          <ol className="grid gap-3 sm:grid-cols-5">
            {[
              ['01', 'Internet', 'Raw traffic from clients'],
              ['02', 'TLS Ingress', 'Terminates HTTPS, forwards plain HTTP'],
              ['03', 'Auth layer', 'Verifies JWT / API key; attaches identity'],
              ['04', 'Rate Limiter ★', 'Checks quota; passes or rejects'],
              ['05', 'Upstream services', 'Orders, Users, Inventory …'],
            ].map(([n, title, body]) => (
              <li
                key={n}
                className={`rounded-xl border p-5 ${title === 'Rate Limiter ★' ? 'border-primary bg-primary/5' : 'border-border'}`}
              >
                <span className="text-xs font-medium text-primary">{n}</span>
                <h3 className="mt-2 text-sm font-medium">{title}</h3>
                <p className="mt-2 text-xs leading-5 text-muted-foreground">
                  {body}
                </p>
              </li>
            ))}
          </ol>
          <p className="text-sm leading-7 text-muted-foreground">
            Placing the limiter after auth means identity is already resolved;
            the limiter uses a verified user or credential ID rather than a raw
            IP address, giving precise per-customer quota.
          </p>
        </section>

        {/* Policy model */}
        <section id="policy-model" className="scroll-mt-20 space-y-4">
          <h2 className="text-xl font-semibold tracking-tight">Policy model</h2>
          <p className="text-sm leading-7 text-muted-foreground">
            Gateway policies are broader than per-service policies and are
            structured in tiers:
          </p>
          <div className="overflow-x-auto rounded-xl border border-border">
            <table className="w-full text-left text-sm">
              <thead className="bg-muted/40">
                <tr>
                  <th className="px-4 py-3 font-semibold">Tier</th>
                  <th className="px-4 py-3 font-semibold">Identity</th>
                  <th className="px-4 py-3 font-semibold">Limit example</th>
                  <th className="px-4 py-3 font-semibold">Scope</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-border">
                {[
                  [
                    'Global',
                    'Any IP',
                    '10 000 req / min',
                    'DDoS floor — protects the gateway itself',
                  ],
                  [
                    'Free tier',
                    'API credential',
                    '100 req / min',
                    'Per customer across all services',
                  ],
                  [
                    'Pro tier',
                    'API credential',
                    '5 000 req / min',
                    'Per customer across all services',
                  ],
                  [
                    'Endpoint',
                    'API credential',
                    '10 req / min on /auth/*',
                    'Sensitive route override',
                  ],
                ].map(([tier, identity, limit, scope]) => (
                  <tr key={tier}>
                    <td className="px-4 py-3 font-medium">{tier}</td>
                    <td className="px-4 py-3 text-muted-foreground">
                      {identity}
                    </td>
                    <td className="px-4 py-3 font-mono text-muted-foreground">
                      {limit}
                    </td>
                    <td className="px-4 py-3 text-muted-foreground">{scope}</td>
                  </tr>
                ))}
              </tbody>
            </table>
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
                <li>One policy file governs the entire platform</li>
                <li>
                  Cross-service quotas enforced without service coordination
                </li>
                <li>Services are completely decoupled from rate-limit logic</li>
                <li>
                  Easy to audit, roll back, or change tiers for a customer
                </li>
              </ul>
            </div>
            <div className="rounded-xl border border-border bg-muted/20 p-5">
              <h3 className="mb-3 text-xs font-semibold tracking-wider text-destructive uppercase">
                Limitations
              </h3>
              <ul className="space-y-2 text-sm leading-6 text-muted-foreground">
                <li>
                  Gateway is a critical path — requires HA and careful sizing
                </li>
                <li>
                  Service-specific limits need extra route policies in the
                  gateway config
                </li>
                <li>
                  Internal service-to-service calls that bypass the gateway are
                  unprotected
                </li>
                <li>
                  Redis is required for consistent quota across gateway replicas
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
              Gateway Mode is ideal for multi-service platforms where you sell
              API access with tiered quotas, where service teams should not own
              rate-limit policy, or where you need a single observable
              enforcement point for compliance and billing.
            </p>
          </div>
          <div className="grid gap-4 sm:grid-cols-2">
            <Link
              href="/system-design/rate-limiter/distributed-mode"
              className="group rounded-xl border border-border p-5 transition-colors hover:bg-accent focus-visible:outline-2 focus-visible:outline-ring"
            >
              <h3 className="flex items-center justify-between gap-3 text-sm font-medium">
                Distributed Mode
                <ArrowRight className="size-4 text-muted-foreground group-hover:text-foreground" />
              </h3>
              <p className="mt-2 text-sm leading-6 text-muted-foreground">
                Scale gateway replicas across multiple Redis instances.
              </p>
            </Link>
            <Link
              href="/system-design/rate-limiter/algorithms"
              className="group rounded-xl border border-border p-5 transition-colors hover:bg-accent focus-visible:outline-2 focus-visible:outline-ring"
            >
              <h3 className="flex items-center justify-between gap-3 text-sm font-medium">
                Algorithms
                <ArrowRight className="size-4 text-muted-foreground group-hover:text-foreground" />
              </h3>
              <p className="mt-2 text-sm leading-6 text-muted-foreground">
                Choose the right algorithm for your gateway tier policies.
              </p>
            </Link>
          </div>
        </section>
      </div>
    </div>
  )
}
