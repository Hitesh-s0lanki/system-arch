# Sliding-window test review and verification

Reviewed and verified on 2026-11-11 using real Redis 7.4.12 in isolated Docker
containers. No algorithm defect was observed in these runs. This review changed
tests and the test runner; it did not change the limiter implementation.

## Test review findings and improvements

- Boundary tests previously omitted rejection reasons. They now compare complete
  decisions, including allowance, remaining count, reason, and exact retry time.
- The Redis rolling-window fixture previously used RAM as its only oracle. It now
  checks both implementations against explicit, independently specified results.
  The separate 3,000-step RAM reference model still checks broad event sequences.
- Several tests discarded constructor errors. The reviewed algorithm tests now
  check setup failures before using a limiter.
- Added checks that rejected attempts neither extend retention nor allocate
  capacity; heap rescheduling preserves partially active identities; cleanup
  retires identities exactly once; and nanosecond/millisecond boundaries behave
  correctly. Whitespace in valid keys and the 256-byte UTF-8 boundary are covered.
- Added a deterministic mutex-wait cancellation test. Synchronization ensures the
  request passes its initial context check before cancellation, so this exercises
  the post-lock cancellation check without sleeps.
- Added policy/revision/tenant isolation, persistent compatibility guards after
  quota expiration, invalid/canceled operations writing no Redis state, a bounded
  backend timeout, and a real lost-reply test proving ambiguous writes are not
  retried or refunded.
- SDK tests now verify complete decisions and tenant/route/subject isolation,
  capacity, and cancellation. HTTP tests check remaining/retry/cache headers,
  rejection bodies, capacity status, and whether rejected traffic reaches upstream.
- The runner now selects sliding-window SDK/proxy cases explicitly, enables
  shuffled order, and supports repeated runs through `SLIDING_TEST_COUNT`.

## Results

| Verification | Result | Evidence |
| --- | --- | --- |
| RAM and real Redis algorithm tests, race detection | Passed | [Algorithm test output](sliding-window-algorithm-tests.txt) |
| Algorithm tests, shuffled order, 20 repetitions, race detection | Passed | [Repeated test output](sliding-window-stress-tests.txt) |
| Sliding-window SDK/proxy tests, shuffled order, 10 repetitions, race detection | Passed | [Integration test output](sliding-window-integration-tests.txt) |
| Complete repository test suite with real Redis and race detection | Passed | [Full test output](sliding-window-full-tests.txt) |
| Updated Docker-owning test runner | Passed; temporary Redis removed on exit | [Runner output](sliding-window-runner-tests.txt) |
| Deliberate behavioral mutations | All 5 detected by failing assertions | [Mutation results](sliding-window-mutation-tests.txt) |
| `go vet` for sliding-window, SDK, and proxy test packages | Passed | Command completed without diagnostics |
| `git diff --check` | Passed | No whitespace errors |

The full-suite run supersedes the earlier report's leaky-bucket failure; that
failure was not present in this run. Existing process-based Redis failure/HA tests
still skip without `REDIS_SERVER_BIN`. All sliding-window Redis cases ran against
real Redis and did not skip. Cluster/Sentinel/failover for this algorithm remains
outside the verified scope.

## Coverage and test sensitivity

Merged Go statement coverage from [the profile](sliding-window-coverage.out):

- RAM: 85/85 statements, **100.0%**.
- Redis Go adapter: 34/36 statements, **94.4%**.
- Combined: 119/121 statements, **98.3%**.

The two uncovered adapter statements handle an invalid script result length or an
unknown decision code. Go statement coverage does not measure Lua coverage. Redis
behavior is exercised by actual script execution, rolling-boundary fixtures,
concurrency, persistence/expiry, and error tests. Coverage is supporting evidence,
not proof of correctness or production performance.

Five temporary source overlays introduced these bugs without changing workspace
source files. Every mutation caused the intended behavioral test to fail:

1. RAM includes a timestamp exactly at the expired boundary.
2. RAM permits one extra identity at capacity.
3. RAM records rejected requests in the active log.
4. Redis includes a timestamp exactly at the expired boundary.
5. Redis permits one extra identity at capacity.

## Reproduce

From the rate-limiter module directory:

```sh
make test-sliding-window
```

The runner starts a temporary Docker Redis when `REDIS_TEST_URL` is unset and
cleans it up. Set `REDIS_TEST_URL` to use a dedicated existing Redis instead.
Use `SLIDING_TEST_COUNT=20` to repeat both algorithm and integration cases.

This machine's default macOS SDK caused external linker errors. Runs used a
compatible installed SDK through a per-command environment setting:

```sh
SDKROOT=/Library/Developer/CommandLineTools/SDKs/MacOSX15.4.sdk make test-sliding-window
```

See [the usage guide](../docs/sliding-window.md) for configuration and storage.
