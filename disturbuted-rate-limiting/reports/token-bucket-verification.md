# Token-bucket server verification

Verified on 2026-11-11 using Go 1.26.10 on macOS/Apple M1 and disposable Redis
7.4.12 containers. The standalone Redis deployment and multiple proxy replicas
passed the checks below. This is evidence for these configurations, not a claim
that every failure mode or production workload has been tested.

## Bug found and fixed

With one token per second, checking an empty bucket every 100 ms accumulated
floating-point error. At exactly one second the original implementation rejected
with a further 1 ns delay. A regression test reproduced this before the fix.

The RAM implementation now retains whole tokens and an integer fractional
numerator, with 128-bit multiplication/division for overflow-safe refill math.
Redis uses whole tokens and an integer fractional numerator as well. Construction
rejects Redis configurations whose fractional-credit intermediate exceeds the
exact integer range of Lua numbers. Retry and expiry calculations round upward.

Redis state now uses a `token_bucket:v2` policy signature. Replicas with older
experimental token-bucket state will fail compatibility checks; migrate with a
coordinated policy revision change rather than mixing implementations.

## Passing checks

| Area | Evidence |
| --- | --- |
| Burst and rate | Initial capacity, exhaustion, partial refill, exact refill boundary, no accumulation beyond capacity, positive rounded retry hints |
| Reference correctness | 30,000 deterministic randomized RAM checks against an independent integer-credit reference, including backward clock movement and cleanup |
| Redis parity | 2,500 repeated/fractional refill checks against RAM, plus multiple identities, time rollback, capacity and usage checks |
| Concurrency | 200 concurrent algorithm checks enforce a single burst; simultaneous RAM admission, cleanup and usage run under the race detector |
| State lifetime | Only fully replenished buckets are removed; the latest identity deadline controls shared Redis hash expiry |
| Validation | Invalid limits, burst, key budgets, clocks, key lengths, cancellation and oversized full-refill durations |
| Replica compatibility | Conflicting burst capacity, rate/window, key budget and identity semantics fail compatibility checks; separate revisions initialize independently |
| Redis lost reply | A real script response is dropped after execution; the request returns an error, consumes exactly once and is not replayed |
| SDK | RAM/Redis construction, readiness without quota consumption, tenant isolation, cleanup and invalid policies |
| Real HTTP servers | 100 concurrent requests across two proxy instances admit exactly 7 and reject 93; tenant and route quotas are isolated; storage exhaustion returns 503; forged identity headers do not bypass authentication |
| Network failure | A TCP bridge drops Redis commands after startup; requests and readiness return 503 within a one-second bound with a configured 100 ms Redis timeout; recovery preserves the previously consumed quota |
| Built executable | Two separately launched `ratelimitd -config ...` processes share Redis state: 7 of 100 requests reach the upstream and the remaining 93 return 429 with retry headers |
| Repository checks | Full `go test -race -count=1 ... ./...`, `go vet ./...`, command build, shell syntax validation and `git diff --check` pass |

The dedicated runner executed 23 top-level tests without skips. The full
repository race run passed, with four separately gated process-based tests
skipped because `REDIS_SERVER_BIN` was not provided:

- `TestRedisReadinessOOMAndRevisionMigration`
- `TestSentinelPromotionPreservesReplicatedQuota`
- `TestClusterSharedQuota`
- `TestRedisSharedQuotaReadinessAndCompatibility`

Go statement coverage for token-bucket packages was 93.3% for RAM and 87.5% for
the Redis Go adapter. These figures do not measure Lua statement coverage;
Redis script behavior is exercised through real Redis integration tests.

## Reproduce

```sh
cd disturbuted-rate-limiting
make test-token-bucket
```

The runner builds the server, starts and removes its own Docker Redis, and runs
algorithm, SDK, HTTP and executable checks with race detection in the test
process. A supplied `REDIS_TEST_URL` uses that backend instead. Normal `go test`
without the Redis URL skips Redis cases; the dedicated runner supplies it.

The default local macOS SDK had a linker incompatibility. These checks used an
already installed compatible SDK without changing system configuration:

```sh
SDKROOT=/Library/Developer/CommandLineTools/SDKs/MacOSX15.4.sdk make test-token-bucket
```

Artifacts:

- [Dedicated integration run](token-bucket-integration-tests.txt)
- [Full repository race run](token-bucket-full-tests.txt)
- [Go coverage profile](token-bucket-coverage.out)
- [RAM microbenchmarks](token-bucket-benchmarks.txt)

## Deployment limits

- Use Redis for a shared quota across server replicas. RAM quota is local to each
  process and resets when that process restarts.
- Launch the configurable proxy with `ratelimitd -config proxy.json` and set each
  intended route to `algorithm: "token_bucket"`, with positive `capacity`,
  `limit`, `window` and `max_keys`. The legacy flags/demo retain fixed windows.
- Set bounded Redis timeouts, disable automatic retries, check readiness before
  serving traffic, and make all replicas agree on policy revision and settings.
  The existing proxy does this; SDK users own their Redis client configuration.
- Configure Redis to retain quota state. Eviction, loss of persisted state or
  unreplicated writes lost during failover can restore allowance. Failover
  durability and Cluster/Sentinel routing were not tested for this algorithm.
- Capacity rejection in RAM scans up to `MaxKeys` identities for reclaimable
  state. On this machine, the synthetic 10,000-key benchmark averaged about
  235 microseconds per new-identity rejection, versus about 50 ns for a tracked
  identity. These are in-process averages with an injected clock, not HTTP
  latency or production capacity estimates. Redis can also spend extra time
  reclaiming many expired identities in one script.
- Choose burst capacity for the upstream's actual capacity and load-test the
  intended rate, identity cardinality and Redis topology before rollout. This
  verification does not include a long-running soak, production-scale load,
  token-bucket-specific OOM/read-only Redis injection, or HA failover.
