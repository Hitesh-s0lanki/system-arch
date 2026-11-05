export type TestConfig = {
  targetUrl: string
  method: string
  virtualUsers?: number
  maxVirtualUsers?: number
  durationSeconds?: number
  requestsPerSecond?: number
  rateSteps?: { requestsPerSecond: number; durationSeconds: number }[]
  requestTimeoutSeconds: number
  p95ThresholdMs: number
  maxErrorRate: number
  headroomPercent: number
  headers?: Record<string, string>
  body?: string
}
export type Job = {
  id: string
  status:
    'queued' | 'running' | 'cancelling' | 'cancelled' | 'completed' | 'failed'
  config: TestConfig
  createdAt: string
  startedAt?: string
  finishedAt?: string
  error?: string
  thresholdsPassed?: boolean
  exitCode?: number
}
export type RateStep = {
  requestedRps: number
  observedRps: number
  durationSeconds: number
  requests: number
  successfulRequests: number
  statusCounts: Record<string, number>
  errorRate: number
  successfulP95Ms: number | null
  droppedIterations: number
  status: string
  reasons: string[]
}
export type Report = {
  test: Job
  results: {
    metrics: Record<string, { values: Record<string, number> }>
    statusCounts: Record<string, number>
    insights: string[]
    rateAnalysis?: {
      steps: RateStep[]
      highestPassingRps: number
      recommendedRateLimitRps: number | null
      headroomPercent: number
      boundary: string
      scope: string
      explanation: string
    }
  }
}
export const apiBase = '/api/load-tester'
export const activeStatus = (job: Job | null) =>
  !!job && ['queued', 'running', 'cancelling'].includes(job.status)
export async function api<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(apiBase + path, {
    ...init,
    cache: 'no-store',
    headers: { 'Content-Type': 'application/json', ...init?.headers },
  })
  const data = await response.json()
  if (!response.ok)
    throw new Error(data.error || `Request failed (${response.status})`)
  return data as T
}
