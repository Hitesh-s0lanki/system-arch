# Docker reverse proxy: RAM or Redis

Run these commands from `disturbuted-rate-limiting/`.

## Start with RAM

```sh
docker compose up -d --build --wait
curl -i http://127.0.0.1:8088/orders
```

Repeat the request four times within the same 10-second window. Three requests
reach the private demo backend; the fourth receives 429 and `Retry-After`.
The backend echoes the method, path, query, body, and number of requests received.
Try a POST to verify forwarding:

```sh
curl -i -X POST 'http://127.0.0.1:8088/orders?source=demo' -d 'hello'
```

The gateway is the only published application port. The sample upstream and Redis
have no host port. The image runs as a non-root user with a read-only filesystem;
Go compilation happens in a separate build stage.

## Switch to Redis

Stop the RAM gateway before switching so the same port is available:

```sh
docker compose down
docker compose -f compose.yaml -f compose.redis.yaml up -d --build --wait
curl -i http://127.0.0.1:8088/orders
```

The override starts Redis, waits for its health check, and selects Redis storage.
Redis counters use a named volume with AOF enabled. `down` preserves that volume;
`down -v` deletes it and resets state. RAM counters disappear on a gateway restart.

```sh
docker compose -f compose.yaml -f compose.redis.yaml logs gateway
docker compose -f compose.yaml -f compose.redis.yaml down
```

Use `.env.example` as a reference for Compose variables. Change `PROXY_PORT` if
8088 is already in use. This is a local example; it does not publish a container
image to a registry or configure external TLS termination.

## Point at your own backend

Edit `configs/proxy.json` or set `RATE_LIMIT_UPSTREAM` for Compose. The upstream
must be reachable from the gateway container. `localhost` inside the gateway
means the gateway itself. On a shared Docker network, use the backend's service
name and its container port. For Docker Desktop host services, use a reachable
host address such as `http://host.docker.internal:3000`.

For an existing Docker network without the sample stack:

```sh
docker build -t rate-limiter:local .
docker run --rm --network your-app-network -p 127.0.0.1:8088:8080 \
  -e RATE_LIMIT_UPSTREAM=http://your-backend:8080 \
  -v "$PWD/configs/proxy.json:/etc/ratelimit/proxy.json:ro" \
  rate-limiter:local
```

To use a Redis server on that network, add:

```text
-e RATE_LIMIT_STORAGE=redis -e REDIS_URL=redis://your-redis:6379/0
```

`redis://` URLs support Redis authentication; `rediss://` enables TLS. Supply real
credentials through deployment secrets/environment, not committed config files.
Keep your backend private so clients cannot bypass the gateway.

## Configuration

The image starts `ratelimitd -config /etc/ratelimit/proxy.json`. JSON is versioned
and unknown fields fail startup. Config is loaded once; restart to apply changes. Policy revisions and tenant budgets
are described in [production operation](production.md).

| Field | Purpose |
| --- | --- |
| `version` | Required: `1` |
| `listen` | Address inside the container, normally `:8080` |
| `upstream` | One fixed HTTP(S) backend; optional base path is prepended |
| `storage.mode` | `ram` or `redis` |
| `storage.redis_url` | Redis connection URL; overridden by `REDIS_URL` |
| `storage.namespace` | Quota namespace shared by replicas, distinct across deployments |
| `storage.timeout` | Deadline for Redis startup/readiness/decision operations |
| `cleanup_interval` | Periodic RAM cleanup, e.g. `1s`; Redis uses expiry |
| `upstream_timeout` | Total admitted request timeout, e.g. `10s` |
| `anonymous_tenant` | Trusted constant tenant for IP policies |
| `trusted_proxies` | CIDRs allowed to supply `X-Forwarded-For`; empty trusts none |
| `forward_authorization` | Forward the incoming Authorization header; defaults to false |
| `routes` | Explicit pattern-to-policy mappings; unmatched routes return 404 |

Environment overrides: `RATE_LIMIT_LISTEN`, `RATE_LIMIT_UPSTREAM`,
`RATE_LIMIT_STORAGE`, `REDIS_URL`, and `RATE_LIMIT_NAMESPACE`.
`-addr` overrides the listen address when explicitly supplied alongside `-config`.
Other legacy rate flags apply to the demonstration server, not proxy JSON.

Each route specifies `pattern`, `policy`, `limit`, `window`, and `max_keys`.
Patterns use Go ServeMux syntax, such as `/`, `POST /orders`, or
`GET /items/{id}`. A catch-all `/` policy shares one allowance across all matched
paths for an identity; it does not create a new allowance for every raw URL.
`GET /items/{id}` groups all item IDs together when `per_route` is true. Multiple
patterns may use the same policy ID only with identical policy and rate settings.

Example route entry for authenticated users:

```json
{
  "pattern": "POST /orders",
  "policy": { "id": "orders-user", "identity": "user", "per_route": true },
  "limit": 10,
  "window": "1m",
  "max_keys": 10000
}
```

Identity is `ip`, `user`, or `api_key`. User/credential policies verify bearer
tokens using `RATE_LIMIT_CREDENTIALS`, with the same format as
`configs/credentials.example.json`. No default credentials are enabled. For
Compose, export that environment variable before `up`. Do not infer users from
unverified request headers. The static adapter is a minimal integration; an
OIDC integration and provisioned tenant budgets are described in
[production operation](production.md). Static credential changes require restart.

Forwarded identity headers are sanitized. Only verified identities populate
`X-RateLimit-Tenant-ID`, `X-RateLimit-User-ID`, and `X-RateLimit-API-Key-ID`.
Common incoming identity headers are removed. `X-Forwarded-For` is rebuilt from
the socket/trusted-proxy chain; trust only proxies you actually control. Backend
code should trust these headers only from the private gateway. Enable
`forward_authorization` explicitly if the backend needs the original credential.

## Storage guarantees and limits

| RAM | Redis |
| --- | --- |
| Separate counters per process | Shared counters for replicas using identical policies and namespace |
| Process clock with rollback protection | Redis server clock, Unix-epoch-aligned millisecond windows |
| At most `max_keys` entries per tenant/policy in proxy mode | At most `max_keys` counter fields per tenant/policy/revision |
| Constant-time expired map retirement | Whole policy hash expires at the window boundary |
| Restart clears quotas | Restarting the gateway preserves unexpired Redis state |

For unusual durations, RAM's Go `Time.Truncate` alignment and Redis's Unix-epoch
alignment may differ. Conventional windows such as 1s, 10s, or 1m align in both.
Redis windows must be at least 1ms and whole milliseconds; Redis limits/capacity
must fit positive signed 32-bit values. Metadata fields do not consume key capacity.
Counters, capacity checks, and expiry run atomically in Lua on a Redis primary.
There is no per-request scan in Redis, but a busy tenant/policy is a single hash/hot key.

Redis decisions fail closed: outage, timeout, read-only state, or command errors
return 503 without forwarding to the backend. There is no automatic RAM fallback
and automatic Redis command retries are disabled to avoid double consumption.
A timed-out operation may still have consumed quota; no automatic refund occurs.
The sample uses `noeviction`; Redis out-of-memory errors also fail closed.

Use identical policy settings across replicas. Persistent revision guards reject conflicting rate/window/capacity or identity
semantics at startup and during readiness. Use a new revision with a coordinated
cutover to intentionally reset quotas. Changing
identity semantics or namespace is a deliberate quota migration, not a live update.
Each request checks one policy; multiple-policy atomic enforcement is not included.
Both modes retain fixed-window boundary bursts and charge admitted attempts even
when the upstream later fails. Redis expiration depends on the Redis server clock;
it does not provide a durable rollback guarantee after expiration/restart.

## Health, errors, and scope

- `/healthz`: gateway liveness; not forwarded or rate limited.
- `/readyz`: gateway readiness, including Redis connectivity in Redis mode.
  It validates policy guards and expiring Redis probe writes without charging quota.
  It does not test upstream health or guarantee future command success.
- 429: allowance exhausted; 503: rate-limit capacity/backend unavailable.
- 502: upstream unreachable; 504: upstream timeout.
- Request method, path, query, body, upstream status, and ordinary response headers
  are forwarded. The upstream Host is selected from configuration.
- Initial scope is bounded HTTP requests to one upstream. Protocol upgrades return
  501; unbounded WebSocket/SSE sessions and multi-upstream load balancing are not
  part of this first release. Incomplete streamed responses cannot change their
  status to 504 after headers have already been sent.
- HTTP header/body read timeouts are bounded; shutdown waits up to five seconds.
- Redis standalone, Sentinel, and Cluster clients are supported. Protected metrics,
  deployment manifests, and alerts are in [production operation](production.md).
  AOF/replication do not provide exactly-once or zero-loss failover guarantees.

## Verification

```sh
go test -race ./...
go vet ./...
docker compose config --quiet
docker compose -f compose.yaml -f compose.redis.yaml config --quiet
```

Real-Redis tests are opt-in, and use isolated test namespaces. Point them only at
a development/test Redis instance:

```sh
REDIS_TEST_URL=redis://127.0.0.1:6379/0 go test -race \
  ./tests/algorithm/fixedwindow/redis ./tests/internal/proxyapp
```

Tests cover shared quotas/capacity across clients, expiry, mismatched settings,
replica proxy decisions, fail-closed behavior, forwarding, identity sanitation,
upstream errors, and timeouts.

Implementation references: [Go reverse proxy](https://pkg.go.dev/net/http/httputil),
[Redis atomic scripting](https://redis.io/docs/latest/develop/programmability/eval-intro/),
and [Compose dependency health checks](https://docs.docker.com/compose/how-tos/startup-order/).
