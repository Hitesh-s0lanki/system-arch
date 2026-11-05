# Independent load tester — implementation tracker

Scope: a standalone Go API that runs k6 containers on demand using local Docker and returns measurements and insights. Existing frontend and rate-limiter services remain untouched.

## 1. Service foundation

- [x] Create a separate `load-tester/` Go module and package structure.
- [x] Add environment configuration and bounded workload validation.
- [x] Add Dockerfile, independent Compose file, named volume, and network.
- [x] Add liveness and Docker readiness endpoints.

## 2. Job API and persistence

- [x] Implement test creation with asynchronous 202 responses.
- [x] Persist configuration, status, timestamps, execution errors, and outcomes in SQLite.
- [x] Implement paginated history and individual job retrieval.
- [x] Implement queued and running cancellation.
- [x] Enforce one active test and a bounded pending queue.
- [x] Preserve queued jobs and reconcile active jobs after restart.

## 3. On-demand Docker execution

- [x] Launch a temporary k6 container per job using the local Docker Engine.
- [x] Use a fixed embedded script and pass validated configuration through a file.
- [x] Share a named results volume and configure target networking.
- [x] Add resource limits, a deadline, container ownership labels, and cleanup.
- [x] Collect logs and exit codes; distinguish threshold failure from execution failure.

## 4. Measurements and insights

- [x] Persist structured summaries and timestamped metrics.
- [x] Expose throughput, request counts, latency percentiles, and HTTP status counts.
- [x] Separate successful, rate-limited, and other unsuccessful latency.
- [x] Count transport failures and request timeouts.
- [x] Explain observed errors and failed thresholds without inventing bottlenecks.
- [x] Expose results, paginated metrics, and bounded logs through APIs.
- [x] Omit request secrets from public configuration responses.

## 5. Verification and handoff

- [x] Test API validation, cancellation, threshold outcomes, SQLite persistence, and restart recovery.
- [x] Test result parsing, 429 separation, and metrics pagination.
- [x] Run Go race detection and static analysis.
- [x] Run real Docker smoke tests: success, 429, 500, timeout, unreachable target.
- [x] Verify POST bodies/headers and service-name routing to a target on the shared Docker network.
- [x] Verify real cancellation, crash recovery, persisted queue, and container cleanup.
- [x] Document startup, API usage, networking, storage, and operational limits.

## 6. Requested rate and capacity guidance

- [x] Add `requestsPerSecond` using a constant-arrival-rate workload.
- [x] Add increasing `rateSteps` with separate per-step metrics and drain periods.
- [x] Validate RPS bounds, VU allocation, headroom, thresholds, and total duration.
- [x] Report requested versus delivered RPS and dropped iterations.
- [x] Evaluate successful latency and errors independently at each rate.
- [x] Suggest an aggregate limit with configurable headroom from consecutive passing evidence.
- [x] Withhold suggestions for short/sparse, incomplete, or generator-limited measurements.
- [x] Distinguish a tested lower bound from a threshold crossing and inconclusive evidence.
- [x] Add recommendation and validation regression tests.
- [x] Verify fixed RPS, generator saturation, and stepped recommendations with real Docker runs.
- [x] Document the API fields, calculation, and limits of the recommendation.

## 7. Frontend integration

- [x] Add the workbench under the frontend's separate Load Tester navigation.
- [x] Proxy supported API calls through Next.js with a server-side service URL.
- [x] Add compact capacity and fixed-RPS forms with expandable request settings.
- [x] Poll jobs, cancel execution, restore selected runs, and show recent history.
- [x] Display provisional aggregate guidance, headroom, step evidence, and insufficient-data reasons.
- [x] Expose execution logs and downloadable JSON reports.
- [x] Test real Docker jobs through the browser, including reload, cancellation, and cleanup.
- [x] Test input validation, proxy restrictions, offline state, and insufficient evidence.
- [x] Verify desktop/mobile layouts, RTL, and 200% zoom.
- [x] Pass frontend type checking, targeted lint, and production build.

## Follow-up features (outside the initial MVP)

- [ ] Publish a versioned OpenAPI contract for frontend integration.
- [ ] Add retention/deletion for old results and disk-usage limits.
- [ ] Add server-side time-bucket aggregation for large chart datasets.
- [ ] Add live progress/events if required by the dashboard.
- [ ] Add explicit baseline/protected comparison records and endpoints.
- [ ] Add configurable multi-request business workflows.
- [ ] Add authentication before expanding beyond local use.
