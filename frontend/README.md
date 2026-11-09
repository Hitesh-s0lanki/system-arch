This is a [Next.js](https://nextjs.org) project bootstrapped with [`create-next-app`](https://nextjs.org/docs/app/api-reference/cli/create-next-app).

## Getting Started

First, run the development server:

```bash
npm run dev
# or
yarn dev
# or
pnpm dev
# or
bun dev
```

Open [http://localhost:3000](http://localhost:3000) with your browser to see the result.

You can start editing the page by modifying `app/page.tsx`. The page auto-updates as you edit the file.

This project uses [`next/font`](https://nextjs.org/docs/app/building-your-application/optimizing/fonts) to automatically optimize and load [Geist](https://vercel.com/font), a new font family for Vercel.

## Learn More

To learn more about Next.js, take a look at the following resources:

- [Next.js Documentation](https://nextjs.org/docs) - learn about Next.js features and API.
- [Learn Next.js](https://nextjs.org/learn) - an interactive Next.js tutorial.

You can check out [the Next.js GitHub repository](https://github.com/vercel/next.js) - your feedback and contributions are welcome!

## Deploy on Vercel

The easiest way to deploy your Next.js app is to use the [Vercel Platform](https://vercel.com/new?utm_medium=default-template&filter=next.js&utm_source=create-next-app&utm_campaign=create-next-app-readme) from the creators of Next.js.

Check out our [Next.js deployment documentation](https://nextjs.org/docs/app/building-your-application/deploying) for more details.

## Load Tester workbench

Open `/system-design/load-tester` from **Load Tester** in the sidebar. **Rate Limiter** remains a separate section for limiter controls.

1. Install Docker Desktop on this computer. It can be stopped initially.
2. Run this frontend with `npm run dev`, open Load Tester, and click **Start load tester** if it is offline. The page checks Docker, opens Docker Desktop if needed, starts the local service, and reconnects automatically.
3. Enter your website or server address. For a host API, use `http://host.docker.internal:PORT/path`.
4. Enter **Maximum load**, the largest number of requests per second you want to try, then click **Test my server**. The app automatically tests smaller loads before reaching that maximum. **Advanced settings** holds response-time and failure limits, spare capacity, request settings, and an optional steady-traffic mode.
5. Run the test. The page polls status, supports cancellation, and restores the selected test after a reload. Recent runs reopen stored reports.
6. Review the provisional aggregate suggestion, headroom, per-step latency/errors/dropped iterations, logs, or download the JSON report.

The Next.js server forwards only supported API routes through `/api/load-tester/*`. The backend URL is server-side: `LOAD_TESTER_URL`, default `http://127.0.0.1:8090`; see `.env.example`. If the frontend runs in Docker, configure a reachable backend service URL instead of container-local `127.0.0.1`. Restart Next.js after changing environment settings.

The backend and frontend are intended for local use. This screen measures an endpoint and suggests a limit; it does not enable the gateway's limiter. An all-passing test establishes the highest tested rate, not the maximum capacity. Suggestions require sufficient duration, successful samples, and delivered load. HTTP 429s count as errors in this capacity workflow.

### Browser end-to-end checks

With Docker and the load-tester service running:

```sh
npx playwright install chromium
npm run test:e2e:load-tester
```

The suite starts an isolated local target and uses real k6 jobs through the UI and Next.js proxy. It checks navigation, input validation, an automatically generated increasing-load test and recommendation, reload recovery, cancellation/container cleanup, logs/export, a short fixed-rate run, offline state, responsive layouts, RTL, and zoom. It does not send traffic to public websites. An existing frontend on port 3000 is reused, otherwise the suite starts it. `BROWSER_EXECUTABLE` can select an already-installed Chromium binary. Screenshots and failure traces are stored under ignored `test-results/`.

The main form accepts a single maximum, not a comma-separated RPS list. The frontend selects increasing loads at approximately one quarter, one half, and the chosen maximum, removing duplicates at low values. Each load runs for at least 30 seconds and enough time to schedule 120 requests, so a low maximum does not silently produce a sample too small for guidance. The chosen maximum is never exceeded. The displayed duration includes pauses between loads. All-passing results still indicate only the highest tested load, not an untested capacity ceiling.

### Starting Docker from the page

When the backend is offline, the startup panel checks whether Docker is installed and reachable. **Start load tester** opens Docker Desktop if necessary and starts only the load-tester Compose service. Startup progress appears on the page; duplicate clicks reuse the active startup operation. When `/ready` succeeds, the panel disappears and testing is enabled. If Docker is missing, the page asks for installation and offers **Check again**. Setup/permission prompts in Docker Desktop may still need your attention.

This is local server-side control: Next.js must run on your computer with access to the Docker CLI and the `load-tester/` source folder. The browser itself does not execute host commands. The management endpoint `/api/load-tester/control` accepts localhost URLs and rejects cross-origin requests; keep this local app off public networks. `LOAD_TESTER_PROJECT_DIR` optionally sets the backend folder. The configured `LOAD_TESTER_URL` must point to the local Compose service. Desktop startup uses `docker desktop start`, with a macOS application-launch fallback; Linux Engine installations that require privileged service startup must be started by their administrator.

Startup checks: `npx playwright test tests/e2e/startup.spec.ts`. These cover actual local startup, retry/install states, and request restrictions. They can open Docker Desktop and leave the load tester running.
