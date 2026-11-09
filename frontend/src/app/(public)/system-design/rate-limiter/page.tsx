import type { Metadata } from 'next'
import Link from 'next/link'
import { ArrowRight, Zap } from 'lucide-react'
import { DocsIndex } from './_components/DocsIndex'

export const metadata: Metadata = {
  title: 'Rate Limiter | Overview',
  description:
    'How this Go fixed-window rate limiter checks requests and shares quotas through Redis.',
}

export default function RateLimiterPage() {
  return (
    <div className="mx-auto w-full max-w-5xl min-w-0 space-y-10 px-5 py-8 md:px-10 md:py-12 lg:pr-64 [&_h2]:scroll-mt-20">
      <DocsIndex
        sections={[
          ['request-flow', 'Request flow'],
          ['storage', 'Storage modes'],
          ['policies', 'Identity and policies'],
          ['limits', 'Fixed-window tradeoff'],
          ['next', 'Start here'],
        ]}
      />
      <header className="border-b border-border pb-8">
        <div className="mb-4 flex items-center gap-2 text-xs font-medium tracking-widest text-muted-foreground uppercase">
          <Zap className="size-4" /> Rate Limiter · Overview
        </div>
        <h1 className="text-3xl font-semibold tracking-tight md:text-4xl">
          A rate limiter built in Go
        </h1>
        <p className="mt-4 max-w-2xl text-base leading-7 text-muted-foreground">
          This project implements a fixed-window counter with RAM and Redis
          storage. Use it as a Go library or run its reverse proxy in front of
          an HTTP backend to check each request against a configured allowance.
        </p>
      </header>
      <section aria-labelledby="request-flow" className="space-y-4">
        <h2 id="request-flow" className="text-xl font-semibold tracking-tight">
          How a request moves through the system
        </h2>
        <p className="text-sm leading-7 text-muted-foreground">
          The gateway matches a route to a policy, resolves the caller’s
          identity, and checks its counter. Requests within the limit reach the
          upstream. Once the allowance is exhausted, the gateway returns 429
          with a Retry-After header without calling the backend.
        </p>
        <ol className="grid gap-3 sm:grid-cols-3">
          {[
            [
              'Match a policy',
              'Routes select the limit, window, identity type, and policy revision.',
            ],
            [
              'Check the counter',
              'A tenant, policy, identity, and optional route identify the allowance.',
            ],
            [
              'Forward or reject',
              'Admitted attempts consume quota, even if the upstream later fails.',
            ],
          ].map(([title, detail], index) => (
            <li key={title} className="rounded-xl border border-border p-5">
              <span className="text-xs font-medium text-primary">
                0{index + 1}
              </span>
              <h3 className="mt-2 text-sm font-medium">{title}</h3>
              <p className="mt-2 text-sm leading-6 text-muted-foreground">
                {detail}
              </p>
            </li>
          ))}
        </ol>
      </section>
      <section aria-labelledby="storage" className="space-y-4">
        <h2 id="storage" className="text-xl font-semibold tracking-tight">
          One algorithm, two storage modes
        </h2>
        <div className="grid gap-4 sm:grid-cols-2">
          {[
            [
              'RAM for a single process',
              'Counters live in the gateway’s memory. Concurrent access is synchronized, keys are bounded, and expired entries are cleaned up. Each process has its own allowance, and a restart clears its counters.',
            ],
            [
              'Redis for shared quotas',
              'Replicas share counters using the same namespace and policy settings. A Lua script checks capacity, updates the counter, and sets expiry atomically on Redis. Storage failures return 503 without forwarding or falling back to RAM.',
            ],
          ].map(([title, detail]) => (
            <div key={title} className="rounded-xl border border-border p-5">
              <h3 className="font-medium">{title}</h3>
              <p className="mt-2 text-sm leading-7 text-muted-foreground">
                {detail}
              </p>
            </div>
          ))}
        </div>
      </section>
      <section aria-labelledby="policies" className="space-y-4">
        <h2 id="policies" className="text-xl font-semibold tracking-tight">
          Identity and policy boundaries
        </h2>
        <p className="text-sm leading-7 text-muted-foreground">
          Policies can limit by client IP, verified user ID, or verified API
          credential ID. Tenant and policy identifiers separate allowances;
          optional route grouping uses canonical route patterns rather than raw
          URLs. User and credential identities require verified authentication.
          Forwarded IP headers are accepted only from configured trusted
          proxies.
        </p>
        <p className="text-sm leading-7 text-muted-foreground">
          The reverse proxy loads a versioned JSON configuration at startup. It
          controls routes, upstream address, rate limits, key capacity,
          timeouts, and storage. The published Docker image includes the gateway
          and a demo backend for trying the request flow locally.
        </p>
      </section>
      <section
        aria-labelledby="limits"
        className="rounded-xl border border-border bg-muted/20 p-5"
      >
        <h2 id="limits" className="text-base font-semibold">
          Understand the fixed-window tradeoff
        </h2>
        <p className="mt-2 text-sm leading-7 text-muted-foreground">
          Each identity receives a new allowance when its window resets. A
          client can use one allowance just before a boundary and another just
          after it, creating a short burst. This implementation checks one
          policy per request; it does not combine several policies into one
          atomic decision. Redis provides shared counters, while RAM remains
          local to each gateway.
        </p>
      </section>
      <section aria-labelledby="next" className="space-y-4 pb-6">
        <h2 id="next" className="text-xl font-semibold tracking-tight">
          Start here
        </h2>
        <div className="grid gap-4 sm:grid-cols-2">
          {[
            {
              href: '/system-design/rate-limiter/docs',
              title: 'Installation guide',
              description:
                'Pull the Docker image, run the local demo, connect your backend, and enable Redis.',
            },
            {
              href: '/system-design/rate-limiter/fixed-window',
              title: 'Fixed Window Counter',
              description:
                'Explore the request flow, configure policies, and simulate RAM and Redis decisions.',
            },
          ].map(({ href, title, description }) => (
            <Link
              key={href}
              href={href}
              className="group rounded-xl border border-border p-5 transition-colors hover:bg-accent focus-visible:outline-2 focus-visible:outline-ring"
            >
              <h3 className="flex items-center justify-between gap-3 text-sm font-medium">
                {title}
                <ArrowRight className="size-4 text-muted-foreground group-hover:text-foreground" />
              </h3>
              <p className="mt-2 text-sm leading-6 text-muted-foreground">
                {description}
              </p>
            </Link>
          ))}
        </div>
      </section>
    </div>
  )
}
