# Leaky bucket

The `algorithm/leakybucket` package implements a leaky bucket **meter**. Each
identity starts with an empty bucket. Accepted requests add one unit of load;
elapsed time drains `Limit` units per `Window`, down to zero. A request is
rejected if adding it would exceed `Capacity`. Rejections add no load.

This variant returns immediate decisions through the existing limiter interface.
It permits a burst of `Capacity` requests and does not queue or delay requests.
A leaky bucket queue would instead hold requests and dispatch them at a steady
rate; that requires a separate scheduling API and is not implemented here.

For example, with `Limit: 2`, `Window: time.Second`, and `Capacity: 3`, three
simultaneous requests are accepted. A fourth is rejected with a 500ms retry hint.
After 250ms, only half a unit has drained, so a request still cannot fit. After
500ms, one request fits. `Remaining` reports whole requests that still fit after
admission. `RetryAfter` rounds up to a nanosecond in RAM or a millisecond in Redis.
Concurrent requests can consume the space before a retry arrives.

## Go usage

```go
limiter, err := leakybucket.New(leakybucket.Config{
    Limit: 2, Window: time.Second, Capacity: 3, MaxKeys: 1000,
})
if err != nil {
    return err
}
decision, err := limiter.Evaluate(ctx, "tenant:user")
```

Import `github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/algorithm/leakybucket`.
For policy and identity handling, set `sdk.Policy.LeakyBucket` and call
`sdk.NewLeakyBucketMemory` or `sdk.NewLeakyBucketRedis`. Memory constructors accept
`leakybucket.WithClock` for deterministic clocks. Existing fixed-window and
sliding-window SDK constructors keep their own configuration fields.

## Proxy configuration

Add these fields to a route alongside its existing pattern and policy:

```json
{
  "algorithm": "leaky_bucket",
  "limit": 2,
  "window": "1s",
  "capacity": 3,
  "max_keys": 1000
}
```

`limit`, `window`, `capacity`, and `max_keys` must be positive. Capacity is required
for this algorithm. Duplicate routes sharing a policy must agree on the rate,
capacity, algorithm, revision, and identity settings. The existing middleware
returns HTTP 429 and `Retry-After` for a full bucket.

## Storage and lifecycle

RAM uses one bucket per identity, protected by a mutex. `MaxKeys` bounds retained
identities. Call `CleanupExpired` periodically to remove fully drained buckets;
new identities also trigger cleanup at the capacity bound. Partially drained
buckets must remain to preserve quota. No background workers are created.

Redis uses one atomic Lua operation to drain and admit, using Redis server time.
Replicas share quota within the same namespace, policy, revision, and tenant
scope. Bucket state and an expiry index share a cluster hash slot with an `lb`
key prefix, separate from other algorithms. Idle state expires once all buckets
drain; a policy configuration guard persists to reject incompatible replicas.
`Ready` checks compatibility without consuming allowance, and `Usage` reports
active identity capacity. Redis clients must disable automatic retries and use
bounded timeouts, as with the existing Redis backends. Windows must be aligned
to whole milliseconds. Backend failures propagate without a local fallback.

Both backends clamp backward clock movement to the last observed time, so clock
rollback does not grant extra space. Retry hints use that clamped clock.

## Verification

```sh
CGO_ENABLED=0 go test ./...
REDIS_TEST_URL=redis://127.0.0.1:6379/0 go test ./tests/algorithm/leakybucket/... ./tests/sdk ./tests/internal/proxyapp
```

Redis integration tests require a disposable Redis instance and use unique test
namespaces. Coverage includes drain boundaries, partial drainage, retry rounding,
clock rollback, concurrent admissions across replicas, capacity reclamation,
policy mismatch, readiness, cancellation, SDK integration, and proxy responses.

For an isolated end-to-end check, run `make test-leaky-bucket`; it creates and
removes a temporary Docker Redis. Set `LEAKY_LATENCY=1` to also measure direct
and HTTP latency without race instrumentation and write percentile results to
`reports/leaky-bucket-latency.json`. These tests require disposable Redis because
the NOSCRIPT recovery check flushes its script cache.

See the [verification and latency report](../reports/leaky-bucket-verification.md)
for measured percentiles, coverage, and the RAM capacity-scan bottleneck.
