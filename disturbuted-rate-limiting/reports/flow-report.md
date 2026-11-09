# Client → rate limiter → backend flow report

Tested: 2026-11-06 12:15:33 IST. Public origin: https://hiteshsolanki.com/.

All 66 flow checks passed. The full Go race-test suite also passed with a real Redis container; integration tests were enabled, not skipped. No production code or default rate configuration changed for this test.

## Actual website

The client called a local Docker gateway, which applied an IP policy and forwarded admitted requests to the actual HTTPS origin. The origin TLS certificate was verified. Redirects were not followed automatically. All admitted responses were HTTP 200 and matched the direct response byte-for-byte (265,903 bytes).

| Path | Cold full response | Warm median full response | Warm median time to response headers |
| --- | ---: | ---: | ---: |
| Direct to website | 72.59 ms | 42.19 ms | 10.82 ms |
| Through RAM proxy | 66.85 ms | 51.61 ms | 14.52 ms |
| Through Redis proxy | 73.42 ms | 70.09 ms | 17.30 ms |

One cold plus five warm GETs per path were measured: six direct, six through RAM, and six through Redis. An earlier independent direct baseline probe was also made. No concurrency, POST, or intentional failures were sent to the public site. Tests ran sequentially with a short pause between public timing requests.

For these temporary proxy instances, the test policy allowed six requests per IP per hour. After the sixth request, 50 requests per proxy returned 429 locally, including requests with forged forwarding headers. The unchanged example configuration still allows three per 10 seconds.

Median blocked response: RAM 1.12 ms; Redis 0.87 ms (50 local rejections each).

These five-sample public medians are observations, not stable latency guarantees. WAN, CDN/cache, TLS, connection reuse, payload transfer, and Docker networking affect them. Public p95/p99 estimates are intentionally omitted. Origin access logs were unavailable; proof that rejected requests do not reach an upstream comes from the instrumented controlled backend below.

## Controlled overhead measurement

200 requests per path were interleaved across direct, RAM, and Redis paths using warmed persistent connections. The same controlled HTTP backend performed 2 ms of work and returned a small JSON response. All 600 measured requests succeeded. The backend ran on the host and the proxies/Redis in Docker; added time includes the extra TCP/proxy hop and Docker Desktop networking.

| Path | Median | p95 | p99 |
| --- | ---: | ---: | ---: |
| Direct to backend | 3.13 ms | 3.61 ms | 3.78 ms |
| RAM proxy | 5.06 ms | 6.68 ms | 7.45 ms |
| Redis proxy | 5.44 ms | 7.65 ms | 8.43 ms |

Median of paired added times: RAM **1.98 ms**, Redis **2.35 ms**. These are whole-path differences, not the standalone algorithm's execution time.

This was a small local experiment, not a sustained production load test. Concurrency validation was separate: 40 requests using eight workers, with exactly 10 admitted and 30 rejected in each storage mode.

## Checked behavior

| Case | Result |
| --- | --- |
| IP allowance exhausted | Expected 200/429 sequence in both modes |
| Forged X-Forwarded-For | Did not create a new allowance |
| Blocked requests | Instrumented backend count increased only for admitted requests |
| Concurrent requests | Exactly 10 admitted out of 40; 30 rejected; backend saw only 10 |
| Missing, invalid, forged user authentication | 401; verified token accepted |
| Separate authenticated users | Independent allowances |
| API credential identity | First accepted, next rejected at configured limit |
| New identity at capacity | 503 rather than misleading 429 |
| Canonical routes | Different item IDs shared a quota; a different template had its own |
| HTTP forwarding | POST method, body, repeated query values, upstream 201 and headers preserved |
| Header sanitation | Unverified user header and bearer token not sent upstream by default |
| Slow backend | 504 in both modes |
| Protocol upgrade | Explicit 501, as documented |
| Window reset | Rejected within window; accepted at next window |
| Redis outage | 503, no backend forwarding, readiness 503, liveness 200 |
| Redis recovery | Forwarding resumed |
| Actual public HTTPS page | 200 and exact body match in both modes |

The existing race-test suite additionally checked shared Redis quotas/capacity across replicas, expiry, configuration mismatch, upstream connection failure (502), trusted proxy chains, identity isolation, invalid config/keys, clock rollback, concurrent cleanup, and shutdown. This covers the current documented feature set; it does not prove every possible production failure case.

## Reproduce

Build the current image, then run:

```sh
docker build -t rate-limiter:local .
python3 tests/e2e/run_flow.py --upstream https://hiteshsolanki.com/ --samples 200
```

The runner creates isolated containers and a temporary controlled backend, writes reports/flow-results.json, and removes its containers/network afterward. The controlled fixture temporarily listens on the host so Docker can reach it; it closes when the run ends. Use only an upstream you are authorized to test. Never replace the small public GET run with a production load/failure test.

Raw results: [flow-results.json](flow-results.json).
