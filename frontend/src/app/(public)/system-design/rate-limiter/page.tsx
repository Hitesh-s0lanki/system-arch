import type { Metadata } from 'next'
import Link from 'next/link'
import { ArrowRight, Zap } from 'lucide-react'
import { DocsIndex } from './_components/DocsIndex'

export const metadata: Metadata = {
  title: 'Rate Limiter | Overview',
  description:
    'Five rate-limiting algorithms, a Go SDK and Docker reverse proxy, with local RAM or shared Redis storage.',
}

export default function RateLimiterPage() {
  return (
    <div className="mx-auto w-full max-w-5xl min-w-0 space-y-10 px-5 py-8 md:px-10 md:py-12 lg:pr-64 [&_h2]:scroll-mt-20">
      <DocsIndex
        sections={[
          ['integration', 'Integration options'],
          ['request-flow', 'Request flow'],
          ['algorithms', 'Algorithms'],
          ['storage', 'Storage modes'],
          ['policies', 'Identity and policies'],
          ['limits', 'Behavior and limits'],
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
          Control request rates with fixed windows, sliding-window logs,
          sliding-window counters, token buckets, or leaky buckets. Embed the Go
          SDK in your application or deploy the Docker reverse proxy in front of
          an HTTP backend. Each algorithm supports RAM and Redis storage.
        </p>
      </header>
      <section aria-labelledby="integration" className="space-y-4">
        <h2 id="integration" className="text-xl font-semibold tracking-tight">
          Two ways to use the limiter
        </h2>
        <div className="grid gap-4 sm:grid-cols-2">
          {[
            [
              'Go SDK',
              'Create a reusable client in your Go application. Select a policy and pass a trusted identity to Check before running an operation. Your application handles admission, rejection, and storage errors.',
            ],
            [
              'Docker image',
              'Run the reverse proxy in front of your existing HTTP service. Configure routes and policies in JSON; the gateway resolves identities, checks quotas, and forwards admitted requests to your upstream.',
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
      <section aria-labelledby="request-flow" className="space-y-4">
        <h2 id="request-flow" className="text-xl font-semibold tracking-tight">
          How a request moves through the system
        </h2>
        <p className="text-sm leading-7 text-muted-foreground">
          The gateway matches a route to a policy, resolves the caller’s
          identity, and evaluates the configured algorithm. Requests within the
          limit reach the upstream. Once the allowance is exhausted, the gateway
          returns 429 with a Retry-After header without calling the backend.
        </p>
        <ol className="grid gap-3 sm:grid-cols-3">
          {[
            [
              'Match a policy',
              'Routes select the limit, window, identity type, and policy revision.',
            ],
            [
              'Evaluate the allowance',
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
      <section aria-labelledby="algorithms" className="space-y-4">
        <h2 id="algorithms" className="text-xl font-semibold tracking-tight">
          Choose an algorithm
        </h2>
        <ul className="space-y-3 text-sm leading-7 text-muted-foreground">
          <li>
            <strong className="text-foreground">Fixed window:</strong> count
            admitted requests in aligned intervals; allowances reset at each
            boundary, which can allow short boundary bursts.
          </li>
          <li>
            <strong className="text-foreground">Sliding-window log:</strong>{' '}
            track admitted request timestamps for an exact rolling limit, with
            more state per identity.
          </li>
          <li>
            <strong className="text-foreground">Sliding-window counter:</strong>{' '}
            weight the current and previous window counts to approximate rolling
            usage with less state.
          </li>
          <li>
            <strong className="text-foreground">Token bucket:</strong> refill
            tokens over time and allow bursts up to the configured capacity.
          </li>
          <li>
            <strong className="text-foreground">Leaky bucket:</strong> drain
            accumulated usage at a steady rate and reject requests when capacity
            is full. Admitted requests are forwarded immediately; this
            implementation does not queue them.
          </li>
        </ul>
      </section>
      <section aria-labelledby="storage" className="space-y-4">
        <h2 id="storage" className="text-xl font-semibold tracking-tight">
          Two storage modes for every algorithm
        </h2>
        <div className="grid gap-4 sm:grid-cols-2">
          {[
            [
              'RAM for a single process',
              'Algorithm state lives in the client or gateway process. Concurrent access is synchronized, keys are bounded, and expired entries are cleaned up. Each process has its own allowance, and a restart clears its counters.',
            ],
            [
              'Redis for shared quotas',
              'Replicas share algorithm state using the same namespace and policy settings. A Lua script checks capacity, updates state, and sets expiry atomically on Redis. The proxy returns 503 on storage failures; the SDK returns an error. Neither falls back to RAM.',
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
          Admission, failures, and quota boundaries
        </h2>
        <p className="mt-2 text-sm leading-7 text-muted-foreground">
          Each check evaluates one policy; several checks do not form an atomic
          decision. Admitted attempts consume quota even if the application or
          upstream later fails. RAM state is local and resets on restart. Redis
          shares state across replicas with matching namespace, revision, and
          policy settings. Change revisions through a coordinated cutover: a new
          revision starts a fresh allowance. The proxy returns 429 for exhausted
          quota and 503 for storage or identity-capacity failures.
        </p>
      </section>
      <section aria-labelledby="next" className="space-y-4 pb-6">
        <h2 id="next" className="text-xl font-semibold tracking-tight">
          Start here
        </h2>
        <div className="grid gap-4 sm:grid-cols-2">
          {[
            {
              href: '/system-design/rate-limiter/algorithms',
              title: 'Algorithms',
              description:
                'Fixed Window, Sliding Log, Sliding Counter, Token Bucket, Leaky Bucket — how each works and when to use it.',
            },
            {
              href: '/system-design/rate-limiter/docs',
              title: 'Installation guide',
              description:
                'Set up the Go SDK or Docker image with the required configuration, functions, and Redis settings.',
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
