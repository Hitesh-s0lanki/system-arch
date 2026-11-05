# Local load tester

An independent Go HTTP service that queues tests, launches temporary k6 containers on the local Docker Engine, and exposes results through JSON APIs. It does not modify or start the repository's existing frontend, rate limiter, or target services.

## Start

Install and start Docker Desktop (or Docker Engine with Compose), then:

```sh
cd load-tester
cp .env.example .env
docker compose up --build -d
curl http://localhost:8090/ready
```

The API is published only on host loopback. `/health` checks HTTP liveness; `/ready` checks Docker Engine access. The first test pulls the pinned k6 image if it is absent, so it can take longer to start. Startup recovery requires Docker access when unfinished jobs exist.

On macOS, if the default Docker socket is unavailable, set `DOCKER_SOCKET` in `.env` to the socket path reported by `docker context inspect --format '{{.Endpoints.docker.Host}}'` (omit `unix://`).

## Submit a test

Use an endpoint you control. The URL must be reachable **from a k6 container**.

```sh
curl -s http://localhost:8090/tests \
  -H 'Content-Type: application/json' \
  -d '{
    "targetUrl": "http://host.docker.internal:8080/api/items",
    "method": "GET",
    "virtualUsers": 5,
    "durationSeconds": 10,
    "requestTimeoutSeconds": 5,
    "p95ThresholdMs": 200,
    "maxErrorRate": 0.01
  }'
```

The response is HTTP `202` with an `id`, `status: queued`, and a `Location` header. Poll `/tests/ID` until the job finishes, then read `/tests/ID/results`.

Optional `headers` is a string map and `body` is a string. Supported methods: GET, POST, PUT, PATCH, DELETE, HEAD, OPTIONS. Limits: 1–200 virtual users, 1–300 seconds, 1–30 seconds per request, 64 KiB body, 32 headers. Request timeout defaults to 10 seconds. `maxErrorRate` defaults to zero and counts all non-2xx responses, including 429 and redirects. `p95ThresholdMs` is optional and applies only to successful-request latency. A p95 is unavailable when no successful requests were measured.

The default workload uses a constant number of virtual users and sends the next request after the previous one completes. For explicit requests per second or capacity exploration, use the arrival-rate modes below. Redirects are disabled so the configured endpoint is measured directly. Response bodies are discarded; request headers, bodies, URLs, and status codes determine this generic workload. Business workflow scripting is future work.

## Requested RPS and a suggested rate limit

Set `requestsPerSecond` to generate a paced workload instead of a constant-VU loop. With this fixed script there is one HTTP request per iteration, so the arrival rate is the requested RPS. k6 attempts that rate independently of response latency. `virtualUsers` becomes the preallocated pool (default 10), and `maxVirtualUsers` is its expansion ceiling (default at least 50, maximum 200). The container still has 1 CPU and 512 MiB; it may not deliver the requested rate. Dropped iterations are exposed and prevent a recommendation for that step.

```json
{
  "targetUrl": "http://host.docker.internal:8080/api/items",
  "requestsPerSecond": 20,
  "durationSeconds": 30,
  "virtualUsers": 10,
  "maxVirtualUsers": 50,
  "p95ThresholdMs": 200,
  "maxErrorRate": 0.01,
  "headroomPercent": 20
}
```

For capacity exploration, send increasing `rateSteps` to `POST /tests` and omit `requestsPerSecond` and top-level `durationSeconds`:

```sh
curl -s http://localhost:8090/tests \
  -H 'Content-Type: application/json' \
  -d '{
    "targetUrl": "http://host.docker.internal:8080/api/items",
    "rateSteps": [
      {"requestsPerSecond": 5, "durationSeconds": 30},
      {"requestsPerSecond": 10, "durationSeconds": 30},
      {"requestsPerSecond": 20, "durationSeconds": 30}
    ],
    "requestTimeoutSeconds": 5,
    "p95ThresholdMs": 200,
    "maxErrorRate": 0.01,
    "headroomPercent": 20
  }'
```

Each plateau uses its own constant-arrival-rate scenario. Outstanding requests drain for up to `requestTimeoutSeconds` before the next plateau starts, keeping the metrics separate. All configured steps run, even if an earlier step fails. There are at most 8 increasing steps, rates are 1–1000 RPS, and total step time including graceful drains must fit within 300 seconds. A positive `p95ThresholdMs` is required for stepped tests. `headroomPercent` defaults to 20 and accepts 1–90.

Read `GET /tests/ID/results`. Alongside the existing metrics, `results.rateAnalysis` returns:

- `steps`: requested/observed RPS, completed request count, successful count, status counts, error fraction, successful p95, dropped iterations, evaluation status, and reasons.
- `highestPassingRps`: highest rate in the initial consecutive sequence of passing steps. Later passing steps after a failure/inconclusive step cannot raise it.
- `recommendedRateLimitRps`: `floor(highestPassingRps × (1 − headroomPercent/100))`, or `null` when the evidence is insufficient or the rounded suggestion is below 1 RPS.
- `boundary`: `lower_bound` if all steps pass, `threshold_crossed` when a failed step ends the passing sequence, or `inconclusive` when the generator/evidence prevents evaluation.
- `scope` and `explanation`: what was measured and how the provisional recommendation was calculated.

A passing step must meet the configured error and successful p95 thresholds, deliver at least 95% of the scheduled requests with zero dropped iterations, run for at least 30 seconds, and contain at least 100 successful responses. These are explicit evidence gates, not statistical confidence guarantees. A missing latency threshold, short run, or underpowered generator produces no suggestion for that step. `observedRps` is completed requests divided by the scheduled plateau duration; completions during the final drain are included. Missing successful latency is `null`, not zero. For requested-rate tests the k6 threshold outcome also checks delivery, dropped iterations, and per-step latency/errors; evidence duration/sample gates are separately reported in `rateAnalysis`.

Example: if 10 RPS passes and 20 RPS fails, the provisional suggestion with 20% headroom is **8 RPS**. The precise ceiling between those rates was not measured. If every step passes, the highest tested rate is only a lower bound; test additional rates to explore further. Suggestions apply to aggregate traffic for the tested endpoint, workload, and client identity. They do not establish a per-user quota, account for unmeasured routes or other traffic, or configure the server's limiter. Validate longer representative runs before applying the suggestion.

Existing constant-VU results cannot be converted into a capacity recommendation; submit a new requested-rate or stepped test.

## API

All responses are JSON. Lists accept `limit` (default 100, maximum 1000) and `offset` (default 0).

| Method | Endpoint | Behavior |
| --- | --- | --- |
| GET | `/health` | HTTP process liveness |
| GET | `/ready` | Docker availability; 503 if unavailable |
| POST | `/tests` | Validate and queue a job; 202 or 400/503 |
| GET | `/tests` | Persisted history, newest first; `items`, `limit`, `offset` |
| GET | `/tests/{id}` | Configuration, status, timestamps, exit code, execution error, threshold outcome |
| POST | `/tests/{id}/cancel` | 202 for cancellation; 409 if already terminal/cancelling |
| GET | `/tests/{id}/results` | `test` and `results` containing `metrics`, `statusCounts`, `insights` |
| GET | `/tests/{id}/metrics` | Timestamped k6 points; `items`, `limit`, `offset`, `hasMore`, `nextOffset` |
| GET | `/tests/{id}/logs` | Execution logs, capped at 1 MiB |

Unknown test IDs return 404. Results, metrics, and logs return 409 while queued/running/cancelling or when the requested artifact is absent/incomplete. Partial results after interruption are not guaranteed. These are post-run artifact APIs; live progress streaming is not implemented.

Headers, body, and URL query strings are omitted from public job responses. The fixed script excludes URL and error-message metric tags. Logs replace the full configured target URL and configured header values; do not treat this as a general-purpose secret scrubber. Request configuration is persisted locally in SQLite, so protect the Docker volume.

### Job lifecycle

```text
queued → running → completed
   │        │          └── thresholdsPassed: true / false
   │        ├── failed (execution/timeout/infrastructure failure)
   │        └── cancelling → cancelled (after container cleanup)
   └── cancelled
```

One worker executes one test at a time to reduce interference between local benchmarks. Up to 20 pending/running/cancelling jobs are accepted. k6 containers are limited to 1 CPU, 512 MiB RAM, and 128 processes. Tests also have an execution deadline covering the configured workload plus time for image preparation. Results depend on these generator limits and host load.

A k6 exit code of 99 means a completed measurement with failed thresholds, not a runner failure. Execution failures have `status: failed`. A graceful shutdown interrupts active execution and preserves queued jobs. After a crash, startup removes containers for previously running/cancelling jobs, records interruption/cancellation, and resumes queued tests. A cleanup failure stops the worker rather than silently losing track of the container; restart after restoring Docker access to reconcile it.

### Results

`metrics` retains the structured k6 summary. Useful entries include:

- `http_reqs.values.count` and `.rate`: total requests and achieved throughput.
- `successful_latency.values.med`, `p(95)`, `p(99)`: successful-request latency in milliseconds.
- `rate_limited_latency`: 429 response latency, reported separately.
- `other_error_latency`: latency of other unsuccessful requests.
- `unsuccessful_requests.values.rate`: non-2xx fraction.
- `transport_failures.values.count`: requests with no HTTP response.
- `request_timeouts.values.count`: request timeout count.

`statusCounts` contains counts by HTTP status; `"0"` means no HTTP response. Insight messages describe failed thresholds and observed errors without guessing a server-side cause. Metrics without observations may be absent. Raw metrics retain timestamps for client-side charts. Exact nested summary fields follow the pinned k6 version.

HTTP measurements do not expose target CPU, memory, database timings, or internal traces. A fixed-load test does not establish maximum capacity. Arrival-rate tests additionally return `rateAnalysis` with per-step evidence and a provisional limit when the evidence gates pass. Baseline/protected runs can be submitted separately, but this service does not toggle a target's limiter or implement a comparison endpoint yet.

## Networking and persistence

For a host service on Docker Desktop use `http://host.docker.internal:PORT`. The worker adds the host-gateway alias for Linux too; the target must listen on an interface reachable from Docker. `localhost` would refer to the k6 container itself.

For a container target, attach that target to the `load-tester` network and use its network name and internal port. To use a network already managed by another Compose project, configure `testing` as an external network in a local Compose override and set `K6_NETWORK` to that network's name. The service does not alter other services automatically.

The API container and dynamically created k6 containers mount the same named volume at `/data`. It holds `jobs.sqlite` and per-job `summary.json`, `metrics.json`, `logs.txt`, and `script.js` files. The temporary `config.json` is removed on normal completion. The SQLite record retains the submitted configuration. Keep `DATA_DIR=/data` for Compose; the k6 mount path is fixed. `RESULTS_VOLUME` and `K6_NETWORK` must match the actual volume and network names.

No retention policy is implemented yet: retained raw metrics can grow substantially. `docker compose down` stops the service and preserves results. Do not run `down -v` unless you intend to delete all test data.

Docker socket access gives this local service Docker Engine control. Keep the API local. No authentication or public deployment support is included. Users submit validated workload settings, never container images, shell commands, or JavaScript; the operator chooses the pinned image through environment configuration.

## Development and verification

```sh
go test -race ./...
go vet ./...
docker compose config --quiet
docker compose up --build -d
python3 tests/integration/smoke.py
python3 tests/integration/rates.py
```

The smoke test starts an isolated host HTTP fixture and exercises real k6 containers: success, 429, 500, timeout, unreachable target, POST body/header forwarding, shared-network targeting, metrics pagination, logs, cancellation, persistence, crash recovery, and cleanup. It pulls a small Nginx image for the shared-network fixture. It deliberately kills/restarts this service's Compose container and retains test records. It does not modify the existing application services.

The rate test adds fixed-RPS delivery, generator saturation, and two 30-second plateaus against a fixture with a known limit. It verifies that API guidance uses passing evidence and headroom.

The Go service invokes the installed Docker CLI using argument arrays, without a shell. The runtime image includes the CLI; no nested Docker daemon is started. SQLite uses the pure Go `modernc.org/sqlite` driver. See [TASKS.md](TASKS.md) for implementation status.

## References

- [Docker container creation](https://docs.docker.com/reference/cli/docker/container/create/)
- [Docker Desktop networking](https://docs.docker.com/desktop/features/networking/networking-how-tos/)
- [k6 custom summaries](https://grafana.com/docs/k6/latest/results-output/end-of-test/custom-summary/)
- [k6 constant arrival rate](https://grafana.com/docs/k6/latest/using-k6/scenarios/executors/constant-arrival-rate/)
- [k6 granular JSON output](https://grafana.com/docs/k6/latest/results-output/real-time/json/)
