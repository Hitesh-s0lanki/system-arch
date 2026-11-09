# Production operation

The proxy enforces one fixed-window policy per matched route and charges admitted
attempts, including upstream failures. It fails closed with 503 when Redis cannot
make a decision. Fixed windows allow up to two windows' allowances close to a
boundary. These are explicit API contracts; use a different algorithm if rolling
interval fairness is required. Multiple-policy transactions, refunds, WebSockets,
and unlimited response streams are not implemented.

## Bounded admission

`max_body_bytes` defaults to 1 MiB; `max_concurrent` defaults to 128. The gateway
reserves a concurrency slot before reading a body, buffers at most the configured
body limit, and rejects overload immediately with 503 `gateway_overloaded`.
Oversized bodies, including bodies without Content-Length, return 413 before
quota consumption or upstream forwarding. Malformed bodies return 400. Header
limits and read/write deadlines are also enforced. Upstream connection limits
and Redis connection pools are bounded.

The product of the body and concurrency limits cannot exceed 256 MiB. This is a
configured payload budget, not a process RSS guarantee: buffer growth, responses,
Redis clients, and Go runtime memory also need headroom. Tune container memory
and `GOMEMLIMIT` using actual load measurements. No request bodies or tokens are
written to request logs. Logs contain configured route, status, and duration.

## Tenant budgets

Each `(namespace, policy, revision, tenant)` owns independent identity capacity.
`max_keys` is per tenant per policy, not a deployment-wide total. Tenant IDs must
be provisioned in `tenants`. If omitted for static credentials, the proxy derives
a bounded set from those credentials and `anonymous_tenant`. OIDC requires an
explicit list. An authenticated but unprovisioned tenant receives 403; it cannot
allocate limiter state. Configure at most 256 tenants and 256 routes. Different
tenants have separate RAM locks and Redis hash slots. One hot tenant/policy still
uses one Redis hash; gateway replicas cannot remove that hotspot.

Total counter capacity is the sum of each policy's max_keys times the number of
provisioned tenants. Estimate this before increasing tenant counts. New tenant
provisioning and static credential changes currently require a coordinated
configuration deployment; they are not an unbounded self-service API.

## Policy revisions and rollout

Routes have `revision`, defaulting to `v1`. All patterns sharing a policy ID must
have identical revision, identity semantics, limit, window, and capacity. Redis
stores a persistent configuration guard per policy revision and tenant. Startup
and readiness validate that guard, even before any traffic arrives. Reusing a
revision with different settings fails startup instead of accepting a replica
that later returns errors. Readiness also performs expiring probe writes and
checks metadata without charging a user allowance. It detects read-only Redis,
ACL failures for probe commands, and OOM; it is still a point-in-time check.

Counter hashes expire at their window boundary. Configuration guards intentionally
do not expire: they prevent an old replica from silently reintroducing obsolete
settings after counters expire. Probe hashes expire after ten seconds. Retiring
old guards is a deliberate operator task after every replica using that revision
is gone. Use SCAN with the deployment namespace on the primary or each Cluster
primary; verify exact keys and delete retired revision keys only. Never FLUSHDB
or wildcard-delete a shared Redis deployment.

A new revision starts fresh allowance. Mixed revisions therefore have independent
quotas and can jointly grant extra requests. For strict enforcement, drain the
old deployment and wait for in-flight requests before directing traffic to the
new revision. Use a maintenance cutover or separately named blue/green Services;
ordinary rolling deployment is safe only when policy settings and revision remain
unchanged. Rolling back to a previous revision reuses its unexpired counters.
Changing the namespace also resets quotas. This release changes proxy Redis key
scoping from the earlier prototype: upgrading from that prototype requires the
same coordinated cutover and starts fresh quota state.

## Redis topology and transport

`storage.topology` is `standalone` (default), `sentinel`, or `cluster`.
`REDIS_URL` supplies data-node username/password, database, and TLS settings.
Use `rediss://` in production when Redis is outside a trusted local network.
TLS certificate verification remains enabled. For managed endpoints, use a
standalone topology when the provider exposes one stable primary endpoint.

Sentinel example:

```json
{"mode":"redis","namespace":"production","timeout":"500ms","pool_size":32,
 "redis_url":"rediss://redis-primary.example:6379/0",
 "topology":"sentinel","master_name":"limiter",
 "addresses":["sentinel-a.example:26379","sentinel-b.example:26379","sentinel-c.example:26379"]}
```

Provide `REDIS_SENTINEL_USERNAME` and `REDIS_SENTINEL_PASSWORD` through secrets if
Sentinel authentication differs from data-node authentication. The URL's TLS
configuration applies to Sentinel as well as the data nodes.

For Cluster, use `"topology":"cluster"` with seed `addresses`; only database zero
is supported. Script keys share one hash slot. `pool_size` defaults to 32 and
bounds active connections **per node**, not for the entire Cluster. Cluster
routing can follow MOVED/ASK responses where the script was not executed.
Quota-changing commands disable both normal command retries and Cluster retries
on ambiguous I/O errors. NOSCRIPT falls back to EVAL because no script executed.
Timeouts may still consume allowance; clients must not assume a refund.

Use noeviction and monitor memory. Required application commands include PING,
TIME, GET, SET, HGET, HSET, HLEN, DEL, PEXPIREAT, PEXPIRE, EVALSHA, and EVAL, scoped
to the namespace. Redis clients also need their connection-handshake commands;
Sentinel/Cluster require their topology commands. Readiness exercises probe
writes but does not prove every future command will succeed. Validate the exact
ACL against integration tests before deploying it.

Sentinel and Cluster improve availability; asynchronous replication can lose
acknowledged counters during failure. AOF is not zero-loss durability. Define
acceptable over-admission after failover with your service owner. Requirements
for strict durable quotas need a stronger storage/consistency design. The tests
verify quota preservation after confirmed replication, not zero-loss failover.

## Identity and credentials

The static bearer adapter now performs a lookup by SHA-256 digest rather than a
linear scan of all tokens. Use random high-entropy credentials; possession grants
access. Replace credentials through deployment secrets and restart all replicas
when rotating or revoking static credentials.

For federated login, configure `oidc` instead of static credentials:

```json
{"issuer":"https://identity.example","audience":"your-client-id",
 "tenant_claim":"tenant_id","user_claim":"sub"}
```

This adapter verifies OIDC ID tokens, their signature, issuer, audience, and expiry
using discovered JWKS, with signing-key rotation. All discovery/key requests and
redirects require HTTPS and have bounded timeouts. Map an optional `api_key_claim`
only when that claim identifies a verified credential. Tenant/user claims must
be strings and are validated against identity length limits. Set `tenants` to
the provisioned tenant list. A provider's access tokens are not automatically ID
tokens: integrate a resource-server verifier through `Authenticator` if that is
your token contract. Use short token lifetimes; immediate revocation/introspection
is not supplied by this adapter. Invalid/unverifiable tokens fail closed.

## Metrics and alerts

Set a random `RATE_LIMIT_METRICS_TOKEN` secret to enable `/metrics`. Without it,
the route returns 404. Scrapes must send `Authorization: Bearer <secret>` over
HTTPS or a controlled private network. Metrics never label raw URLs, IPs, users,
tenants, or credentials. Labels use configured policies/routes and bounded
outcomes/statuses.

Metrics include decision counts and latency histograms, HTTP status counts,
upstream failures, in-flight requests, Go runtime statistics, and maximum tenant
capacity utilization per policy. Capacity collection has a storage timeout;
failed samples are omitted rather than falsely reporting zero usage.

`deploy/monitoring/` contains Prometheus scrape configuration and alert rules.
Adapt target discovery, secret paths, and thresholds to your deployment. Inspect
429 volume separately from backend errors: rejecting excess traffic is normal
limiter operation. A readiness failure must remove a gateway from service; a
Redis outage must not trigger repeated liveness restarts.

## Deploy

`deploy/kubernetes/ratelimiter.yaml` provides two gateway replicas, probes, a
ClusterIP Service, bounded resources, non-root/read-only security, disruption
budget, CPU autoscaling, and restricted ingress. Before applying:

1. Build and push an immutable image; replace `rate-limiter:release` with its digest.
2. Set the real upstream and approved proxy CIDRs. The sample empty proxy list
   identifies the direct peer; behind ingress it must be configured to identify
   actual clients correctly. Never trust every CIDR.
3. Provision Redis HA/TLS/ACLs and create Secret `rate-limiter` with `redis-url`
   and `metrics-token`. No production credentials are checked in.
4. Configure a TLS ingress/load balancer in your environment and make the backend
   reachable only from authorized callers, including the gateway. The supplied
   NetworkPolicy restricts gateway ingress; add backend ingress and environment-
   specific egress policies separately.
5. Confirm namespace labels match the supplied ingress/monitoring NetworkPolicy.
6. Run load/failure tests on staging, then choose resource budgets and alerts.

A ConfigMap change does not automatically restart pods. Deploy unchanged-policy
updates with `kubectl rollout restart deployment/rate-limiter` and monitor
`kubectl rollout status`. Use the coordinated cutover above for policy revisions.
Never apply the development Compose secrets or test namespaces to production.

## Failure response

- **503 / not ready:** check Redis reachability, writable primary, TLS/ACLs, OOM,
  and policy guard mismatch. Inspect decision-error metrics and logs. Restore the
  dependency; do not silently switch to RAM or flush counters.
- **Capacity exceeded:** identify which policy is at capacity. Investigate traffic
  abuse and tenant sizing; raising max_keys requires a new revision/cutover.
- **Gateway overloaded:** check in-flight requests and upstream latency. Scale
  only after verifying upstream/Redis capacity; raising concurrency alone can
  worsen overload.
- **502/504:** inspect upstream availability and deadlines. Already-admitted
  requests remain charged; retry non-idempotent business requests only under the
  upstream's own idempotency contract.
- **Bad release:** roll back the image/config together. Unchanged revisions retain
  Redis state. Different revisions need a coordinated traffic cutover.

## Verification and capacity acceptance

```sh
make verify
REDIS_TEST_URL=redis://127.0.0.1:6379/0 REDIS_SERVER_BIN=/path/to/redis-server make integration
```

The integration target requires both variables and runs real shared-quota tests,
OOM/read-only/recovery tests, policy migration, Sentinel promotion, and a
three-primary Cluster fixture. Failure fixtures create and stop only their own
loopback Redis processes with temporary directories. The shared Redis URL must
point to an isolated test instance. CI also validates Compose, builds the image,
and runs a two-gateway k6 smoke test.

For a reproducible local load stack (Compose 2.24.4+ and k6 required):

```sh
docker compose -f compose.yaml -f compose.redis.yaml -f tests/load/compose.yaml up -d --build --wait
TARGETS=http://127.0.0.1:8088,http://127.0.0.1:8089 RATE=100 DURATION=2m k6 run tests/load/sustained.js
docker compose -f compose.yaml -f compose.redis.yaml -f tests/load/compose.yaml down -v
```

The load configuration intentionally uses a very high limit to measure admitted
traffic. Tune RATE, DURATION, VUS/MAX_VUS, P95_MS, and P99_MS. Test rejection traffic
separately with a low-limit policy and EXPECT_STATUS=429 after exhausting it.
Use a distinct namespace/revision for each changed test policy. Exercise both
hot identities and many credentials, boundary bursts, several gateway replicas,
and dependency failures. Archive k6 output, memory/CPU/Redis metrics, topology,
versions, and the exact workload. Default smoke thresholds are provisional local
checks, not a production SLO. Select launch capacity only after a sustained run
on the actual deployment hardware and network.
