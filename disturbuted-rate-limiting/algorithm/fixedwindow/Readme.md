# Distributed Rate Limiter

A Go learning project that develops rate-limiting algorithms
incrementally, from in-memory implementations to distributed execution.

## Current milestone

Fixed-window enforcement with RAM and Redis integrations.

## Initial behavior

- Track limits independently for each key.
- Allow requests until the window's limit is reached.
- Reject additional requests during that window.
- Start a fresh count when the next window begins.
- Support concurrent callers safely.

Limit: 3 requests per key per 10-second window.
Windows align to multiples of 10 seconds on the clock.
Each request costs one unit. Rejected requests do not increase the count.

| Time  | Key    | Expected result           |
| ----- | ------ | ------------------------- |
| 00:01 | user-a | Allow                     |
| 00:02 | user-a | Allow                     |
| 00:03 | user-a | Allow                     |
| 00:04 | user-a | Reject                    |
| 00:04 | user-b | Allow—independent counter |
| 00:10 | user-a | Allow—new window          |

## Expired-key cleanup

Call `limiter.CleanupExpired()` periodically to remove keys whose windows have
ended. It returns the number of keys removed. A window expires exactly at its
end time; cleanup preserves counters in active windows. A removed key receives
a fresh allowance when it next makes a request.

The application schedules cleanup for idle periods; requests crossing a window
boundary also retire the entire expired map. All entries share a boundary, so
retirement takes constant time under the mutex instead of scanning identities.
Cleanup returns only entries retired by that call; a request may already have
retired them. Reclaiming detached memory is the Go garbage collector's job.
Requests still share one mutex per limiter; benchmark hot policies before launch.

## Key capacity

Every constructor call must provide a positive `MaxKeys`, for example:

```go
limiter, err := fixedwindow.New(fixedwindow.Config{
    Limit:   3,
    Window:  10 * time.Second,
    MaxKeys: 10000,
})
```

Existing keys keep their counters and continue working when capacity is full.
For a new key at capacity, the limiter first removes expired entries. If still
full, it rejects the request without storing the key or evicting active keys.
The lookup, cleanup, capacity check, and insertion share one mutex, so concurrent
requests cannot exceed `MaxKeys`.

Inspect `Decision.Reason` to distinguish outcomes:

| Outcome | Allowed | Reason | RetryAfter |
| --- | --- | --- | --- |
| Accepted | true | empty | zero |
| Key exhausted its limit | false | `ReasonRateLimited` | Time until the window ends, using internal time |
| New key cannot be stored | false | `ReasonCapacityExceeded` | Zero; no retry time promised |
| Key is invalid | false | `ReasonInvalidKey` | Zero |

Remaining is zero for capacity rejections; it does not mean the new key consumed
its allowance. Omitted or nonpositive `MaxKeys` makes construction fail.

Existing-key decisions are average O(1). New-window requests retire the expired
map before capacity checks. Capacity rejections do not scan or evict active keys.
New-key insertion is amortized O(1), and garbage collection may still affect tail
latency. Use separate limiter instances to isolate tenants and policies.

## Direct key validation

`Allow` rejects empty or whitespace-only keys and keys larger than `MaxKeyBytes`
(256 bytes). UTF-8 is measured in bytes, not characters. Invalid keys consume no
allowance or capacity. Other keys are kept exact, without trimming whitespace.
New keys receive an owned copy, reused on updates, so a short substring does not
retain an arbitrarily large caller string through the map. The service constructs
fixed-length namespaced keys after validating its identity components separately.

## Clock rollback behavior

The limiter remembers the latest wall-clock time observed by either `Allow` or
`CleanupExpired`. Earlier readings use that remembered time, so a backward clock
adjustment cannot reset counters or grant a second allowance for an older window.
This applies to all keys, including new keys created after cleanup.

During rollback, internal time stays frozen until the clock catches up. RetryAfter
is calculated from that internal time; it is a retry hint, not a guarantee that a
request will succeed after that real-time duration. A large forward jump followed
by a rollback can therefore delay new allowances until the clock catches up.
This protection belongs to a single limiter instance and does not survive restart.

## Benchmarks

From the module root, run:

```sh
go test -run '^$' -bench . -benchmem ./tests/algorithm/fixedwindow
```

The benchmarks cover allowed requests, rejected requests, parallel callers sharing
one key, lookups across 1,024 existing keys, and cleanup of 100, 1,000, and 10,000
expired keys, new-key insertions, and capacity rejections at those sizes.
They use a fixed clock. Setup is outside the timed sections, and
cleanup receives a freshly populated limiter on each iteration.

The existing-key benchmarks exclude key creation and map growth. The parallel
result reports aggregate time per operation across workers, not individual request
latency. Results depend on the machine and workload; run benchmarks without race
detection and use race detection separately to check correctness.

New-key insertion benchmarks include key copying and map growth; warmed existing
keys do not. Capacity benchmarks use a full active window and do not measure
worst-case garbage collection latency.

## Boundary bursts and deployment scope

With a limit of 3 per 10 seconds, 3 requests immediately before a boundary and 3
immediately after it can all succeed. Fixed windows do not guarantee at most 3
requests in every rolling 10-second interval. This is the algorithm's expected
tradeoff; a rolling-window or bucket algorithm has different behavior.

The basic in-memory milestone includes validation, concurrency and edge-case
tests, cleanup, rollback protection, and benchmarks. For HTTP routes and the
cleanup/shutdown lifecycle example, run `go run ./cmd/ratelimitd` from the module
root and see the root README. Direct users of the library must schedule cleanup.

Counters belong to one limiter instance and disappear on restart. Different
RAM processes do not share quotas. The `redis` subpackage now provides shared
atomic enforcement for the Docker proxy; see [the Docker guide](../../docs/docker.md)
for Redis-specific clock, expiration, capacity, and failure behavior. See [production operation](../../docs/production.md) for protected metrics,
policy revisions, HA clients, and deployment guidance.
