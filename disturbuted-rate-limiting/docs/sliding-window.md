# Sliding-window log

The limiter counts accepted requests in `(now - window, now]` for each scoped
identity. A request exactly one window old is expired. Rejections consume no
allowance and do not extend timestamp retention. `RetryAfter` is the time until
the oldest active accepted request expires. Capacity rejections return no retry
promise, matching the existing fixed-window contract.

With a limit of 2 per 10 seconds, requests at seconds 1 and 9 succeed. A request
at second 10 is rejected even though a fixed-window boundary was crossed. At
second 11, only the timestamp at second 1 expires, allowing one new request.

## Reverse proxy: RAM or Redis

Add `"algorithm": "sliding_window_log"` to a route in `configs/proxy.json`:

```json
{
  "pattern": "/",
  "algorithm": "sliding_window_log",
  "revision": "sliding-v1",
  "policy": {"id": "default-ip", "identity": "ip", "per_route": false},
  "limit": 3,
  "window": "10s",
  "max_keys": 10000
}
```

Omitting `algorithm` selects `fixed_window`. Routes sharing a policy ID must
agree on the algorithm, identity grouping, rate, capacity, and revision.

From the rate-limiter module directory, start RAM mode:

```sh
docker compose up -d --build --wait
```

For Redis mode, stop the RAM deployment and start the Redis overlay:

```sh
docker compose down
docker compose -f compose.yaml -f compose.redis.yaml up -d --build --wait
```

Send four requests within 10 seconds:

```sh
for request in 1 2 3 4; do
  curl -i http://127.0.0.1:8088/orders
done
```

The first three reach the backend; the fourth returns 429 with `Retry-After`.
Wait 10 seconds after the last accepted request and try again to see recovery.
This assumes a fresh policy scope and no other traffic consuming its allowance.
Redis replicas share quota; separate RAM processes do not. Redis failures return
503 without a RAM fallback. Change algorithms with a coordinated deployment:
fixed-window and sliding-window state use different scopes, so switching creates
fresh allowance. Do not mix algorithms across replicas for a strict shared quota.

## Embedded SDK

Import `algorithm/slidingwindow`, `sdk`, `identity`, `policy`, and `service` from
this module, plus `context` and `time` from the standard library.

```go
policies := []sdk.Policy{{
    Policy: policy.Policy{ID: "orders", Identity: identity.User},
    SlidingWindow: slidingwindow.Config{
        Limit: 100, Window: time.Minute, MaxKeys: 10000,
    },
}}
client, err := sdk.NewSlidingWindowMemory(policies)
if err != nil {
    return err
}
decision, err := client.Check(context.Background(), "orders", service.Request{
    TenantID: "verified-tenant", UserID: "verified-user",
})
// Handle err and decision before performing the protected operation.
```

For Redis, replace construction with:

```go
client, err := sdk.NewSlidingWindowRedis(redisClient, sdk.RedisConfig{
    Policies: policies, Namespace: "myapp", Revision: "sliding-v1",
    Timeout: 500 * time.Millisecond,
})
if err != nil {
    return err
}
if err := client.Ready(ctx); err != nil {
    return err
}
```

The caller owns `redisClient`. Disable automatic retries with `MaxRetries: -1`,
enable `ContextTimeoutEnabled`, and configure connection timeouts. Errors may
follow an executed quota mutation, so do not retry ambiguous failures. RAM users
can call `client.CleanupExpired()` periodically; no goroutine starts implicitly.
Direct package users can use `slidingwindow.New` or `slidingwindow/redis.New`
through the existing service limiter interface.

## Storage and resource bounds

RAM stores a map of identity keys to ordered timestamp slices. One heap entry per
identity tracks when its final timestamp expires, avoiding a full identity scan
on capacity rejection. A mutex protects quota and capacity together. Cleanup
removes fully expired identities; request evaluation prunes partial expiry.
Updates cost up to O(Limit) for timestamp compaction plus O(log MaxKeys) for the
expiry heap; reclaiming many expired identities adds work to that operation.

Redis uses three keys per policy/revision/tenant scope, sharing a Cluster hash
slot under `<namespace>:sl:{<scope-hash>}`:

| Key | Contents |
| --- | --- |
| Base key | Hash with `_last` clock high-water mark and `k:<identity>` fields containing JSON arrays of accepted millisecond timestamps |
| Base key + `:expiry` | Sorted set of identity fields scored by last accepted timestamp plus window |
| Base key + `:config` | Persistent policy compatibility signature |

A short-lived `:probe` key is also used for readiness. The decision Lua script
uses Redis server time, retires expired identities, prunes the target log, checks
capacity and allowance, appends an accepted timestamp, and updates expiry in one
atomic execution. Timestamp arrays preserve multiple requests in one millisecond.
The expiry index releases idle identities before capacity checks. Usage metrics
also retire expired identities without consuming allowance. The hash and index
expire when idle; the configuration signature persists to reject mismatched
replicas even after quota expiration. Retired revision guards need operational
cleanup when no replicas use those scopes.

Each identity retains at most `Limit` timestamp entries, including expired entries
awaiting pruning. Storage is O(MaxKeys × Limit), plus key/index overhead. Redis
rewrites the target timestamp array per accepted request; a decision costs up to
O(Limit) plus expiry-index operations. Retiring many expired identities can block
Redis while the script runs. Benchmark large limits and identity budgets for your
workload; this log algorithm uses more memory than counters.

RAM supports nanosecond durations; Redis requires whole millisecond windows and
uses millisecond timestamps. Both clamp clock rollback while their high-water
state exists. RAM rollback protection lasts for that limiter instance; Redis
protection ends when state expires or is lost. Restart, failover, or eviction can
lose quota state. Use the same no-eviction and persistence considerations described
in [production operation](production.md).

## Automated testing

From the module directory, run:

```sh
make test-sliding-window
```

This requires Go with race support and Docker. It starts a temporary Redis on a
random loopback port, runs RAM, real Redis, SDK, and HTTP proxy tests with race
detection, and removes the container on exit. To use an existing dedicated test
Redis instead:

```sh
REDIS_TEST_URL=redis://127.0.0.1:6379/0 make test-sliding-window
```

Tests clean up their own namespaces. Redis cases fail on connection errors and
cannot silently skip when run through this target. Plain `go test ./...` skips
Redis cases when no test Redis configuration is provided. Existing process-based
failure tests additionally require `REDIS_SERVER_BIN` as described in the main
README; the new sliding-window tests do not require that binary.

Coverage includes rolling rather than aligned expiry, partial and exact-boundary
expiry, rollback, a randomized RAM reference comparison, concurrent allowance and
capacity enforcement, shared replica state, invalid keys, cancellation, policy
compatibility, idle TTL expiry, SDK wiring, HTTP 429/retry headers, and Redis
failure returning 503 before reaching the upstream.

On this development machine, the default macOS SDK caused an external linker
`unknown architecture` error. The installed compatible SDK lets race tests run:

```sh
SDKROOT=/Library/Developer/CommandLineTools/SDKs/MacOSX15.4.sdk make test-sliding-window
```

This is a local toolchain workaround and does not change application behavior.

The [test review and verification report](../reports/sliding-window-verification.md)
records coverage, repeated race runs, and deliberate bug checks. To repeat the
algorithm and SDK/proxy tests with shuffled order:

```sh
SLIDING_TEST_COUNT=20 make test-sliding-window
```
