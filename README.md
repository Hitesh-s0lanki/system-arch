# system-arch

Hands-on system design playground: production-shaped services that demonstrate rate limiting, load testing, and adaptive video streaming, tied together by a Next.js control panel.

## Projects

| Directory | What it is | Stack |
| --- | --- | --- |
| [`disturbuted-rate-limiting/`](disturbuted-rate-limiting/) | Distributed rate limiter runnable as an embedded Go library, a demo server, or a configurable reverse proxy. Implements fixed-window, sliding-window log, sliding-window counter, token-bucket, and leaky-bucket algorithms with per-process RAM or shared Redis storage. | Go, Redis, Docker Compose |
| [`load-tester/`](load-tester/) | Local load-testing service that queues tests, launches ephemeral k6 containers against a target endpoint, and reports latency/error metrics plus a suggested rate limit. | Go, k6, Docker |
| [`frontend/`](frontend/) | Web dashboard: system catalog, rate limiter controls, and a load-tester workbench that starts/stops the local service and walks through capacity finding. | Next.js, TypeScript, Playwright |
| [`video-streaming-service/`](video-streaming-service/) | Netflix-style HLS adaptive bitrate streaming demo. Upload an MP4, get FFmpeg-transcoded renditions (360p/720p/1080p) and `master.m3u8` playback. | Java, Spring Boot, FFmpeg |

## Repo layout

```
system-arch/
├── disturbuted-rate-limiting/   # Go rate limiter (library, server, reverse proxy)
│   ├── algorithm/               # One package per limiting algorithm (+ Redis backends)
│   ├── sdk/                     # Embedded RAM/Redis constructors for apps
│   ├── docs/                    # Algorithm comparisons, SDK, Docker, sliding-window guides
│   └── deploy/                  # Deployment configuration
├── load-tester/                 # k6-driven load testing API
├── frontend/                    # Next.js dashboard (catalog, limiter UI, load-tester UI)
├── video-streaming-service/     # Spring Boot HLS streaming service
└── docs/                        # Top-level documentation
```

## Quick starts

Each project is self-contained — see its README for full details.

### Rate limiter (Docker)

```sh
cd disturbuted-rate-limiting
docker compose up -d --build --wait
curl -i http://127.0.0.1:8088/orders   # hammer it to hit limits
```

Swap to shared Redis storage with:

```sh
docker compose down
docker compose -f compose.yaml -f compose.redis.yaml up -d --build --wait
```

Run the frontend against it with `cd frontend && npm run dev` and open the **Rate Limiter** section.

### Load tester

```sh
cd load-tester
cp .env.example .env
docker compose up --build -d
curl http://localhost:8090/ready
```

Then drive it from the frontend's **Load Tester** page (`/system-design/load-tester`) or the JSON API directly.

### Video streaming

```sh
cd video-streaming-service
mvn spring-boot:run
```

Upload an MP4 via the API and stream it from any HLS-compatible player.

## Documentation

- [Rate-limiting algorithms](disturbuted-rate-limiting/docs/algorithms.md) — algorithm comparison and trade-offs
- [Sliding window guide](disturbuted-rate-limiting/docs/sliding-window.md) and [counter guide](disturbuted-rate-limiting/docs/sliding-window-counter.md)
- [Rate limiter SDK](disturbuted-rate-limiting/docs/sdk.md) — embedding in Go applications
- [Docker guide](disturbuted-rate-limiting/docs/docker.md) — configuration, auth, storage guarantees
- [Load tester README](load-tester/README.md) — API reference, RPS modes, capacity suggestions
- [Video streaming README](video-streaming-service/README.md) — HLS architecture and how ABR works

## Conventions

- Frontend component placement follows the rules in [`GEMINI.md`](GEMINI.md) (page-local `_components/` folders, shared components in `src/components/`).
- Go packages mirror their tests under each project's `tests/` directory.
