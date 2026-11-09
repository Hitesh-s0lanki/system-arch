# Production hardening verification

Verified locally on 7 November 2026. Runtime: Go 1.26.10, macOS ARM64,
Redis 7.4.7, k6 1.0.0. This report is separate from the earlier flow report.

| Check | Result |
| --- | --- |
| Full uncached Go race suite | Passed with REDIS_TEST_URL and REDIS_SERVER_BIN set |
| Shared Redis quotas/capacity and expiry | Passed |
| Per-tenant isolation and unprovisioned tenants | Passed |
| Policy guard conflict before first traffic and revision reset | Passed |
| Oversized declared/chunked bodies rejected before quota/forwarding | Passed |
| Concurrent overload rejected without queuing or quota consumption | Passed |
| Redis OOM, writable readiness, recovery, read-only primary | Passed |
| Sentinel automatic promotion after isolated primary shutdown | Passed; confirmed-replicated quota preserved |
| Three-primary Cluster and two gateway instances | Passed; shared quota enforced |
| Ambiguous Cluster script reply after execution | Passed; returned error, increment not replayed |
| OIDC signature, issuer, audience, expiry, claim validation, key rotation | Passed using a trusted local TLS issuer and real RSA signatures |
| Metrics authorization and absence of sensitive identity labels | Passed |
| go vet / go mod verify | Passed |
| govulncheck v1.9.0 | No vulnerabilities found with Go 1.26.10 |
| Linux AMD64 static cross-build | Passed |
| RAM / Redis / two-replica load Compose config | Valid |
| Kubernetes schema validation (kubeconform v0.7.0) | Six resources valid, zero invalid/errors/skipped |

## Sustained local load

Two gateway processes shared one isolated standalone Redis namespace and forwarded
to a local controlled HTTP backend. The policy used a high allowance so the test
measured admitted request latency. No public upstream was used. The same k6 script
in `tests/load/sustained.js` ran for two minutes at 100 iterations per second,
32 preallocated VUs, and 128 maximum VUs. The backend returned a small `ok` body.

- Requests: 12,001; all returned the expected HTTP 200.
- Unexpected responses, failed checks, and dropped iterations: zero.
- Response duration: median 2.097 ms; p95 4.044 ms.
- p99 13.48 ms, as reported by k6's threshold output.
- Thresholds passed: p95 < 50 ms, p99 < 100 ms, unexpected responses < 0.1%, no drops.

[Raw k6 summary](hardening-load-summary.json) and [race suite output](hardening-tests.txt)
are saved alongside this report. The k6 JSON export uses its default trend summary
fields; p99 is recorded above from the threshold result printed during the run.

## Limits of this verification

This is a local smoke/correctness run, not a production capacity or durability
certification. It does not establish multi-region guarantees, zero-loss failover,
peak throughput, arbitrary tenant cardinality, or a production latency SLO.

Docker's daemon was unavailable: Compose configuration and remote image tags were
checked, but a container image build/run was not performed locally. CI contains
container build and two-container k6 jobs; those jobs have not run in GitHub during
this session. Kubernetes resources were schema-validated, not deployed. Production
Redis/TLS, identity-provider configuration, secret injection, and final resource
budgets require the actual deployment environment.
