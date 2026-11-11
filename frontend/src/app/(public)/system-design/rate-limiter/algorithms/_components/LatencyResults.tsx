'use client'

import { useEffect, useState } from 'react'

const reportUrl = '/benchmarks/rate-limiter.json'
const algorithmNames: Record<string, string> = {
  fixed_window: 'Fixed window',
  sliding_window_log: 'Sliding log',
  sliding_window_counter: 'Sliding counter',
  token_bucket: 'Token bucket',
  leaky_bucket: 'Leaky bucket meter',
}
const scenarios = {
  'allowed-hot': 'Decision · accepted · one identity',
  'denied-hot': 'Decision · rejected · one identity',
  'allowed-1000-identities': 'Decision · accepted · 1,000 identities',
  'capacity-full-1000-identities':
    'Decision · capacity full · 1,000 identities',
  'http-allowed': 'HTTP proxy · accepted',
  'http-denied': 'HTTP proxy · rejected',
}

type Measurement = {
  algorithm: string
  backend: 'ram' | 'redis'
  scenario: string
  workers: number
  samples: number
  allowed: number
  rejected: number
  errors: number
  p50_us: number
  p95_us: number
  p99_us: number
  max_us: number
  operations_per_second: number
}
type Report = {
  schema_version: number
  timestamp: string
  goos: string
  goarch: string
  go_version: string
  cpus: number
  gomaxprocs: number
  redis_version: string
  methodology: string
  warmup_requests: number
  window: string
  allowed_limit: number
  denied_limit: number
  max_keys: number
  results: Measurement[]
}

function parseReport(value: unknown): Report {
  const report = value as Report | null
  if (
    !report ||
    report.schema_version !== 1 ||
    !Number.isFinite(Date.parse(report.timestamp)) ||
    !Array.isArray(report.results) ||
    report.results.length !== 120
  )
    throw new Error('Invalid benchmark report')
  const keys = new Set<string>()
  for (const row of report.results) {
    if (
      !Object.hasOwn(algorithmNames, row.algorithm) ||
      !Object.hasOwn(scenarios, row.scenario) ||
      !['ram', 'redis'].includes(row.backend) ||
      ![1, 16].includes(row.workers) ||
      ![
        row.samples,
        row.allowed,
        row.rejected,
        row.errors,
        row.p50_us,
        row.p95_us,
        row.p99_us,
        row.max_us,
        row.operations_per_second,
      ].every((n) => typeof n === 'number' && Number.isFinite(n) && n >= 0) ||
      row.samples < 100 ||
      row.allowed + row.rejected + row.errors !== row.samples ||
      row.p50_us > row.p95_us ||
      row.p95_us > row.p99_us ||
      row.p99_us > row.max_us
    )
      throw new Error('Invalid benchmark measurement')
    keys.add(`${row.algorithm}:${row.backend}:${row.scenario}:${row.workers}`)
  }
  if (keys.size !== 120) throw new Error('Duplicate benchmark measurements')
  return report
}

function latency(us: number) {
  return us >= 1000 ? `${(us / 1000).toFixed(2)} ms` : `${us.toFixed(3)} µs`
}
const number = new Intl.NumberFormat('en-IN', { maximumFractionDigits: 0 })

export function LatencyResults({
  activeAlgorithm,
}: {
  activeAlgorithm: string
}) {
  const [report, setReport] = useState<Report | null>(null)
  const [failed, setFailed] = useState(false)
  const [attempt, setAttempt] = useState(0)
  const [scenario, setScenario] =
    useState<keyof typeof scenarios>('allowed-hot')
  const [workers, setWorkers] = useState(1)
  const [allMetrics, setAllMetrics] = useState(false)

  useEffect(() => {
    const controller = new AbortController()
    fetch(reportUrl, { signal: controller.signal, cache: 'no-store' })
      .then((response) => {
        if (!response.ok) throw new Error('Report unavailable')
        return response.json()
      })
      .then((value) => setReport(parseReport(value)))
      .catch(() => {
        if (!controller.signal.aborted) setFailed(true)
      })
    return () => controller.abort()
  }, [attempt])

  const rows =
    report?.results.filter(
      (row) => row.scenario === scenario && row.workers === workers,
    ) ?? []
  const selected = rows.filter(
    (row) => row.algorithm.replaceAll('_', '-') === activeAlgorithm,
  )

  return (
    <section
      id="latency"
      aria-labelledby="latency-title"
      className="min-w-0 scroll-mt-20 space-y-5"
    >
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h2
            id="latency-title"
            className="text-xl font-semibold tracking-tight"
          >
            Measured latency
          </h2>
          <p className="mt-2 text-sm leading-7 text-muted-foreground">
            RAM vs Redis. Same workload. Lower latency is faster.
          </p>
        </div>
        {report && (
          <a
            href={reportUrl}
            download="rate-limiter-latency.json"
            className="rounded-md text-sm font-medium text-primary underline underline-offset-4 focus-visible:outline-2 focus-visible:outline-ring"
          >
            Download results JSON
          </a>
        )}
      </div>
      {!report && !failed && (
        <p role="status" className="text-sm text-muted-foreground">
          Loading benchmark results…
        </p>
      )}
      {failed && !report && (
        <div
          role="alert"
          className="rounded-xl border border-border p-4 text-sm"
        >
          <p>Benchmark results could not be loaded. Try loading them again.</p>
          <button
            type="button"
            onClick={() => {
              setFailed(false)
              setAttempt((value) => value + 1)
            }}
            className="mt-3 rounded-md border border-border px-3 py-2 font-medium focus-visible:outline-2 focus-visible:outline-ring"
          >
            Retry loading results
          </button>
        </div>
      )}
      {report && (
        <>
          <div className="grid gap-4 sm:grid-cols-[minmax(0,1fr)_auto]">
            <label className="flex min-w-0 flex-1 flex-col gap-2 text-xs font-medium">
              Workload
              <select
                aria-label="Workload"
                value={scenario}
                onChange={(event) =>
                  setScenario(event.target.value as keyof typeof scenarios)
                }
                className="min-w-0 rounded-md border border-border bg-background px-3 py-2 text-sm focus-visible:outline-2 focus-visible:outline-ring"
              >
                {Object.entries(scenarios).map(([key, label]) => (
                  <option key={key} value={key}>
                    {label}
                  </option>
                ))}
              </select>
            </label>
            <label className="flex flex-col gap-2 text-xs font-medium">
              Concurrent workers
              <select
                aria-label="Concurrent workers"
                value={workers}
                onChange={(event) => setWorkers(Number(event.target.value))}
                className="rounded-md border border-border bg-background px-3 py-2 text-sm focus-visible:outline-2 focus-visible:outline-ring"
              >
                <option value={1}>1 worker</option>
                <option value={16}>16 workers</option>
              </select>
            </label>
          </div>
          <p className="text-xs leading-6 text-muted-foreground">
            <span className="mb-1 block font-medium text-foreground">
              {scenarios[scenario]} · {workers}{' '}
              {workers === 1 ? 'worker' : 'workers'}
            </span>
            {scenario.startsWith('http-')
              ? 'Full HTTP round trip. Accepted requests reach the local upstream; rejected requests stop at the proxy.'
              : 'Limiter decision only, including locking and the Redis round trip.'}
          </p>
          <div
            className="grid gap-3 sm:grid-cols-2"
            aria-label="Selected algorithm latency"
          >
            {selected.map((row) => (
              <div
                key={row.backend}
                className="rounded-xl border border-border bg-muted/20 p-4"
              >
                <p className="text-xs font-medium text-muted-foreground">
                  {algorithmNames[row.algorithm]} ·{' '}
                  {row.backend === 'ram' ? 'RAM' : 'Redis'}
                </p>
                <p className="mt-2 text-2xl font-semibold tabular-nums">
                  {latency(row.p95_us)}{' '}
                  <span className="text-xs font-normal text-muted-foreground">
                    p95
                  </span>
                </p>
                <p className="mt-2 text-xs text-muted-foreground">
                  {number.format(row.samples)} requests ·{' '}
                  {number.format(row.operations_per_second)} ops/s ·{' '}
                  {row.errors} errors
                </p>
              </div>
            ))}
          </div>
          <div className="flex flex-wrap items-center justify-between gap-3">
            <p className="text-xs text-muted-foreground">
              Compare all five · scroll for more columns
            </p>
            <button
              type="button"
              aria-pressed={allMetrics}
              onClick={() => setAllMetrics((value) => !value)}
              className="min-h-9 rounded-md border border-border px-3 text-xs font-medium focus-visible:outline-2 focus-visible:outline-ring"
            >
              {allMetrics ? 'Show key metrics' : 'Show all metrics'}
            </button>
          </div>
          <div
            role="region"
            aria-label="Latency comparison table"
            tabIndex={0}
            className="overflow-x-auto rounded-xl border border-border focus-visible:outline-2 focus-visible:outline-ring"
          >
            <table className="w-full text-left text-xs whitespace-nowrap">
              <caption className="sr-only">
                {scenarios[scenario]}, {workers} concurrent workers. Latency
                units appear in every cell.
              </caption>
              <thead className="bg-muted/40">
                <tr>
                  {[
                    'Algorithm',
                    'Storage',
                    'p50',
                    'p95',
                    'p99',
                    ...(allMetrics ? ['Maximum'] : []),
                    'Ops/s',
                    ...(allMetrics ? ['Samples', 'Accepted', 'Rejected'] : []),
                    'Errors',
                  ].map((label) => (
                    <th
                      key={label}
                      scope="col"
                      className="px-4 py-3 font-semibold"
                    >
                      {label}
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody className="divide-y divide-border">
                {rows.map((row) => (
                  <tr
                    key={`${row.algorithm}-${row.backend}`}
                    className={
                      row.algorithm.replaceAll('_', '-') === activeAlgorithm
                        ? 'bg-primary/5'
                        : ''
                    }
                  >
                    <th scope="row" className="px-4 py-3 font-medium">
                      {algorithmNames[row.algorithm]}
                    </th>
                    <td className="px-4 py-3">
                      {row.backend === 'ram' ? 'RAM' : 'Redis'}
                    </td>
                    {[
                      row.p50_us,
                      row.p95_us,
                      row.p99_us,
                      ...(allMetrics ? [row.max_us] : []),
                    ].map((value, index) => (
                      <td key={index} className="px-4 py-3 tabular-nums">
                        {latency(value)}
                      </td>
                    ))}
                    {[
                      row.operations_per_second,
                      ...(allMetrics
                        ? [row.samples, row.allowed, row.rejected]
                        : []),
                      row.errors,
                    ].map((value, index) => (
                      <td key={index} className="px-4 py-3 tabular-nums">
                        {number.format(value)}
                      </td>
                    ))}
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <div className="rounded-xl border border-border bg-muted/20 p-4 text-xs leading-6 text-muted-foreground">
            <p>
              Local benchmark · {number.format(selected[0]?.samples ?? 0)}{' '}
              requests per case · p95 = 95% finished within this time.
            </p>
            <details className="mt-2">
              <summary className="cursor-pointer rounded-sm font-medium text-foreground focus-visible:outline-2 focus-visible:outline-ring">
                Benchmark method and settings
              </summary>
              <p>
                Measured{' '}
                {new Intl.DateTimeFormat('en-IN', {
                  dateStyle: 'medium',
                  timeStyle: 'short',
                  timeZone: 'Asia/Kolkata',
                }).format(new Date(report.timestamp))}{' '}
                IST · {report.goos}/{report.goarch} · {report.cpus} CPUs ·{' '}
                {report.go_version} · Redis {report.redis_version}
              </p>
              <p>
                One local run, not a production guarantee. Hardware, log size,
                and network affect results.
              </p>
              <p className="mt-2">{report.methodology}</p>
              <p>
                {report.warmup_requests} warmup requests per case; window{' '}
                {report.window}; accepted workload limit and bucket capacity{' '}
                {number.format(report.allowed_limit)}; rejected workload limit
                and capacity {report.denied_limit}; maximum{' '}
                {number.format(report.max_keys)} identities. The many-identity
                workload cycles through 1,000 identities. Measurements exclude
                setup and warmup. Workers issue a new request after their
                previous request completes.
              </p>
              <p className="mt-2">
                p50: half the requests completed within this time. p95/p99:
                95%/99% completed within this time. Throughput is completed
                operations divided by the total measured duration.
              </p>
            </details>
          </div>
        </>
      )}
    </section>
  )
}
