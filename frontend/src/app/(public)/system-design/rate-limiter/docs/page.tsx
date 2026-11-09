import type { Metadata } from 'next'
import { DocsIndex } from '../_components/DocsIndex'
import Link from 'next/link'
import { ArrowLeft, ArrowUpRight, Container } from 'lucide-react'
import {
  CodeBlock,
  CodeBlockActions,
  CodeBlockContent,
  CodeBlockCopyButton,
  CodeBlockHeader,
  CodeBlockTitle,
} from '@/components/ui/code-block'

export const metadata: Metadata = {
  title: 'Rate Limiter | Installation guide',
  description:
    'Run the distributed rate limiter image with Docker, connect a backend, and share quotas with Redis.',
}

const image = 'hiteshs0lanki/distributed-rate-limiter:latest'
const compose = `name: rate-limiter-demo
services:
  gateway:
    image: ${image}
    ports:
      - "127.0.0.1:8088:8080"
    environment:
      RATE_LIMIT_STORAGE: ram
      RATE_LIMIT_UPSTREAM: http://backend:8080
    read_only: true
    cap_drop: [ALL]
    security_opt: [no-new-privileges:true]
    depends_on:
      backend:
        condition: service_healthy

  backend:
    image: ${image}
    entrypoint: ["/demo-backend"]
    command: []
    healthcheck:
      test: [CMD, /ratelimitd, -healthcheck, "http://127.0.0.1:8080/healthz"]
      interval: 5s
      timeout: 3s
      retries: 5`

const redis = `services:
  gateway:
    environment:
      RATE_LIMIT_STORAGE: redis
      REDIS_URL: redis://redis:6379/0
    depends_on:
      redis:
        condition: service_healthy

  redis:
    image: redis:7.4.7-alpine
    command: [redis-server, --appendonly, "yes", --maxmemory, 128mb, --maxmemory-policy, noeviction]
    volumes:
      - redis-data:/data
    healthcheck:
      test: [CMD, redis-cli, ping]
      interval: 2s
      timeout: 2s
      retries: 15

volumes:
  redis-data:`

function Snippet({ title, children }: { title: string; children: string }) {
  return (
    <CodeBlock className="bg-muted/20">
      <CodeBlockHeader>
        <CodeBlockTitle>{title}</CodeBlockTitle>
        <CodeBlockActions>
          <CodeBlockCopyButton value={children} />
        </CodeBlockActions>
      </CodeBlockHeader>
      <CodeBlockContent>{children}</CodeBlockContent>
    </CodeBlock>
  )
}

const sections = [
  ['quick-start', 'Quick start'],
  ['verify', 'Verify requests'],
  ['your-backend', 'Connect your backend'],
  ['redis', 'Share quotas with Redis'],
  ['operations', 'Operate the gateway'],
] as const

export default function RateLimiterDocsPage() {
  return (
    <div className="mx-auto w-full max-w-6xl min-w-0 px-5 py-8 md:px-10 md:py-12 lg:pr-64">
      <Link
        href="/system-design/rate-limiter"
        className="inline-flex items-center gap-2 text-sm text-muted-foreground hover:text-foreground"
      >
        <ArrowLeft className="size-3.5" /> Rate Limiter overview
      </Link>
      <header className="mt-7 border-b border-border pb-8">
        <div className="mb-4 flex items-center gap-2 text-xs font-medium tracking-widest text-muted-foreground uppercase">
          <Container className="size-4" /> Deployment guide
        </div>
        <h1 className="text-3xl font-semibold tracking-tight md:text-4xl">
          Run the rate limiter with Docker
        </h1>
        <p className="mt-4 max-w-2xl text-base leading-7 text-muted-foreground">
          Put the gateway in front of your API, try the fixed-window limits
          locally, then use Redis to share quotas across replicas.
        </p>
        <div className="mt-5 flex flex-wrap items-center gap-3 text-xs text-muted-foreground">
          <span className="rounded-md border px-2 py-1">
            Linux amd64 + arm64
          </span>
          <span className="rounded-md border px-2 py-1">Non-root image</span>
          <a
            href="https://hub.docker.com/r/hiteshs0lanki/distributed-rate-limiter"
            target="_blank"
            rel="noreferrer"
            className="inline-flex items-center gap-1 underline underline-offset-4"
          >
            View on Docker Hub <ArrowUpRight className="size-3" />
          </a>
        </div>
      </header>
      <div className="mt-8 grid min-w-0 gap-10">
        <article className="min-w-0 space-y-12 text-sm leading-7 [&_h2]:mb-3 [&_h2]:text-xl [&_h2]:font-semibold [&_h2]:tracking-tight [&_p]:mb-4 [&_p]:text-muted-foreground [&_section]:scroll-mt-16">
          <section id="quick-start">
            <h2>1. Start a local demo</h2>
            <p>
              Install Docker with Compose and start the Docker engine. No Go
              installation or repository checkout is needed. Pull the published
              image:
            </p>
            <Snippet title="Terminal">{`docker pull ${image}`}</Snippet>
            <p className="mt-5">
              In an empty directory, save this as <code>compose.yaml</code>. It
              starts the gateway and the demo backend included in the same
              image. Only the gateway publishes a host port.
            </p>
            <Snippet title="compose.yaml">{compose}</Snippet>
            <div className="mt-4">
              <Snippet title="Terminal">
                {'docker compose up -d --wait'}
              </Snippet>
            </div>
            <p className="mt-4">
              The gateway is available at <code>http://localhost:8088</code>.
              Its bundled configuration allows three requests per client IP in
              each fixed 10-second window. RAM mode keeps counters in one
              gateway process.
            </p>
          </section>
          <section id="verify">
            <h2>2. Verify forwarding and limits</h2>
            <p>Check readiness, then send four requests in quick succession:</p>
            <Snippet title="Terminal">{`curl -i http://localhost:8088/readyz

curl -i http://localhost:8088/orders
curl -i http://localhost:8088/orders
curl -i http://localhost:8088/orders
curl -i http://localhost:8088/orders`}</Snippet>
            <p className="mt-4">
              Readiness returns <code>200</code>. Within one window, the first
              three order requests return <code>200</code> and the fourth
              returns <code>429</code> with a <code>Retry-After</code> header.
              If the requests cross a window boundary, the allowance resets.
              Wait for the next window to try again.
            </p>
            <p>
              Readiness checks do not consume quota. The demo backend echoes
              forwarded requests, so you can also try a POST after the quota
              resets:
            </p>
            <Snippet title="Terminal">{`curl -i -X POST 'http://localhost:8088/orders?source=demo' -d 'hello'`}</Snippet>
          </section>
          <section id="your-backend">
            <h2>3. Connect your own backend</h2>
            <p>
              For a backend on an existing Docker network, run the gateway on
              that network. Replace <code>your-app-network</code> and{' '}
              <code>your-backend:3000</code> with your network and service
              address:
            </p>
            <Snippet title="Terminal">{`docker run -d --name rate-limiter \\
  --network your-app-network \\
  -p 127.0.0.1:8089:8080 \\
  --read-only --cap-drop ALL \\
  --security-opt no-new-privileges:true \\
  -e RATE_LIMIT_UPSTREAM=http://your-backend:3000 \\
  ${image}`}</Snippet>
            <p className="mt-4">
              Send API traffic to <code>http://localhost:8089</code>. On Docker
              Desktop, a backend running on your host can use{' '}
              <code>http://host.docker.internal:3000</code> as the upstream.
              Inside a container, <code>localhost</code> refers to that
              container.
            </p>
            <p>
              To customize routes, limits, windows, or identity policies, mount
              your complete version-1 proxy configuration with{' '}
              <code>
                -v &quot;$PWD/proxy.json:/etc/ratelimit/proxy.json:ro&quot;
              </code>
              . Restart the container after changing the file. The architecture
              page has a configuration editor for exploring policies.
            </p>
          </section>
          <section id="redis">
            <h2>4. Share quotas with Redis</h2>
            <p>
              Use Redis when multiple gateways need one shared allowance. Save
              the following beside <code>compose.yaml</code> as{' '}
              <code>compose.redis.yaml</code>. Redis stays on the private
              Compose network.
            </p>
            <Snippet title="compose.redis.yaml">{redis}</Snippet>
            <div className="mt-4">
              <Snippet title="Terminal">{`docker compose down
docker compose -f compose.yaml -f compose.redis.yaml up -d --wait`}</Snippet>
            </div>
            <p className="mt-4">
              Replicas must use the same namespace and policy settings,
              including revisions. Configure <code>RATE_LIMIT_NAMESPACE</code>{' '}
              to separate deployments. Redis counters survive gateway restarts
              until expiry; if Redis becomes unavailable, the gateway returns{' '}
              <code>503</code> and does not forward requests.
            </p>
          </section>
          <section id="operations">
            <h2>5. Inspect, update, and stop</h2>
            <Snippet title="Terminal · RAM demo">{`docker compose logs -f gateway

# Pull the latest image and recreate the services
docker compose pull
docker compose up -d --wait

# Stop and remove the demo containers
docker compose down`}</Snippet>
            <p className="mt-4">
              For Redis mode, include{' '}
              <code>-f compose.yaml -f compose.redis.yaml</code> in each Compose
              command. Stopping the stack preserves the Redis volume; adding{' '}
              <code>-v</code> deletes it and resets stored state. Remove the
              standalone gateway with <code>docker rm -f rate-limiter</code>.
            </p>
            <div className="overflow-x-auto rounded-lg border">
              <table className="w-full text-left text-sm">
                <caption className="sr-only">
                  Gateway health endpoints and error responses
                </caption>
                <thead className="bg-muted/40">
                  <tr>
                    <th scope="col" className="px-4 py-2">
                      Response
                    </th>
                    <th scope="col" className="px-4 py-2">
                      Meaning
                    </th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-border">
                  {[
                    ['/healthz', 'Gateway process is alive.'],
                    [
                      '/readyz',
                      'Gateway is ready; includes Redis checks in Redis mode, but does not check the upstream.',
                    ],
                    [
                      '429',
                      'Request allowance exhausted. Retry after the indicated delay.',
                    ],
                    ['503', 'Rate-limit storage or key capacity unavailable.'],
                    [
                      '502 / 504',
                      'Upstream unreachable or timed out. Check its address and network.',
                    ],
                  ].map(([code, meaning]) => (
                    <tr key={code}>
                      <th
                        scope="row"
                        className="px-4 py-3 font-mono font-normal"
                      >
                        {code}
                      </th>
                      <td className="px-4 py-3 text-muted-foreground">
                        {meaning}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
            <p className="mt-4">
              The examples bind to loopback for local use. For a public
              deployment, terminate TLS at your ingress and keep the backend
              private so clients cannot bypass the gateway. The{' '}
              <code>latest</code> tag can change; pin a tested image digest for
              reproducible deployments.
            </p>
          </section>
        </article>
        <DocsIndex sections={sections} />
      </div>
    </div>
  )
}
