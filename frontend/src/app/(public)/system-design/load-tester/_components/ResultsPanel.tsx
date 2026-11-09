'use client'

import { useState } from 'react'
import {
  ArrowDownToLine,
  CheckCircle2,
  Gauge,
  Loader2,
  Terminal,
  XCircle,
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import { api, apiBase, type Job, type Report } from './types'

function readableReason(reason: string): string {
  if (reason.includes('maxErrorRate'))
    return 'Too many requests failed or were rejected at this load.'
  if (reason.includes('p95ThresholdMs'))
    return 'Responses were slower than the chosen response-time limit.'
  if (reason.includes('generator') || reason.includes('Incomplete measurement'))
    return 'The test tool could not send all the requested traffic. Try this load again with more test resources.'
  if (reason.includes('100 successful') || reason.includes('latency threshold'))
    return 'There were not enough successful responses to judge this load. Try a longer test.'
  return reason
}

export function ResultsPanel({
  job,
  report,
}: {
  job: Job | null
  report: Report | null
}) {
  const [logs, setLogs] = useState<{ id: string; text: string } | null>(null)
  const [loadingLogs, setLoadingLogs] = useState(false)
  async function showLogs() {
    if (!job) return
    setLoadingLogs(true)
    try {
      const value = await api<{ logs: string }>('/tests/' + job.id + '/logs')
      setLogs({ id: job.id, text: value.logs })
    } catch (e) {
      setLogs({
        id: job.id,
        text: e instanceof Error ? e.message : 'Logs unavailable',
      })
    } finally {
      setLoadingLogs(false)
    }
  }
  if (!report)
    return (
      <section
        className="flex min-h-80 flex-col justify-center rounded-xl border border-dashed border-border p-6 text-center"
        aria-label="Test results"
      >
        <div className="mx-auto mb-4 flex size-11 items-center justify-center rounded-xl bg-muted text-primary">
          <Gauge className="size-5" />
        </div>
        <h2 className="text-base">
          {job?.status === 'failed'
            ? 'Test could not finish'
            : job?.status === 'cancelled'
              ? 'Test cancelled'
              : job
                ? 'Measuring your endpoint'
                : 'Find out what your server can handle'}
        </h2>
        <p className="mx-auto mt-2 max-w-sm text-sm">
          {job?.error ||
            (job?.status === 'cancelled'
              ? 'No recommendation was generated. You can adjust the workload and run again.'
              : job
                ? 'Results appear when the run finishes. You can leave this page and reopen the test from recent runs.'
                : 'Enter your address and the maximum load to test. We’ll increase traffic automatically and suggest a limit based on the results.')}
        </p>
        {!job && (
          <div className="mt-6 flex flex-wrap justify-center gap-3 text-xs text-muted-foreground">
            <span>1. Enter your address</span>
            <span aria-hidden>→</span>
            <span>2. Test your server</span>
            <span aria-hidden>→</span>
            <span>3. See your results</span>
          </div>
        )}
        {job && ['failed', 'cancelled'].includes(job.status) && (
          <Button
            className="mx-auto mt-4"
            variant="outline"
            onClick={showLogs}
            disabled={loadingLogs}
          >
            <Terminal />
            View execution logs
          </Button>
        )}
        {logs && logs.id === job?.id && (
          <pre className="mt-4 max-h-48 overflow-auto rounded-lg bg-muted p-3 text-start text-xs break-all whitespace-pre-wrap">
            {logs?.text}
          </pre>
        )}
      </section>
    )
  const result = report.results
  const analysis = result.rateAnalysis
  const suggestion = analysis?.recommendedRateLimitRps
  const total =
    result.metrics.http_reqs?.values.count ??
    Object.values(result.statusCounts).reduce((a, b) => a + b, 0)
  const successful = Object.entries(result.statusCounts).reduce(
    (sum, [status, n]) =>
      Number(status) >= 200 && Number(status) < 300 ? sum + n : sum,
    0,
  )
  const limited = result.statusCounts['429'] ?? 0
  const p95 = result.metrics.successful_latency?.values['p(95)']
  return (
    <section className="min-w-0 space-y-4" aria-label="Test results">
      <div className="rounded-xl border border-primary/25 bg-primary/5 p-5">
        <div className="flex items-start justify-between gap-3">
          <div>
            <h2 className="text-sm text-primary">Suggested limit</h2>
            <div className="mt-2 flex items-baseline gap-2">
              <strong className="text-4xl font-semibold tracking-tight tabular-nums">
                {suggestion ?? '—'}
              </strong>
              <span className="text-sm text-muted-foreground">
                requests / second
              </span>
            </div>
          </div>
          <Gauge className="mt-1 size-5 text-primary" />
        </div>
        <p className="mt-3 text-xs">
          {suggestion != null
            ? `${analysis?.headroomPercent}% spare capacity below ${analysis?.highestPassingRps} requests per second, the highest tested load that passed.`
            : analysis
              ? 'Not enough passing evidence to suggest a limit. Review the step reasons below.'
              : 'This run used virtual users. Start a capacity test to get rate guidance.'}
        </p>
        {analysis?.boundary === 'lower_bound' && (
          <p className="mt-2 text-xs font-medium text-primary">
            Your server passed every load we tried. Its maximum capacity may be
            higher.
          </p>
        )}
        <p className="mt-2 text-xs">
          A starting point for this address and test. This is a total traffic
          limit, not a separate limit for each visitor.
        </p>
      </div>
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
        {[
          ['Requests', total.toLocaleString()],
          [
            'Response time (95%)',
            p95 != null ? `${p95.toFixed(1)} ms` : 'Unavailable',
          ],
          ['Rate limited', limited.toLocaleString()],
          [
            'Other failures',
            Math.max(0, total - successful - limited).toLocaleString(),
          ],
        ].map(([label, value]) => (
          <div
            key={label}
            className="rounded-lg border border-border bg-card p-3"
          >
            <div className="text-xs text-muted-foreground">{label}</div>
            <div className="mt-1 text-lg font-semibold tabular-nums">
              {value}
            </div>
          </div>
        ))}
      </div>
      {analysis && (
        <div className="overflow-hidden rounded-xl border border-border bg-card">
          <div className="flex flex-wrap items-center justify-between gap-2 px-4 py-3">
            <h2 className="text-sm">Tested loads</h2>
            <span className="text-xs text-muted-foreground">
              Time for 95% of successful responses
            </span>
          </div>
          <p className="px-4 pb-2 text-xs text-muted-foreground sm:hidden">
            Scroll the table horizontally for all columns.
          </p>
          <div
            className="overflow-x-auto focus-visible:outline-2 focus-visible:outline-ring"
            role="region"
            aria-label="Load step measurements"
            tabIndex={0}
          >
            <table className="w-full text-start text-xs tabular-nums">
              <caption className="sr-only">
                Requested and delivered requests per second, latency, errors,
                dropped iterations and evaluation for each step
              </caption>
              <thead className="border-y border-border bg-muted/60 text-muted-foreground">
                <tr>
                  {[
                    'Load / sec',
                    'Sent / sec',
                    'Response time',
                    'Failed',
                    'Not sent',
                    'Result',
                  ].map((label) => (
                    <th
                      key={label}
                      scope="col"
                      className="px-3 py-2 text-start font-medium whitespace-nowrap"
                    >
                      {label}
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody className="divide-y divide-border">
                {analysis.steps.map((step, i) => (
                  <tr key={i}>
                    <td className="px-3 py-3 font-medium">
                      {step.requestedRps}
                    </td>
                    <td className="px-3 py-3">{step.observedRps.toFixed(1)}</td>
                    <td className="px-3 py-3 whitespace-nowrap">
                      {step.successfulP95Ms?.toFixed(1) ?? '—'}
                      {step.successfulP95Ms != null ? ' ms' : ''}
                    </td>
                    <td className="px-3 py-3">
                      {(step.errorRate * 100).toFixed(1)}%
                    </td>
                    <td className="px-3 py-3">{step.droppedIterations}</td>
                    <td className="px-3 py-3">
                      <span
                        className={`inline-flex items-center gap-1.5 whitespace-nowrap ${step.status === 'failed' ? 'text-destructive' : step.status === 'passed' ? 'text-primary' : 'text-muted-foreground'}`}
                      >
                        {step.status === 'passed' ? (
                          <CheckCircle2 className="size-3.5" />
                        ) : step.status === 'failed' ? (
                          <XCircle className="size-3.5" />
                        ) : null}
                        {step.status}
                      </span>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          {analysis.steps.some((step) => step.reasons.length > 0) && (
            <ul className="space-y-2 border-t border-border px-4 py-3 text-xs text-muted-foreground">
              {analysis.steps.flatMap((step, i) =>
                step.reasons.map((reason) => (
                  <li key={`${i}-${reason}`}>
                    <strong className="font-medium text-foreground">
                      {step.requestedRps} requests / second:
                    </strong>{' '}
                    {readableReason(reason)}
                  </li>
                )),
              )}
            </ul>
          )}
        </div>
      )}
      <details className="rounded-lg border border-border bg-card px-4 py-3">
        <summary className="cursor-pointer text-sm font-medium">
          Technical details &amp; full report
        </summary>
        <dl className="mt-3 grid grid-cols-2 gap-3 text-xs">
          <div>
            <dt className="text-muted-foreground">Target</dt>
            <dd className="mt-1 break-all">
              {report.test.config.method} {report.test.config.targetUrl}
            </dd>
          </div>
          <div>
            <dt className="text-muted-foreground">Threshold outcome</dt>
            <dd className="mt-1">
              {report.test.thresholdsPassed ? 'Passed' : 'Failed'} · exit{' '}
              {report.test.exitCode}
            </dd>
          </div>
          <div>
            <dt className="text-muted-foreground">HTTP status counts</dt>
            <dd className="mt-1">
              {Object.entries(result.statusCounts)
                .map(([code, n]) => `${code}: ${n}`)
                .join(' · ')}
            </dd>
          </div>
          <div>
            <dt className="text-muted-foreground">
              Request timeouts / transport failures
            </dt>
            <dd className="mt-1">
              {result.metrics.request_timeouts?.values.count ?? 0} /{' '}
              {result.metrics.transport_failures?.values.count ?? 0}
            </dd>
          </div>
        </dl>
        <ul className="mt-4 space-y-2 text-xs text-muted-foreground">
          {result.insights.map((text) => (
            <li key={text}>{text}</li>
          ))}
        </ul>
        <div className="mt-4 flex flex-wrap gap-2">
          <Button variant="outline" onClick={showLogs} disabled={loadingLogs}>
            {loadingLogs ? <Loader2 className="animate-spin" /> : <Terminal />}
            Execution logs
          </Button>
          <a
            className="inline-flex min-h-8 items-center gap-2 rounded-lg border border-border px-3 text-xs hover:bg-muted focus-visible:outline-2 focus-visible:outline-ring"
            href={`${apiBase}/tests/${report.test.id}/results`}
            download={`load-test-${report.test.id}.json`}
          >
            <ArrowDownToLine className="size-3.5" />
            Download JSON
          </a>
        </div>
        {logs?.id === report.test.id && (
          <pre className="mt-3 max-h-64 overflow-auto rounded-lg bg-muted p-3 text-xs break-all whitespace-pre-wrap">
            {logs.text}
          </pre>
        )}
      </details>
    </section>
  )
}
