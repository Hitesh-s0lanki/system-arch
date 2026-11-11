# Leaky bucket verification and latency

Measured 2026-11-11 17:24 IST on Apple M1, 8 logical CPUs, macOS arm64,
Go go1.26.10, and Redis 7.4.12 in an isolated Docker container.

## Correctness

RAM and Redis passed the exercised cases, including race detection:

- Burst admission, remaining quota, full-bucket rejection, continuous partial
  drainage, exact retry boundaries, and upward rounding of fractional retry times.
- Clock rollback, same-time arrivals, long idle periods, and fresh identity state.
- 20,000 deterministic randomized RAM operations against an independent integer
  debt model across four rates, including nanosecond windows.
- 3,000 deterministic randomized RAM/Redis comparisons across three rates, with
  Redis retry durations compared to RAM rounded up to milliseconds.
- Invalid/oversized keys, cancellation, nil options/clocks, invalid configurations,
  bounded identity storage, preservation of partially drained buckets, idle
  cleanup, and concurrent admission, cleanup, and usage inspection.
- Two Redis clients sharing quota under 200 concurrent attempts; exactly the
  configured 25 requests admitted. Scope isolation across policies, tenants, and
  revisions; rate and capacity mismatches rejected.
- Redis idle TTL expiry, expiry tracking across buckets with different drain
  deadlines, readiness without charging quota, and safe NOSCRIPT recovery.
- Withheld Redis response reaches the configured deadline and returns an error.
  A dropped reply after execution is not replayed; exactly one unit is charged.
- SDK integration for RAM and Redis, tenant isolation, cleanup, and validation.
- Real HTTP proxy behavior for both backends: admission, HTTP 429 with Retry-After,
  recovery after drainage, prevention of rejected traffic reaching the upstream,
  capacity validation, and HTTP 503 on backend failure.

Evidence: [full race-test log](leaky-bucket-full-tests.txt),
[expanded edge-case log](leaky-bucket-edge-tests.txt), and
[standalone integration log](leaky-bucket-integration-tests.txt).
The standalone runner was verified with automatic temporary Redis creation and
cleanup. `CGO_ENABLED=0 go vet ./...` and `git diff --check` also passed.

This verifies the leaky bucket **meter**: decisions return immediately, and
accepted requests are not queued or paced.

## Latency method

The [repeatable harness](../tests/leakybucket/latency/main.go) records individual
wall-clock call durations with Go's monotonic timer, including lock/connection-pool
waiting. It runs without race instrumentation. One worker makes serial requests;
16 workers use a closed loop, sending the next request after the previous reply.
Results include local scheduler and GC effects, and are one local run rather
than production SLO guarantees. RAM timings include timer measurement overhead.

Each scenario warms 500 operations before sampling. Normal direct scenarios use
20,000 samples; full-capacity scenarios use 2,000; HTTP scenarios use 5,000.
Redis has a warmed pool of up to 32 connections with automatic retries disabled.
Direct Redis calls include the Docker loopback TCP round trip and Lua work.
The HTTP rows include a real TCP request to the proxy; allowed rows additionally
include a TCP request to a minimal local upstream returning HTTP 204. Per-request
logging is discarded during measurement. The proxy's periodic cleanup worker is
not started; constructor/readiness overhead, TLS, cold connections, failover, and
external network latency are excluded. No other verification command ran during
the latency sample collection.

Direct scenarios: `allowed-hot` uses one identity with ample quota;
`denied-hot` uses an exhausted identity; `allowed-1000-identities` rotates across
1,000 identities; `capacity-full-10000-identities` attempts to insert a new
identity while all 10,000 slots contain active buckets. Assertions enforce the
expected allowed/rejected counts and zero errors for every scenario.

## Measurements

All timings are microseconds (µs). Raw values and counts are in
[leaky-bucket-latency.json](leaky-bucket-latency.json).

| Backend | Scenario | Workers | p50 µs | p95 µs | p99 µs | Maximum µs | ops/s |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: |
| ram | allowed-hot | 1 | 0.125 | 0.125 | 0.291 | 0.458 | 5,751,397 |
| ram | allowed-hot | 16 | 0.708 | 73.042 | 178.167 | 498.958 | 1,398,675 |
| ram | denied-hot | 1 | 0.209 | 0.250 | 0.250 | 0.417 | 3,152,523 |
| ram | denied-hot | 16 | 0.458 | 51.250 | 123.041 | 351.334 | 2,004,083 |
| ram | allowed-1000-identities | 1 | 0.208 | 0.250 | 0.250 | 49.667 | 3,658,983 |
| ram | allowed-1000-identities | 16 | 0.375 | 50.125 | 108.833 | 296.708 | 2,108,056 |
| ram | capacity-full-10000-identities | 1 | 179.875 | 193.708 | 251.166 | 1336.458 | 5,444 |
| ram | capacity-full-10000-identities | 16 | 2993.792 | 3152.916 | 3452.166 | 4058.083 | 5,303 |
| redis | allowed-hot | 1 | 298.333 | 368.125 | 679.167 | 6271.875 | 3,210 |
| redis | allowed-hot | 16 | 845.334 | 1583.750 | 2144.250 | 4884.875 | 17,478 |
| redis | denied-hot | 1 | 306.458 | 387.042 | 785.041 | 4226.042 | 3,055 |
| redis | denied-hot | 16 | 897.375 | 1777.083 | 2406.333 | 8166.708 | 16,227 |
| redis | allowed-1000-identities | 1 | 294.583 | 365.708 | 663.334 | 5570.500 | 3,240 |
| redis | allowed-1000-identities | 16 | 848.542 | 1511.584 | 2108.708 | 5505.167 | 17,575 |
| redis | capacity-full-10000-identities | 1 | 283.958 | 338.375 | 588.625 | 4443.167 | 3,382 |
| redis | capacity-full-10000-identities | 16 | 814.959 | 1425.208 | 1678.125 | 1860.666 | 18,362 |
| ram | http-allowed | 1 | 85.375 | 159.250 | 223.542 | 352.292 | 10,852 |
| ram | http-allowed | 16 | 471.458 | 973.042 | 1151.458 | 2284.000 | 30,537 |
| ram | http-denied | 1 | 44.500 | 60.625 | 115.583 | 278.625 | 21,468 |
| ram | http-denied | 16 | 161.750 | 590.583 | 807.750 | 1489.958 | 77,000 |
| redis | http-allowed | 1 | 426.167 | 808.542 | 1137.584 | 3156.375 | 2,143 |
| redis | http-allowed | 16 | 1375.417 | 2353.959 | 3248.750 | 6289.042 | 10,730 |
| redis | http-denied | 1 | 370.167 | 441.166 | 667.250 | 2948.792 | 2,633 |
| redis | http-denied | 16 | 1090.250 | 2136.458 | 3759.833 | 31536.500 | 12,113 |

Every measured scenario completed with zero backend/HTTP errors and the expected
admission/rejection counts. These throughput values describe this particular
closed-loop workload and worker count, not the server's maximum capacity.

## Observed latency limits

Normal serial hot-key medians were 0.125µs in RAM and 298.333µs in Redis.
At 16 workers, their p99 latencies were 178.167µs and 2,144.250µs respectively.
The RAM limiter uses a single mutex across identities, so concurrency introduces
contention even when identities differ.

A new identity at the RAM capacity bound triggers an O(MaxKeys) scan under that
mutex on every attempt. With 10,000 active identities this measured 179.875µs
median / 251.166µs p99 serially, and 2,993.792µs median / 3,452.166µs p99 at
16 workers. This is the main observed scaling concern. An expiry heap/index
would avoid rescanning all active identities on repeated capacity rejections.
No algorithm optimization was made as part of this verification.

Redis capacity rejection uses its expiry index and did not show the same full-map
scan cost. Its Lua cleanup still processes all identities whose expiry is due;
a synchronized mass expiry can cause a longer atomic script and was not included
in the sampled latency scenarios. Redis HTTP rejection also had a 31.537ms
maximum outlier at 16 workers, despite a 3.760ms p99; the single-run data does not
establish the cause of that outlier.

## Reproduce

```sh
# From disturbuted-rate-limiting; runner owns and removes a temporary Docker Redis.
make test-leaky-bucket
LEAKY_LATENCY=1 make test-leaky-bucket

# This machine's default SDK cannot link Go race binaries. Its installed
# compatible SDK works without changing global system configuration:
SDKROOT=/Library/Developer/CommandLineTools/SDKs/MacOSX15.4.sdk make test-leaky-bucket

# Existing disposable Redis, direct latency measurement without race instrumentation:
REDIS_TEST_URL=redis://127.0.0.1:6379/0 CGO_ENABLED=0 \
  go run ./tests/leakybucket/latency -samples 20000 -output reports/leaky-bucket-latency.json
```

Only point these checks at disposable Redis: the NOSCRIPT test uses SCRIPT FLUSH.
The latency harness cleans only its unique namespace.

## Coverage limits

The full suite skips existing process-based Redis OOM, Sentinel, Cluster, and one
fixed-window SDK test because REDIS_SERVER_BIN is not installed. All leaky bucket
RAM, Redis, SDK, and proxy cases ran without skips. This run does not verify
Cluster/Sentinel failover, persistence across Redis crashes, remote-network
latency, TLS, maximum memory/cardinality, or multi-hour soak behavior. Deterministic
clock tests inject a future persisted high-water mark into test Redis state;
they do not alter the host or Redis server clock.
