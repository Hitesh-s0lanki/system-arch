# Distributed rate limiter

A Go rate limiter with embedded-library and reverse-proxy integrations.
Fixed-window counters, sliding-window logs, sliding-window counters, and token buckets support RAM or Redis
storage. It can run as
an embedded Go library, a demonstration server, or a configurable reverse proxy.

See [rate-limiting algorithms](docs/algorithms.md) for a comparison of alternative
algorithms, sliding-window timestamp storage, and token-bucket SDK/proxy configuration. See the
[sliding-window guide](docs/sliding-window.md) and
[counter guide](docs/sliding-window-counter.md) for configuration and tests.

For embedding in an application, use the [Go SDK](docs/sdk.md):
`sdk.NewMemory` or `sdk.NewRedis` constructs the policy registry and independent
fixed-window limiters. Reuse the client for `Check` calls or pass it directly to
the HTTP middleware. Run `go run ./examples/sdk` for a complete local example.

## Docker quick start

RAM mode (gateway plus a private demonstration backend):

```sh
docker compose up -d --build --wait
curl -i http://127.0.0.1:8088/orders
```

Redis mode (stop the RAM stack before switching):

```sh
docker compose down
docker compose -f compose.yaml -f compose.redis.yaml up -d --build --wait
```

Edit `configs/proxy.json` to choose upstream, routes, limits, identities, and
timeouts. RAM keeps quotas per process; Redis shares them across replicas using
the same namespace and configuration. Redis failures return 503 without a RAM
fallback. See [the Docker guide](docs/docker.md) for configuration, authentication,
existing backend networks, storage guarantees, and shutdown commands.

## Layers

```text
Trusted application code resolves identity and chooses a policy
    → service.Check(ctx, policyID, request)
    → policy selects IP, user ID, or API credential ID
    → identity validates and builds a tenant/policy/subject/route key
    → bound algorithm evaluates the key
    → shared Decision, or a backend/validation error
```

| Package | Responsibility |
| --- | --- |
| Root `ratelimit` | Algorithm-independent decision and rejection reasons |
| `sdk` | Embedded RAM/Redis constructors, policy checks, readiness, and cleanup |
| `algorithm/fixedwindow` | Counters, windows, capacity, cleanup, concurrency |
| `algorithm/fixedwindow/redis` | Atomic Redis decisions, capacity, and expiry |
| `algorithm/slidingcounter` | Weighted two-window counters with bounded RAM storage |
| `algorithm/slidingcounter/redis` | Shared atomic counters and identity expiry |
| `algorithm/slidingwindow` | Rolling timestamp logs with bounded RAM storage |
| `algorithm/slidingwindow/redis` | Shared atomic rolling logs and identity expiry |
| `algorithm/leakybucket` | Continuously draining buckets with bounded RAM storage |
| `algorithm/leakybucket/redis` | Shared atomic bucket admission and idle expiry |
| `identity` | Bounded identity inputs, canonical IP addresses, namespaced keys |
| `policy` | Identity choice and optional separation by canonical route |
| `service` | Immutable policy registry and context-aware backend dispatch |
| `middleware/http` | HTTP decisions, authentication hooks, trusted client IPs |
| `auth` | Verified principals and static hashed bearer-token adapter |
| `maintenance` | Caller-owned periodic cleanup with cancellation |
| `internal/app` | Routes, policy wiring, HTTP lifecycle, cleanup ownership |
| `internal/proxyapp` | Configurable upstream forwarding with RAM or Redis |
| `cmd/ratelimitd` | Runnable local server and startup configuration |
| `examples/identitybased` | Runnable IP, user, credential, and route examples |

Tests mirror these packages under `tests/`. Existing `fixedwindow.Decision` and
reason names remain available as aliases, and direct `Allow(key)` calls still work.
Services consume an `Evaluate(context.Context, string) (Decision, error)` interface
so another algorithm or a future network-backed implementation can be bound
without changing policy selection. Backend errors propagate; no implicit fail-open
behavior is configured.

## Identity and policy rules

- Each request requires a trusted tenant ID. A single-tenant deployment can use
  an application-configured constant such as `public`; do not let callers choose
  arbitrary tenants or policies to bypass their limits.
- IP limits accept parsed addresses, not host/port pairs or raw forwarding headers.
  IPv4-mapped IPv6 addresses normalize to the same identity as their IPv4 form.
- User IDs must come from verified authentication. API credential IDs identify
  verified credentials; never supply raw API secrets.
- Missing identities are errors. There is no automatic fallback from user to IP.
- Per-route policies require a canonical method plus route template such as
  `GET /orders/{id}`. Use router metadata, not arbitrary URLs or query strings.
- Policies without per-route grouping ignore the supplied route and share one
  allowance for that subject across routes.
- Keys use length-prefixed components and SHA-256 to keep stored keys fixed-length
  and prevent ambiguous separators. Hashing is not authentication or encryption.
- Input limits are measured in bytes: policy ID 64, tenant ID 128, subject 256,
  route 256. The service validates these before consuming an allowance.
- Construct policy bindings at startup. Prefer separate limiter instances per
  policy for independent rates and capacities. Even if a backend is shared,
  policy IDs separate counters, but its capacity is shared.
- Each check evaluates one policy. Evaluating several policies atomically is not
  implemented; calling them sequentially can consume earlier allowances before a
  later rejection.

## Run and verify

```sh
go run ./examples/identitybased
go test -race ./...
go vet ./...
```

The example evaluates each policy separately and prints two allowances followed
by a rejection, provided it runs within one window. It demonstrates wiring, not
an HTTP server or authenticated endpoint.

## Run the HTTP server

From this module directory:

```sh
go run ./cmd/ratelimitd -addr 127.0.0.1:8080 -limit 3 -window 10s -max-keys 10000
```

In another terminal, repeat this request four times within a window:

```sh
curl -i http://127.0.0.1:8080/v1/ip
```

The first three requests succeed; the fourth returns 429. Anonymous IP requests
use the configured `public` tenant. Forwarding headers are ignored by default.

For authenticated routes, stop the server and restart it with local sample
credentials (the sample token is public and is for this local demonstration):

```sh
export RATE_LIMIT_CREDENTIALS="$(cat configs/credentials.example.json)"
go run ./cmd/ratelimitd -limit 3 -window 10s
```

```sh
curl -i -H 'Authorization: Bearer local-demo-token-change-me-00000001' \
  http://127.0.0.1:8080/v1/user
```

Use independently generated random tokens and TLS when integrating beyond the
local example. The server has no default credential. Omitting the environment
variable leaves authenticated routes returning 401. The environment value is a
JSON array with `token`, `tenant_id`, and at least one of `user_id` or `api_key_id`.
Tokens must be 32–4096 bytes without whitespace; duplicate tokens fail startup.
The adapter stores token digests and only passes credential IDs to rate limiting.

| Route (GET, with HEAD sharing its allowance) | Identity | Policy |
| --- | --- | --- |
| `/healthz` | None | Health endpoint, not rate limited |
| `/v1/ip` | Socket/trusted-proxy client IP | `per-ip` |
| `/v1/user` | Authenticated user ID | `per-user` |
| `/v1/api-key` | Authenticated credential ID | `per-credential` |
| `/v1/items/{id}` | Authenticated user ID + route template | `per-user-route` |
| `/v1/orders` | Authenticated user ID + route template | `per-user-route` |

Different item IDs share the `/v1/items/{id}` counter; query strings do not create
new allowances. The orders route has a separate allowance. Each route evaluates
one selected policy, not all policies together. The executable uses the same rate
settings for each policy and separate limiter instances: four policies can track
up to four times `-max-keys` entries. Embedded applications can bind distinct rates.

`auth.Bearer` is an immutable static adapter, not a user-management system.
To integrate an existing authentication provider, implement the middleware's
`Authenticator` interface and return verified tenant/user/credential identifiers.
Downstream handlers can use `httpmiddleware.PrincipalFromContext(r.Context())`.
The middleware ignores `X-User-ID`, `X-Tenant-ID`, and client-supplied policy names.

## Trusted proxies

Configure only the CIDRs of proxies that actually connect to your server:

```sh
go run ./cmd/ratelimitd -trusted-proxy 10.20.0.0/24
```

The socket peer must be trusted before `X-Forwarded-For` is considered. The chain
is walked from right to left until the first untrusted hop; a caller's forged
leftmost value is not automatically accepted. Proxies must append the observed
peer or overwrite untrusted incoming headers. `Forwarded` and `X-Real-IP` are not
used. Chains are limited to 32 hops and 8 KiB. Malformed trusted chains return
400; untrusted clients' forwarding headers are ignored. Trusting all addresses
would let clients choose their own rate-limit identities.

## HTTP outcomes and lifecycle

| Outcome | Response |
| --- | --- |
| Allowed | Handler runs; `X-RateLimit-Remaining` is set |
| Window allowance exhausted | 429 JSON, remaining zero, `Retry-After` rounded up to seconds |
| New identity at capacity | 503 JSON, no promised retry time |
| Missing/invalid bearer token | 401 JSON and `WWW-Authenticate` |
| Valid token without required user/credential identity | 403 JSON |
| Invalid trusted client-IP chain | 400 JSON |
| Authentication/backend unavailable | 503 JSON; request does not reach handler |
| Invalid server policy/identity configuration | 500 JSON |

Responses from middleware use `Cache-Control: no-store`. Internal error details
and bearer tokens are not returned. Custom `X-RateLimit-Remaining` headers are
informational, not an authentication mechanism. During clock rollback,
`Retry-After` remains a hint as described in the algorithm guide.

The server runs periodic cleanup (`-cleanup-interval`, default 1s), has HTTP
header/read/write/idle timeouts, and stops on SIGINT or SIGTERM. Shutdown closes
the listener, allows up to five seconds for active requests, stops cleanup, and
joins both workers. This demonstration mode keeps counters in RAM and does not
serve TLS itself. Proxy mode can persist unexpired counters in Redis as described
in the Docker guide.

## Operational safeguards and verification

The proxy includes bounded bodies/concurrency, per-tenant capacity isolation,
revision-guarded Redis policies, Sentinel/Cluster clients, writable readiness,
OIDC verification, authenticated Prometheus metrics, and structured request logs.
RAM rollover and cleanup retire expired maps without scanning every identity.

See [production operation](docs/production.md) for configuration, rollout contracts,
HA limits, identity integration, monitoring, Kubernetes manifests, and load tests.
Changes from the earlier prototype reset proxy Redis key scopes; use a coordinated
cutover. Revision changes deliberately create fresh allowance, so mixed revisions
must not serve a strict shared quota simultaneously.

```sh
make verify
REDIS_TEST_URL=redis://127.0.0.1:6379/0 REDIS_SERVER_BIN=/path/to/redis-server make integration
make benchmark
```

One fixed-window policy is evaluated per route. Boundary bursts and charging
admitted attempts remain explicit contracts. Multi-policy atomic transactions,
remote decision APIs, additional algorithms, and non-Go SDKs are outside this release.
Real capacity and failover durability must be accepted against your deployment's
requirements; Redis HA does not guarantee zero counter loss.

Leaky bucket meters support RAM and Redis through the SDK and proxy routes using
`"algorithm": "leaky_bucket"` with a required `capacity`. See the
[leaky bucket guide](docs/leaky-bucket.md) for configuration and behavior.

## Continuous integration and Docker publishing

Changes under this module trigger [GitHub Actions](../.github/workflows/rate-limiter.yml)
for Go tests, Redis integration, builds, container checks, and load testing.
Successful default-branch runs publish the tested image to both GHCR and Docker
Hub with `latest` and commit tags. See [registry setup and deployment](docs/docker.md#github-actions-and-published-images)
for the required Docker Hub secret/variables and commands to run a published image.
Use `make test-docker` to run the container checks locally.
