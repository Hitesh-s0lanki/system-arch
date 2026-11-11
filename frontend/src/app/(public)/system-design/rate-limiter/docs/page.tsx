import type { Metadata } from 'next'
import Link from 'next/link'
import { ArrowLeft, Container } from 'lucide-react'
import { DocsIndex } from '../_components/DocsIndex'
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
    'Install the Go SDK or Docker image with required configuration, SDK functions, identity inputs, and optional Redis storage.',
}

const modulePath =
  'github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting'
const image = 'hiteshs0lanki/distributed-rate-limiter:latest'
const memoryExample = `package main

import (
    "context"
    "fmt"
    "log"
    "time"

    "${modulePath}/algorithm/fixedwindow"
    "${modulePath}/identity"
    "${modulePath}/policy"
    "${modulePath}/sdk"
    "${modulePath}/service"
)

func main() {
    // Construct once at startup; reuse across requests.
    policies := []sdk.Policy{{
        Policy: policy.Policy{
            ID: "orders", Identity: identity.User, PerRoute: true,
        },
        FixedWindow: fixedwindow.Config{
            Limit: 100, Window: time.Minute, MaxKeys: 10000,
        },
    }}
    client, err := sdk.NewMemory(policies)
    if err != nil { log.Fatal(err) }

    // Replace these example IDs with verified application identities.
    decision, err := client.Check(context.Background(), "orders", service.Request{
        TenantID: "demo", UserID: "user-42", Route: "POST /orders",
    })
    if err != nil { log.Fatal(err) }
    if !decision.Allowed {
        fmt.Printf("rejected: %s; retry after %s\\n", decision.Reason, decision.RetryAfter)
        return
    }
    // Run the protected operation only after admission.
    fmt.Printf("allowed; remaining=%d\\n", decision.Remaining)
}`
const redisExample = `// Additional import: redis "github.com/redis/go-redis/v9"
// Use the policies from the RAM example; replace sdk.NewMemory with this.
backend := redis.NewClient(&redis.Options{
    Addr: "127.0.0.1:6379",
    MaxRetries: -1,
    DialTimeout: time.Second,
    ReadTimeout: time.Second,
    WriteTimeout: time.Second,
    ContextTimeoutEnabled: true,
})
// Close only after application requests have finished.
defer backend.Close()

client, err := sdk.NewRedis(backend, sdk.RedisConfig{
    Policies: policies,
    Namespace: "orders-service",
    Revision: "v1",
    Timeout: 250 * time.Millisecond,
})
if err != nil { log.Fatal(err) }
ctx, cancel := context.WithTimeout(context.Background(), time.Second)
defer cancel()
if err := client.Ready(ctx); err != nil { log.Fatal(err) }
// Use client.Check with each request's context as in the RAM example.`
const proxyConfig = `{
  "version": 1,
  "listen": ":8080",
  "upstream": "http://your-backend:3000",
  "storage": {
    "mode": "ram",
    "namespace": "orders-service",
    "timeout": "500ms"
  },
  "cleanup_interval": "1s",
  "upstream_timeout": "10s",
  "anonymous_tenant": "public",
  "trusted_proxies": [],
  "forward_authorization": false,
  "routes": [{
    "pattern": "/",
    "algorithm": "fixed_window",
    "policy": { "id": "orders-ip", "identity": "ip", "per_route": false },
    "limit": 100,
    "window": "1m",
    "max_keys": 10000,
    "revision": "v1"
  }],
  "max_body_bytes": 1048576,
  "max_concurrent": 128
}`

const constructors = [
  ['Fixed window', 'FixedWindow / fixedwindow.Config', 'NewMemory / NewRedis'],
  [
    'Sliding-window log',
    'SlidingWindow / slidingwindow.Config',
    'NewSlidingWindowMemory / NewSlidingWindowRedis',
  ],
  [
    'Sliding-window counter',
    'SlidingCounter / slidingcounter.Config',
    'NewSlidingCounterMemory / NewSlidingCounterRedis',
  ],
  [
    'Token bucket',
    'TokenBucket / tokenbucket.Config',
    'NewTokenBucketMemory / NewTokenBucketRedis',
  ],
  [
    'Leaky bucket',
    'LeakyBucket / leakybucket.Config',
    'NewLeakyBucketMemory / NewLeakyBucketRedis',
  ],
]

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

function ReferenceTable({
  caption,
  headings,
  rows,
}: {
  caption: string
  headings: string[]
  rows: string[][]
}) {
  return (
    <div className="overflow-x-auto rounded-lg border">
      <table className="w-full text-left text-sm">
        <caption className="sr-only">{caption}</caption>
        <thead className="bg-muted/40">
          <tr>
            {headings.map((heading) => (
              <th key={heading} scope="col" className="px-4 py-3 font-medium">
                {heading}
              </th>
            ))}
          </tr>
        </thead>
        <tbody className="divide-y divide-border">
          {rows.map((row) => (
            <tr key={row[0]}>
              {row.map((cell, index) =>
                index === 0 ? (
                  <th
                    key={index}
                    scope="row"
                    className="px-4 py-3 align-top font-medium"
                  >
                    {cell}
                  </th>
                ) : (
                  <td
                    key={index}
                    className="px-4 py-3 align-top text-muted-foreground"
                  >
                    {cell}
                  </td>
                ),
              )}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

export default function RateLimiterDocsPage() {
  return (
    <div className="mx-auto w-full max-w-6xl min-w-0 px-5 py-8 md:px-10 md:py-12 lg:pr-64">
      <DocsIndex
        sections={[
          ['go-sdk', 'Go SDK'],
          ['docker-image', 'Docker image'],
        ]}
      />
      <Link
        href="/system-design/rate-limiter"
        className="inline-flex items-center gap-2 text-sm text-muted-foreground hover:text-foreground"
      >
        <ArrowLeft className="size-3.5" /> Rate Limiter overview
      </Link>
      <header className="mt-7 border-b border-border pb-8">
        <div className="mb-4 flex items-center gap-2 text-xs font-medium tracking-widest text-muted-foreground uppercase">
          <Container className="size-4" /> Installation guide
        </div>
        <h1 className="text-3xl font-semibold tracking-tight md:text-4xl">
          Go SDK or Docker image
        </h1>
        <p className="mt-4 max-w-2xl text-base leading-7 text-muted-foreground">
          Embed rate limiting in your Go application or run the reverse proxy in
          front of your HTTP backend. Both options support all five algorithms
          with local RAM or shared Redis state.
        </p>
      </header>
      <article className="mt-8 min-w-0 space-y-12 text-sm leading-7 [&_h2]:text-xl [&_h2]:font-semibold [&_h2]:tracking-tight [&_h3]:text-base [&_h3]:font-medium [&_section]:scroll-mt-20">
        <section id="go-sdk" className="space-y-5">
          <h2>1. Go SDK</h2>
          <p className="text-muted-foreground">
            Requires Go 1.26.10 or newer, as declared by this module. RAM needs
            no external service. Redis mode also needs a reachable Redis server
            and a caller-owned go-redis/v9 client. Create one SDK client at
            startup and reuse it across requests.
          </p>
          <h3>Install from a local checkout</h3>
          <p className="text-muted-foreground">
            In your existing Go module, point the declared import path at the
            checked-out module. Replace the relative path below with your actual
            checkout location; the on-disk directory is named{' '}
            <code>disturbuted-rate-limiting</code>.
          </p>
          <Snippet title="Terminal · your Go application">{`go mod edit -replace=${modulePath}=../system-arch/disturbuted-rate-limiting
go get ${modulePath}/sdk`}</Snippet>
          <h3>Required policy configuration</h3>
          <ReferenceTable
            caption="SDK configuration requirements"
            headings={['Field', 'Requirement']}
            rows={[
              [
                'Policy.ID',
                'Unique, nonblank policy ID, at most 64 bytes. Pass this same ID to Check.',
              ],
              [
                'Policy.Identity',
                'identity.IP, identity.User, or identity.APIKey. Resolve the selected identity in trusted application code.',
              ],
              [
                'Policy.PerRoute',
                'When true, every Check requires a canonical Route such as POST /orders/{id}.',
              ],
              [
                'Algorithm config',
                'Use the field and constructor from the table below. Limit, Window, and MaxKeys must be positive. TokenBucket and LeakyBucket also require positive Capacity.',
              ],
              [
                'RedisConfig',
                'Policies, Namespace, Revision, and a positive Timeout. Replicas must use identical policy and identity settings within the same namespace and revision. Redis windows must be whole milliseconds, at least 1ms.',
              ],
            ]}
          />
          <Snippet title="main.go · fixed-window RAM client">
            {memoryExample}
          </Snippet>
          <h3>Algorithm constructors</h3>
          <ReferenceTable
            caption="SDK algorithm fields and constructors"
            headings={[
              'Algorithm',
              'sdk.Policy field / config type',
              'sdk constructors: RAM / Redis',
            ]}
            rows={constructors}
          />
          <p className="text-muted-foreground">
            Import each config type from{' '}
            <code className="break-all">{modulePath}/algorithm/</code>
            followed by its package name. RAM constructors take{' '}
            <code>[]sdk.Policy</code>; Redis constructors take a{' '}
            <code>redis.UniversalClient</code> and <code>sdk.RedisConfig</code>.
            Each constructor uses its matching algorithm field for every policy.
          </p>
          <h3>Required functions and request inputs</h3>
          <ReferenceTable
            caption="SDK functions and caller responsibilities"
            headings={['Function', 'Requirements and result']}
            rows={[
              [
                'sdk.New…(...) (*sdk.Client, error)',
                'Construct before serving requests and handle validation errors. Redis constructors do not connect; call Ready before accepting traffic.',
              ],
              [
                'client.Ready(ctx) error',
                'Use a bounded startup context to verify Redis writes and policy compatibility without consuming quota. RAM checks context cancellation only.',
              ],
              [
                'client.Check(ctx, policyID, service.Request) (Decision, error)',
                'Pass the request context, a registered policy ID, TenantID, and IP, UserID, or APIKeyID to match Policy.Identity. Route is required when PerRoute is true. APIKeyID is a verified credential ID, never the secret.',
              ],
              [
                'Decision',
                'Run the operation only when err is nil and Allowed is true. Inspect Reason for rejections; RetryAfter is a hint only for rate_limited. Capacity rejection is distinct from exhausted quota. Storage errors propagate to the caller.',
              ],
              [
                'httpmiddleware.Wrap(checker, config, next) (http.Handler, error)',
                'Optional net/http integration from middleware/http. Pass the SDK client, the same registered Policy, and a non-nil next handler. PerRoute requires Route; IP policies require AnonymousTenant; user and API-key policies require an Authenticator.',
              ],
              [
                'client.CleanupExpired() int',
                'Optional RAM memory reclamation during idle periods; safe alongside Check. Returns the number of removed identities. Redis expires its own state.',
              ],
              [
                'backend.Close() error',
                'Close your Redis connection after active requests finish. The SDK client has no Close method and starts no workers.',
              ],
            ]}
          />
          <h3>Optional shared Redis configuration</h3>
          <Snippet title="Replace RAM construction with Redis">
            {redisExample}
          </Snippet>
          <p className="text-muted-foreground">
            Disable automatic retries with <code>MaxRetries: -1</code> and
            enable context timeouts. A timed-out check may already have consumed
            quota. The SDK propagates backend errors without retrying or falling
            back to RAM. For intentional policy changes, use a new revision and
            coordinate replicas; this starts fresh quota.
          </p>
        </section>
        <section id="docker-image" className="space-y-5">
          <h2>2. Docker image</h2>
          <p className="text-muted-foreground">
            Requires a running Docker engine, a backend reachable from the
            container, and an available host port. Go is not required. Redis is
            required only for shared quotas. The image runs the gateway; it does
            not require any SDK functions in your backend.
          </p>
          <h3>Required gateway configuration</h3>
          <p className="text-muted-foreground">
            Save this as <code>proxy.json</code>. Replace the upstream with your
            backend service name and port on a shared Docker network. On Docker
            Desktop, use <code>http://host.docker.internal:3000</code> for a
            host service. Container <code>localhost</code> refers to the
            container itself.
          </p>
          <Snippet title="proxy.json">{proxyConfig}</Snippet>
          <ReferenceTable
            caption="Docker gateway configuration requirements"
            headings={['Field', 'Requirement']}
            rows={[
              [
                'version / upstream / routes',
                'version must be 1. Supply an HTTP(S) upstream and at least one route. Unmatched paths return 404. Unknown JSON fields fail startup.',
              ],
              [
                'routes[].pattern / policy',
                'Go ServeMux pattern, plus a policy ID and identity (ip, user, or api_key). per_route groups quotas by canonical route pattern. Patterns reusing a policy ID must have identical settings.',
              ],
              [
                'routes[].algorithm',
                'fixed_window (default), sliding_window_log, sliding_window_counter, token_bucket, or leaky_bucket.',
              ],
              [
                'routes[].limit / window / max_keys',
                'Positive request rate, duration such as 1m, and maximum tracked identities. Bucket algorithms also need positive capacity. max_keys bounds identities, while capacity controls burst size.',
              ],
              [
                'routes[].revision',
                'Defaults to v1. Keep identical across replicas; use a new revision for coordinated policy changes that start fresh quota.',
              ],
              [
                'storage',
                'mode is ram or redis. Shared replicas need the same namespace and policy settings. timeout bounds Redis operations; Redis mode also needs redis_url or REDIS_URL.',
              ],
              [
                'anonymous_tenant / trusted_proxies',
                'Tenant for IP policies. Only configured proxy CIDRs may supply forwarded client IPs; an empty list trusts none.',
              ],
              [
                'User / API-key authentication',
                'Set policy.identity to user or api_key and configure verified bearer credentials through RATE_LIMIT_CREDENTIALS, or configure OIDC with an explicit tenants list. No default credentials are enabled.',
              ],
              [
                'cleanup_interval / upstream_timeout',
                'Positive durations for RAM cleanup and admitted upstream requests. Config loads at startup; restart to apply edits.',
              ],
              [
                'max_body_bytes / max_concurrent',
                'Bound request body size and simultaneous gateway requests. The example uses 1 MiB and 128 requests.',
              ],
            ]}
          />
          <Snippet title="Terminal · start the gateway">{`docker pull ${image}
docker run -d --name rate-limiter \\
  --network your-app-network \\
  -p 127.0.0.1:8088:8080 \\
  --read-only --cap-drop ALL \\
  --security-opt no-new-privileges:true \\
  -v "$PWD/proxy.json:/etc/ratelimit/proxy.json:ro" \\
  ${image}`}</Snippet>
          <p className="text-muted-foreground">
            Replace <code>your-app-network</code> with an existing network
            shared with your backend. Send requests to{' '}
            <code>http://localhost:8088</code>. Keep the backend private so
            requests pass through the gateway.
          </p>
          <h3>Optional Redis and authentication settings</h3>
          <p className="text-muted-foreground">
            To share quotas, add these environment flags before the image name
            in the run command. Redis must be reachable on the container
            network. Use <code>rediss://</code> for TLS and supply credentials
            through your deployment environment.
          </p>
          <Snippet title="Redis environment flags">{`-e RATE_LIMIT_STORAGE=redis \\
-e REDIS_URL=redis://your-redis:6379/0 \\
-e RATE_LIMIT_NAMESPACE=orders-service`}</Snippet>
          <p className="text-muted-foreground">
            For user or API-key policies, supply{' '}
            <code>RATE_LIMIT_CREDENTIALS</code>
            as a JSON array in the format below, using your own secret token and
            verified IDs. <code>user_id</code> or <code>api_key_id</code> must
            match the selected identity type. Enable{' '}
            <code>forward_authorization</code>
            only if the upstream needs the original Authorization header.
          </p>
          <Snippet title="RATE_LIMIT_CREDENTIALS · JSON format">{`[{
  "token": "<your-secret-bearer-token>",
  "tenant_id": "demo",
  "user_id": "user-42",
  "api_key_id": "credential-7"
}]`}</Snippet>
          <p className="text-muted-foreground">
            Environment overrides are <code>RATE_LIMIT_LISTEN</code>,{' '}
            <code>RATE_LIMIT_UPSTREAM</code>, <code>RATE_LIMIT_STORAGE</code>,{' '}
            <code>REDIS_URL</code>, and <code>RATE_LIMIT_NAMESPACE</code>. They
            override corresponding JSON fields.
          </p>
          <h3>Gateway responses</h3>
          <ReferenceTable
            caption="Gateway health endpoints and response behavior"
            headings={['Endpoint or status', 'Behavior']}
            rows={[
              [
                '/healthz / /readyz',
                'Liveness and readiness endpoints; neither consumes quota. Redis readiness checks storage writes and policy compatibility, not upstream health.',
              ],
              ['429', 'Quota exhausted. Retry-After provides the retry delay.'],
              [
                '503',
                'Storage failure or identity capacity exhausted. The request is not forwarded; there is no automatic RAM fallback.',
              ],
              [
                '502 / 504',
                'Upstream unreachable or upstream timeout. The admitted attempt has already consumed quota.',
              ],
            ]}
          />
          <p className="text-muted-foreground">
            For algorithm details and configurable examples, see the{' '}
            <Link
              href="/system-design/rate-limiter/algorithms"
              className="text-foreground underline underline-offset-4"
            >
              algorithms guide
            </Link>
            .
          </p>
        </section>
      </article>
    </div>
  )
}
