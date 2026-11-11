# Rate limiter latency measurements

Run all five algorithms through the same RAM and real Redis workloads:

```sh
make benchmark-latency
```

The runner starts and removes a disposable Docker Redis. Alternatively, supply
`REDIS_TEST_URL` for an isolated test Redis. Set `LATENCY_SAMPLES` to change the
number of timed requests per case (default 2,000, minimum 100, maximum 100,000).
Run without race instrumentation. On macOS, select a compatible installed SDK if
required by the local linker:

```sh
SDKROOT=/Library/Developer/CommandLineTools/SDKs/MacOSX15.4.sdk make benchmark-latency
```

The command saves the canonical report to
`reports/rate-limiter-latency.json`, then exports an identical snapshot to
`../frontend/public/benchmarks/rate-limiter.json`. The algorithms page reads that
snapshot and offers a JSON download. Refresh the page after regenerating results;
redeploy the frontend to update results on a hosted installation. No benchmarks
run in a visitor's browser.

The report contains 120 measurements: five algorithms × two backends × six
workloads × two worker counts. Each case has 100 warmup requests, excluded from
timing. Every measurement records p50, p95, p99 and maximum latency in microseconds,
completed operations per second, sample count, accepted/rejected count and errors.
The page converts values of at least 1,000 microseconds to milliseconds.

Workloads:

- `allowed-hot`: accepted requests for one identity. Timestamp logs grow across
  warmup and measurement; this measures that growing log, not a constant log size.
- `denied-hot`: a pre-exhausted identity; all timed requests must be rejected.
- `allowed-1000-identities`: accepted requests cycling across 1,000 identities;
  the 100 warmup requests touch the first 100 identities, so later insertions are
  included in timing.
- `capacity-full-1000-identities`: 1,000 prepopulated identities, then checks for
  an additional identity. All checks must return capacity-exceeded.
- `http-allowed`: complete local TCP requests through the real proxy to a minimal
  local HTTP upstream returning 204. Includes forwarding and response handling.
- `http-denied`: complete local TCP requests rejected by the real proxy with 429.
  The upstream receives only the setup request, never a timed rejected request.

Each workload runs with one and 16 concurrent workers. Each worker waits for a
response before issuing its next request; this is a closed-loop workload, not
an externally imposed arrival rate. Redis connections are pooled with automatic
retries disabled. All cases use a one-hour window, a 1,000-identity budget, and
fresh independent policy state. Accepted cases use `samples + 101` for their
limit and bucket capacity; rejected cases use one. The same settings apply across
algorithms. The JSON records those settings, platform, CPU count, GOMAXPROCS,
Go version, Redis version, timestamp and method.

The runner validates decisions and upstream counts, fails on any backend errors,
and replaces the report only after every scenario completes successfully. It
cleans its own Redis namespace and any container it created. Existing reports
remain unchanged if measurement fails before writing a new report.

These are latency observations from one local run, not correctness percentages
or production service guarantees. Hardware load, Redis placement, network latency,
identity cardinality and log size affect the numbers. The results do not measure
TLS, remote upstreams, sustained arrival-rate overload, failover, allocations or
memory consumption. Compare rows with the same workload and worker count;
repeat on deployment hardware before using them for capacity planning.
