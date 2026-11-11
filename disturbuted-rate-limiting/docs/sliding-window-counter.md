# Sliding-window counter

`algorithm/slidingcounter` and `algorithm/slidingcounter/redis` implement an
approximate rolling-window limiter with two counters per identity. The existing
`algorithm/slidingwindow` packages remain the exact timestamp-log implementation.

For an aligned window of duration W, elapsed time E, previous count P, and current
count C, the estimated usage is:

```text
usage = C + P * (W - E) / W
allow when usage + 1 <= Limit
```

For example, with 100 requests per minute, 80 requests in the previous minute,
and 30 in the current minute, 15 seconds into the minute the estimate is
`30 + 80 * 45/60 = 90`. Another request is accepted. At the next boundary the
current count becomes the previous count; after two boundaries old counts vanish.
Rejected requests do not increment either counter or prolong identity expiry.

This assumes the previous window's requests were evenly distributed. It can
underestimate or overestimate actual rolling usage, so use the timestamp log
when an exact rolling cap is required.

## SDK

Import `algorithm/slidingcounter`, `sdk`, `policy`, and `identity` from this module:

```go
policies := []sdk.Policy{{
    Policy: policy.Policy{ID: "api", Identity: identity.User},
    SlidingCounter: slidingcounter.Config{
        Limit: 100, Window: time.Minute, MaxKeys: 10000,
    },
}}
client, err := sdk.NewSlidingCounterMemory(policies)
```

Use `sdk.NewSlidingCounterRedis(redisClient, sdk.RedisConfig{Policies: policies,
Namespace: "myapp", Revision: "counter-v1", Timeout: time.Second})` for shared
quota. Disable Redis retries (`MaxRetries: -1`), enable `ContextTimeoutEnabled`,
configure bounded connection timeouts, and call `client.Ready(ctx)` at startup.
The caller owns the Redis client. Check requests through the existing
`client.Check(ctx, policyID, service.Request{...})` interface or HTTP middleware.

## Proxy

Set these fields alongside the route's existing pattern, revision, and policy:

```json
{
  "algorithm": "sliding_window_counter",
  "limit": 100,
  "window": "1m",
  "max_keys": 10000
}
```

The configured storage mode selects RAM or Redis. The proxy returns HTTP 429
with `Retry-After` on exhaustion; Redis failures propagate as HTTP 503. Tenant
and policy scoping follows the other proxy algorithms. Counter Redis keys use a
separate `:sc:` scope, so changing algorithms creates fresh quota.

## Precision, state, and lifecycle

Both backends use epoch-aligned windows and millisecond timestamps. Windows must
be positive multiples of one millisecond and at most half of `time.Duration`'s
maximum. Limit and MaxKeys must be positive and at most 2^31-1; the product of
Limit and window milliseconds must be at most 2^52. This keeps scaled integer
arithmetic exact in both Go and Redis Lua, without floating-point threshold
rounding. `Remaining` rounds estimated available requests down to a whole number.
`RetryAfter` rounds up to milliseconds and describes the earliest admission if
no other request consumes quota; it may extend beyond the next window boundary.

RAM decisions use a mutex and a nondecreasing clock. There are no background
workers. Call `CleanupExpired` periodically to reclaim idle identities; capacity
checks and `Usage` also clean expired state. Cleanup scans at most MaxKeys entries.
Each identity retains two counts and a window start, rather than request timestamps.

Redis uses server time and one atomic Lua operation for rotation, admission,
capacity checks, and expiry. A hash stores counters and a sorted set indexes
identity expiry. All policy keys share a Redis Cluster hash slot. Identities
expire when the last accepted request's aligned window and its successor end;
idle data keys also receive TTLs. The policy configuration guard persists to
reject incompatible replicas. Shared replicas must agree on limit, window, key
budget, revision, and identity semantics. Clock rollback is clamped while state
exists. Redis failover and persistence guarantees follow the configured backend.

## Tests

```sh
make test-sliding-counter
```

This starts and removes a temporary Docker Redis, or uses a caller-supplied
`REDIS_TEST_URL` pointing to an isolated test instance. It runs race detection,
weighted boundary and retry checks, a randomized timestamp-based reference,
concurrent admission, real Redis/RAM parity, SDK checks, and HTTP proxy checks.
On macOS with an incompatible default SDK, select a compatible installed SDK:

```sh
SDKROOT=/Library/Developer/CommandLineTools/SDKs/MacOSX15.4.sdk make test-sliding-counter
```
