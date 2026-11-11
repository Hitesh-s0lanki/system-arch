# Rate-limiting algorithms

This project implements fixed-window counters, sliding-window logs, sliding-window counters, and token
buckets with RAM and Redis backends. Leaky bucket meters also support both backends; see the
[leaky bucket guide](leaky-bucket.md). Queue-based leaky buckets remain a possible
future implementation.
See [the sliding-window guide](sliding-window.md) for usage and tests.
Measured RAM/Redis latency for every implemented algorithm is saved in
`reports/rate-limiter-latency.json` and displayed on the frontend algorithms page.
See [latency benchmarks](latency-benchmarks.md) to reproduce or refresh the results.

| Algorithm | How it works | Main tradeoff |
| --- | --- | --- |
| Fixed-window counter | Counts requests in aligned windows and resets at each boundary. | Small state, but permits bursts across boundaries. |
| Sliding-window log | Stores accepted request timestamps and counts those within the last window. | Precise rolling-window enforcement, but stores one entry per accepted request still in the window. |
| [Sliding-window counter](sliding-window-counter.md) | Weights the previous window's count and adds the current window's count to estimate recent usage. | Small state and smoother boundaries, but approximate. |
| Token bucket | Refills tokens at a steady rate; each accepted request consumes a token. | Supports a sustained rate with a configurable burst allowance. |
| [Leaky bucket meter](leaky-bucket.md) | Accepted requests fill a bounded bucket; elapsed time drains it at a steady rate. | Immediate decisions with a configurable burst capacity; accepted requests are not paced. |
| Leaky bucket (queue) | Queues requests and releases them at a steady rate; rejects when the queue fills. | Smooths traffic, but introduces waiting time and requires a bounded queue. |
| GCRA (Generic Cell Rate Algorithm) | Tracks a theoretical arrival time and checks requests against a configured interval and burst tolerance. | Compact state, but more subtle time calculations. |

A concurrency limiter is another useful control: it caps requests currently in
progress rather than requests per time window. It can complement these algorithms.

## Where a sliding-window log lives

The word "log" means a collection of request timestamps used to make rate-limit
decisions. It does not mean application log files, console output, or an audit
history. The `algorithm/slidingwindow` and `algorithm/slidingwindow/redis` packages implement
this state.

For a policy such as 3 requests per rolling 10 seconds, one identity might have:

```text
tenant + policy + identity + optional route
    -> [12:00:01, 12:00:04, 12:00:08]
```

At 12:00:09, all three timestamps are still inside the last 10 seconds, so a new
request is rejected. At 12:00:11, the 12:00:01 entry expires and a new request can
be accepted. This example counts timestamps in (now - window, now] and records
only accepted requests; rejected attempts do not extend the log.

The storage backend determines where those timestamps reside:

| Backend | Timestamp storage | Scope and lifetime |
| --- | --- | --- |
| In-process RAM | A Go map from the scoped identity key to an ordered timestamp queue. | Local to one limiter instance; lost on restart and not shared across replicas. |
| Redis | A hash per scoped policy containing timestamp arrays per identity, plus a sorted set indexing identity expiry. | Shared across replicas using the same Redis state and key scope; persistence and recovery depend on Redis configuration. |

The Redis implementation keeps every accepted timestamp as a separate array
entry, so requests in the same millisecond do not overwrite one another. A Lua
script uses Redis time to prune, check capacity, count, decide, insert, and update
expiry atomically. Idle state expires after the last accepted timestamp leaves
the window; a small policy configuration guard persists until explicitly removed.

The RAM implementation protects pruning, counting, and insertion with a mutex,
and indexes identity expiry with a heap. Call `CleanupExpired` for idle cleanup;
capacity checks also reclaim expired identities. Both backends bound tracked
identities with `MaxKeys`; each identity needs at most the request limit in
active timestamp entries. Separate application request logs can record decisions
for debugging, but are not the state used to enforce the rate limit.


## Token bucket

`algorithm/tokenbucket` stores a token balance and last-refill timestamp per
identity. `algorithm/tokenbucket/redis` shares that state across replicas using
an atomic Lua script and Redis server time. Tokens are numeric allowance, not
objects or authentication credentials.

- `Capacity` sets the maximum balance and initial burst allowance.
- `Limit` tokens refill continuously during each `Window`; the sustained rate is
  `Limit / Window`. These settings are independent of burst capacity.
- Every request lazily refills `min(Capacity, balance + elapsed * rate)`, then
  consumes one token if available. Fractional tokens are retained.
- Empty buckets reject immediately. `RetryAfter` is the time needed to earn one
  token; requests are not queued. `Remaining` counts whole available tokens.
- RAM uses a mutex and a nondecreasing clock; Redis refills and consumes in one
  atomic operation. Backward clock movement does not grant tokens.
- `MaxKeys` bounds identities. Cleanup removes only buckets that would be full,
  so recreating a bucket cannot grant extra allowance. RAM cleanup scans keys
  when capacity is reached or when `CleanupExpired` is called. Redis indexes
  full-refill deadlines and expires idle state, retaining the policy signature.
- RAM uses nanosecond time arithmetic; Redis uses millisecond timestamps and
  rounds retry and expiry delays upward. Both retain fractional balances as
  integer remainders to avoid drift from repeated checks. Redis windows must be millisecond aligned, and
  `Window.Milliseconds() * (Limit + 1)` must not exceed `2^53`. Invalid
  configurations are rejected at construction.

For example, `Capacity: 10, Limit: 2, Window: time.Second` allows ten immediate
requests and then earns another allowance every 500 ms. A large burst can still
overload an upstream; size capacity for its available concurrency.

SDK selection is explicit; existing constructors retain fixed-window behavior:

```go
client, err := sdk.NewTokenBucketMemory([]sdk.Policy{{
    Policy: policy.Policy{ID: "api", Identity: identity.User},
    TokenBucket: tokenbucket.Config{
        Capacity: 10, Limit: 2, Window: time.Second, MaxKeys: 10000,
    },
}})
```

Use `sdk.NewTokenBucketRedis(redisClient, sdk.RedisConfig{...})` with the same
`Policy.TokenBucket` settings for shared quotas. As with the other Redis
backends, disable automatic retries, configure bounded connection timeouts,
enable `ContextTimeoutEnabled`, and call `Ready` before serving traffic. Shared
replicas must agree on rate, capacity, key budget, and identity semantics.

For a proxy route, set these fields alongside its existing pattern and policy:

```json
{
  "algorithm": "token_bucket",
  "capacity": 10,
  "limit": 2,
  "window": "1s",
  "max_keys": 10000
}
```

The proxy scopes quota and storage by tenant and policy, and returns HTTP 429
with `Retry-After` when a bucket is empty. SDK policies share their key budget
across tenants, while their quota keys remain isolated.


Run token-bucket algorithm, SDK, and real HTTP server integration tests with:

```sh
make test-token-bucket
```

This starts and removes a disposable Docker Redis unless `REDIS_TEST_URL` is
provided. See [the verification report](../reports/token-bucket-verification.md)
for tested behavior, commands, and deployment limits. The integer state format
uses the `token_bucket:v2` signature; any existing experimental floating-point
Redis state requires a coordinated policy revision change before startup.
